package app

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/emersion/go-smtp"

	"oubliette/internal/httpapi"
	"oubliette/internal/intake"
	"oubliette/internal/live"
	"oubliette/internal/store"
	"oubliette/web"
)

const smtpDrain = 2 * time.Second

type Config struct {
	DataFile           string
	HTTPAddr           string
	SMTPAddr           string
	InsecureAuth       bool
	SMTPCertFile       string
	SMTPKeyFile        string
	MaxMessageBytes    int64
	InboxCap           int
	AgeLimit           time.Duration
	PurgeEvery         time.Duration
	SMTPAdvertisedHost string
	SMTPAdvertisedPort string
	Now                func() time.Time
	Log                *slog.Logger
}

type Server struct {
	store     *store.Store
	http      *http.Server
	httpAddr  string
	smtp      *smtp.Server
	smtpAddr  string
	stopPurge func()
}

func Start(cfg Config) (*Server, error) {
	cfg = withDefaultLimits(cfg)
	encryption, err := smtpEncryption(cfg)
	if err != nil {
		return nil, err
	}
	advertised, err := advertisedSMTP(cfg)
	if err != nil {
		return nil, err
	}
	storage, err := store.Open(cfg.DataFile)
	if err != nil {
		return nil, err
	}
	httpListener, httpErr := net.Listen("tcp", cfg.HTTPAddr)
	smtpListener, smtpErr := net.Listen("tcp", cfg.SMTPAddr)
	err = errors.Join(httpErr, smtpErr)
	if err == nil {
		err = createFirstInbox(storage, cfg)
	}
	if err != nil {
		for _, listener := range []net.Listener{httpListener, smtpListener} {
			if listener != nil {
				listener.Close()
			}
		}
		storage.Close()
		return nil, err
	}
	smtpPort := smtpListener.Addr().(*net.TCPAddr).Port
	hub := live.NewHub()
	streaming, endStreams := context.WithCancel(context.Background())
	server := &Server{
		store: storage,
		http: &http.Server{
			Handler:           httpapi.New(storage, hub, httpapi.SMTPEndpoint{Host: advertised.Host, Port: cmp.Or(advertised.Port, smtpPort)}, web.Dist(), cfg.Now, cfg.Log),
			ReadHeaderTimeout: 10 * time.Second,
			ErrorLog:          slog.NewLogLogger(cfg.Log.Handler(), slog.LevelError),
			BaseContext:       func(net.Listener) context.Context { return streaming },
		},
		httpAddr: httpListener.Addr().String(),
		smtp: intake.NewServer(storage, hub, intake.Rules{
			Encryption:      encryption,
			InsecureAuth:    cfg.InsecureAuth,
			MaxMessageBytes: cfg.MaxMessageBytes,
			InboxCap:        cfg.InboxCap,
		}, cfg.Now, cfg.Log),
		smtpAddr: smtpListener.Addr().String(),
	}
	server.http.RegisterOnShutdown(endStreams)
	server.stopPurge = purgePeriodically(storage, hub, cfg)
	go func() {
		if err := server.http.Serve(httpListener); !errors.Is(err, http.ErrServerClosed) {
			cfg.Log.Error("http server stopped", "error", err)
		}
	}()
	go func() {
		if err := server.smtp.Serve(smtpListener); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
			cfg.Log.Error("smtp server stopped", "error", err)
		}
	}()
	cfg.Log.Info("listening", "http", server.httpAddr, "smtp", server.smtpAddr, "data", cfg.DataFile)
	return server, nil
}

func advertisedSMTP(cfg Config) (httpapi.SMTPEndpoint, error) {
	host := cfg.SMTPAdvertisedHost
	if host != "" && net.ParseIP(host) == nil && strings.IndexFunc(host, notInHostname) >= 0 {
		return httpapi.SMTPEndpoint{}, errors.New("the advertised SMTP host must be a host name or an IP address, without scheme or port: " +
			"check --smtp-advertised-host (OUBLIETTE_SMTP_ADVERTISED_HOST)")
	}
	if cfg.SMTPAdvertisedPort == "" {
		return httpapi.SMTPEndpoint{Host: host}, nil
	}
	port, err := strconv.ParseUint(cfg.SMTPAdvertisedPort, 10, 16)
	if err != nil || port == 0 {
		return httpapi.SMTPEndpoint{}, errors.New("the advertised SMTP port must be a number from 1 to 65535: " +
			"check --smtp-advertised-port (OUBLIETTE_SMTP_ADVERTISED_PORT)")
	}
	return httpapi.SMTPEndpoint{Host: host, Port: int(port)}, nil
}

func notInHostname(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '-'
}

func createFirstInbox(storage *store.Store, cfg Config) error {
	inbox, created, err := storage.CreateFirstInbox(context.Background(), "Default", cfg.Now())
	if err != nil || !created {
		return err
	}
	cfg.Log.Info("created the first Inbox", "name", inbox.Name, "smtp_username", inbox.Username, "smtp_password", inbox.Password)
	return nil
}

func (s *Server) HTTPAddr() string {
	return s.httpAddr
}

func (s *Server) SMTPAddr() string {
	return s.smtpAddr
}

func (s *Server) Stop(ctx context.Context) error {
	draining, cancel := context.WithTimeout(ctx, smtpDrain)
	defer cancel()
	s.smtp.Shutdown(draining)
	s.stopPurge()
	return errors.Join(s.http.Shutdown(ctx), s.store.Close())
}

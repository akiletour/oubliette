package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/emersion/go-smtp"

	"oubliette/internal/httpapi"
	"oubliette/internal/intake"
	"oubliette/internal/store"
	"oubliette/web"
)

const smtpDrain = 2 * time.Second

type Config struct {
	DataFile     string
	HTTPAddr     string
	SMTPAddr     string
	InsecureAuth bool
	Now          func() time.Time
	Log          *slog.Logger
}

type Server struct {
	store    *store.Store
	http     *http.Server
	httpAddr string
	smtp     *smtp.Server
	smtpAddr string
}

func Start(cfg Config) (*Server, error) {
	if !cfg.InsecureAuth {
		return nil, errors.New("SMTP encryption is not available yet, so Credentials would travel unencrypted: " +
			"start with --smtp-insecure-auth (OUBLIETTE_SMTP_INSECURE_AUTH=true) to accept that on a private network")
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
	server := &Server{
		store: storage,
		http: &http.Server{
			Handler:           httpapi.New(storage, web.Dist(), cfg.Now, cfg.Log),
			ReadHeaderTimeout: 10 * time.Second,
			ErrorLog:          slog.NewLogLogger(cfg.Log.Handler(), slog.LevelError),
		},
		httpAddr: httpListener.Addr().String(),
		smtp:     intake.NewServer(storage, cfg.Now, cfg.Log),
		smtpAddr: smtpListener.Addr().String(),
	}
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

func createFirstInbox(storage *store.Store, cfg Config) error {
	inboxes, err := storage.Inboxes(context.Background())
	if err != nil || len(inboxes) > 0 {
		return err
	}
	inbox, err := storage.CreateInbox(context.Background(), "Default", cfg.Now())
	if err != nil {
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
	return errors.Join(s.http.Shutdown(ctx), s.store.Close())
}

package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"oubliette/internal/httpapi"
	"oubliette/internal/store"
	"oubliette/web"
)

type Config struct {
	DataFile string
	HTTPAddr string
	Now      func() time.Time
	Log      *slog.Logger
}

type Server struct {
	store    *store.Store
	http     *http.Server
	httpAddr string
}

func Start(cfg Config) (*Server, error) {
	storage, err := store.Open(cfg.DataFile)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
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
		httpAddr: listener.Addr().String(),
	}
	go func() {
		if err := server.http.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			cfg.Log.Error("http server stopped", "error", err)
		}
	}()
	cfg.Log.Info("listening", "http", server.httpAddr, "data", cfg.DataFile)
	return server, nil
}

func (s *Server) HTTPAddr() string {
	return s.httpAddr
}

func (s *Server) Stop(ctx context.Context) error {
	return errors.Join(s.http.Shutdown(ctx), s.store.Close())
}

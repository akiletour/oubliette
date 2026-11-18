package app

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const certificateOptions = "--smtp-tls-cert and --smtp-tls-key (OUBLIETTE_SMTP_TLS_CERT, OUBLIETTE_SMTP_TLS_KEY)"

func smtpEncryption(cfg Config) (*tls.Config, error) {
	if cfg.SMTPCertFile == "" && cfg.SMTPKeyFile == "" {
		if !cfg.InsecureAuth {
			return nil, errors.New("SMTP Credentials would travel unencrypted: supply a certificate with " + certificateOptions +
				", or start with --smtp-insecure-auth (OUBLIETTE_SMTP_INSECURE_AUTH=true) to accept authentication without encryption on a private network")
		}
		return nil, nil
	}
	if cfg.SMTPCertFile == "" || cfg.SMTPKeyFile == "" {
		return nil, errors.New("the SMTP certificate needs both " + certificateOptions)
	}
	files := &certificateFiles{cert: cfg.SMTPCertFile, key: cfg.SMTPKeyFile, log: cfg.Log}
	certificate, err := files.load()
	if err != nil {
		return nil, fmt.Errorf("the SMTP certificate could not be loaded: check %s: %w", certificateOptions, err)
	}
	if expiry := certificate.Leaf.NotAfter; cfg.Now().After(expiry) {
		return nil, fmt.Errorf("the SMTP certificate expired on %s: check %s", expiry.Format(time.DateOnly), certificateOptions)
	}
	if cfg.InsecureAuth {
		cfg.Log.Warn("SMTP authentication is still accepted without encryption although a certificate is supplied: " +
			"remove --smtp-insecure-auth (OUBLIETTE_SMTP_INSECURE_AUTH) to require STARTTLS")
	}
	files.offered = certificate
	return &tls.Config{GetCertificate: files.current}, nil
}

type certificateFiles struct {
	cert, key string
	log       *slog.Logger
	mu        sync.Mutex
	offered   *tls.Certificate
}

func (f *certificateFiles) load() (*tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(f.cert, f.key)
	return &certificate, err
}

func (f *certificateFiles) current(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	certificate, err := f.load()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.log.Warn("the SMTP certificate could not be read again, the previous one is still offered", "error", err)
		return f.offered, nil
	}
	f.offered = certificate
	return certificate, nil
}

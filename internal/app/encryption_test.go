package app_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oubliette/internal/app"
)

const loopback = "127.0.0.1"

type certificate struct {
	certFile, keyFile string
	trusted           *x509.CertPool
}

func newCertificate(t *testing.T) certificate {
	t.Helper()
	dir := t.TempDir()
	return issueCertificate(t, filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"), time.Now().Add(time.Hour))
}

func issueCertificate(t *testing.T, certFile, keyFile string, notAfter time.Time) certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		NotBefore:    notAfter.Add(-48 * time.Hour),
		NotAfter:     notAfter,
		IPAddresses:  []net.IP{net.ParseIP(loopback)},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	issued := certificate{certFile: certFile, keyFile: keyFile, trusted: x509.NewCertPool()}
	issued.trusted.AddCert(parsed)
	writePEM(t, certFile, "CERTIFICATE", der)
	writePEM(t, keyFile, "PRIVATE KEY", keyDER)
	return issued
}

func writePEM(t *testing.T, path, kind string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func startEncrypted(t *testing.T) (*instance, certificate) {
	t.Helper()
	cert := newCertificate(t)
	i := startWith(t, func(cfg *app.Config) {
		cfg.InsecureAuth = false
		cfg.SMTPCertFile, cfg.SMTPKeyFile = cert.certFile, cert.keyFile
	})
	return i, cert
}

func (i *instance) StartTLS(cert certificate) (*smtp.Client, error) {
	i.t.Helper()
	client, err := i.Connect(nil)
	if err != nil {
		i.t.Fatal(err)
	}
	if offered, _ := client.Extension("STARTTLS"); !offered {
		i.t.Fatal("STARTTLS is not offered")
	}
	return client, client.StartTLS(&tls.Config{RootCAs: cert.trusted, ServerName: loopback})
}

func serveWith(t *testing.T, env map[string]string, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	args = append([]string{"--data", filepath.Join(t.TempDir(), "oubliette.db"), "--http-addr", loopback + ":0", "--smtp-addr", loopback + ":0"}, args...)
	code := app.Main(ctx, append(args, "serve"), strings.NewReader(""), io.Discard, &stderr, func(key string) string { return env[key] })
	return code, stderr.String()
}

func TestServerRefusesToStartWithoutACertificateOrTheInsecureAuthenticationOverride(t *testing.T) {
	t.Parallel()

	code, refusal := serveWith(t, nil)

	if code == 0 {
		t.Fatal("serve started without a certificate or the insecure-authentication override")
	}
	for _, option := range []string{"--smtp-tls-cert", "--smtp-insecure-auth"} {
		if !strings.Contains(refusal, option) {
			t.Fatalf("the refusal does not name %s: %s", option, refusal)
		}
	}
}

func TestServerRefusesAnUnreadableInsecureAuthenticationOverride(t *testing.T) {
	t.Parallel()

	cert := newCertificate(t)

	for name, args := range map[string][]string{
		"without a certificate": nil,
		"with a certificate":    {"--smtp-tls-cert", cert.certFile, "--smtp-tls-key", cert.keyFile},
	} {
		code, refusal := serveWith(t, map[string]string{"OUBLIETTE_SMTP_INSECURE_AUTH": "yes"}, args...)
		if code == 0 || !strings.Contains(refusal, "OUBLIETTE_SMTP_INSECURE_AUTH must be true or false") {
			t.Errorf("an unreadable override %s gave exit %d and %q, want a refusal naming the variable", name, code, refusal)
		}
	}
}

func TestServerRefusesToStartWithAnUnusableCertificate(t *testing.T) {
	t.Parallel()
	good, other := newCertificate(t), newCertificate(t)
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.pem")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.pem")
	expired := issueCertificate(t, filepath.Join(dir, "expired.pem"), filepath.Join(dir, "expired-key.pem"), time.Now().Add(-time.Hour))
	cases := []struct {
		name, cert, key, want string
	}{
		{"unreadable certificate", missing, good.keyFile, missing},
		{"invalid certificate", garbage, good.keyFile, "PEM"},
		{"key of another certificate", good.certFile, other.keyFile, "private key does not match"},
		{"expired certificate", expired.certFile, expired.keyFile, "expired"},
		{"certificate without its key", good.certFile, "", "needs both --smtp-tls-cert and --smtp-tls-key"},
		{"key without its certificate", "", good.keyFile, "needs both --smtp-tls-cert and --smtp-tls-key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			code, refusal := serveWith(t, nil, "--smtp-tls-cert", c.cert, "--smtp-tls-key", c.key)

			if code == 0 {
				t.Fatal("serve started with an unusable certificate")
			}
			if !strings.Contains(refusal, c.want) || !strings.Contains(refusal, "certificate") {
				t.Fatalf("the refusal %q does not explain %q", refusal, c.want)
			}
		})
	}
}

func TestMessageSubmittedAfterStartTLSIsHeld(t *testing.T) {
	i, cert := startEncrypted(t)

	client, err := i.StartTLS(cert)
	if err != nil {
		t.Fatalf("STARTTLS: %v", err)
	}
	if err := client.Auth(plain(i.LoggedCredentials()[0])); err != nil {
		t.Fatalf("authenticate after STARTTLS: %v", err)
	}
	if err := transmit(client, "app@example.com", []string{"dev@example.com"}, "Subject: Encrypted\r\n\r\nHello\r\n"); err != nil {
		t.Fatalf("submit after STARTTLS: %v", err)
	}
	if err := client.Quit(); err != nil {
		t.Fatal(err)
	}

	messages := i.SignedInMember().Messages()
	if len(messages) != 1 || messages[0].Subject != "Encrypted" {
		t.Fatalf("held Messages = %+v, want the one submitted after STARTTLS", messages)
	}
}

func TestAuthenticationIsRefusedBeforeStartTLS(t *testing.T) {
	i, _ := startEncrypted(t)
	application := i.LoggedCredentials()[0]

	for _, mechanism := range []smtp.Auth{plain(application), login(application)} {
		client, err := i.Connect(nil)
		if err != nil {
			t.Fatal(err)
		}
		if offered, _ := client.Extension("AUTH"); offered {
			t.Fatal("authentication is offered before STARTTLS")
		}
		err = client.Auth(mechanism)
		var refusal *textproto.Error
		if !errors.As(err, &refusal) || refusal.Code/100 != 5 {
			t.Fatalf("authentication before STARTTLS gave %v, want a permanent refusal", err)
		}
		if err := client.Mail("app@example.com"); err == nil {
			t.Fatal("a Message is accepted without authentication")
		}
	}
}

func TestARenewedCertificateIsOfferedWithoutARestart(t *testing.T) {
	i, cert := startEncrypted(t)

	renewed := issueCertificate(t, cert.certFile, cert.keyFile, time.Now().Add(2*time.Hour))

	if _, err := i.StartTLS(renewed); err != nil {
		t.Fatalf("STARTTLS with the renewed certificate: %v", err)
	}
}

func TestTheLastUsableCertificateIsKeptWhileTheFilesAreBroken(t *testing.T) {
	i, cert := startEncrypted(t)

	if err := os.WriteFile(cert.certFile, []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := i.StartTLS(cert); err != nil {
		t.Fatalf("STARTTLS while the certificate file is broken: %v", err)
	}
}

func TestTheOperatorIsWarnedThatTheOverrideStillAcceptsUnencryptedAuthentication(t *testing.T) {
	cert := newCertificate(t)
	i := startWith(t, func(cfg *app.Config) {
		cfg.SMTPCertFile, cfg.SMTPKeyFile = cert.certFile, cert.keyFile
	})

	if !strings.Contains(i.logs.String(), `"level":"WARN"`) || !strings.Contains(i.logs.String(), "--smtp-insecure-auth") {
		t.Fatalf("a start with a certificate and the override does not warn that authentication is still accepted unencrypted: %s", i.logs.String())
	}
}

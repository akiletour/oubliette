package intake

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime/v2"

	"oubliette/internal/content"
	"oubliette/internal/live"
	"oubliette/internal/store"
)

const maxLineLength = 1 << 20

var (
	errAuthRequired = &smtp.SMTPError{Code: 530, EnhancedCode: smtp.EnhancedCode{5, 7, 0}, Message: "Authentication required"}
	errUnavailable  = &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary failure, try again later"}
	errStopping     = &smtp.SMTPError{Code: 421, EnhancedCode: smtp.EnhancedCode{4, 3, 2}, Message: "Shutting down, try again later"}
)

type Rules struct {
	Encryption      *tls.Config
	InsecureAuth    bool
	MaxMessageBytes int64
	InboxCap        int
	AnonymousInbox  string
}

type backend struct {
	rules       Rules
	store       *store.Store
	hub         *live.Hub
	now         func() time.Time
	log         *slog.Logger
	mu          sync.Mutex
	stopping    bool
	connections map[*connection]struct{}
}

type Server struct {
	smtp    *smtp.Server
	backend *backend
}

func NewServer(storage *store.Store, hub *live.Hub, rules Rules, now func() time.Time, log *slog.Logger) *Server {
	intake := &backend{rules: rules, store: storage, hub: hub, now: now, log: log, connections: map[*connection]struct{}{}}
	server := smtp.NewServer(intake)
	server.Domain = "oubliette"
	server.TLSConfig = rules.Encryption
	server.AllowInsecureAuth = rules.InsecureAuth
	server.MaxMessageBytes = rules.MaxMessageBytes + 1
	server.EnableSMTPUTF8 = true
	server.MaxLineLength = maxLineLength
	server.ReadTimeout = time.Minute
	server.WriteTimeout = time.Minute
	server.ErrorLog = slog.NewLogLogger(log.Handler(), slog.LevelError)
	return &Server{smtp: server, backend: intake}
}

func (s *Server) Serve(l net.Listener) error {
	return s.smtp.Serve(listener{Listener: l, backend: s.backend})
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.backend.stop()
	return s.smtp.Shutdown(ctx)
}

type listener struct {
	net.Listener
	backend *backend
}

func (l listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return l.backend.track(conn), nil
}

type connection struct {
	net.Conn
	backend       *backend
	inTransaction bool
}

func (c *connection) Close() error {
	c.backend.forget(c)
	return c.Conn.Close()
}

func (b *backend) track(conn net.Conn) *connection {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopping {
		conn.Close()
	}
	tracked := &connection{Conn: conn, backend: b}
	b.connections[tracked] = struct{}{}
	return tracked
}

func (b *backend) forget(c *connection) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.connections, c)
}

func (b *backend) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopping = true
	for c := range b.connections {
		if !c.inTransaction {
			c.Conn.Close()
		}
	}
}

func (b *backend) begin(c *connection) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopping {
		return errStopping
	}
	c.inTransaction = true
	return nil
}

func (b *backend) end(c *connection) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c.inTransaction = false
	if b.stopping {
		c.Conn.Close()
	}
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	conn := c.Conn()
	if encrypted, isEncrypted := conn.(*tls.Conn); isEncrypted {
		conn = encrypted.NetConn()
	}
	return &session{backend: b, connection: conn.(*connection)}, nil
}

type session struct {
	backend            *backend
	connection         *connection
	authenticated      bool
	username, password string
	sender             string
	recipients         []string
}

func (s *session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login}
}

func (s *session) Auth(mechanism string) (sasl.Server, error) {
	switch mechanism {
	case sasl.Plain:
		return sasl.NewPlainServer(func(_, username, password string) error {
			return s.authenticate(username, password)
		}), nil
	case sasl.Login:
		return &loginServer{authenticate: s.authenticate}, nil
	}
	return nil, smtp.ErrAuthUnknownMechanism
}

func (s *session) authenticate(username, password string) error {
	s.authenticated, s.username, s.password = true, username, password
	if _, err := s.resolveInbox(); err != nil {
		s.authenticated = false
		return err
	}
	return nil
}

func (s *session) resolveInbox() (string, error) {
	anonymous := s.backend.rules.AnonymousInbox
	if !s.authenticated {
		if anonymous == "" {
			return "", errAuthRequired
		}
		return anonymous, nil
	}
	inbox, valid, err := s.backend.store.InboxByCredentials(context.Background(), s.username, s.password)
	switch {
	case err != nil:
		s.backend.log.Error("resolving the Inbox of a Message failed", "error", err)
		return "", errUnavailable
	case valid:
		return inbox.ID, nil
	case anonymous != "":
		return anonymous, nil
	}
	return "", smtp.ErrAuthFailed
}

type loginServer struct {
	username     *string
	authenticate func(username, password string) error
}

func (l *loginServer) Next(response []byte) ([]byte, bool, error) {
	switch {
	case l.username == nil && response == nil:
		return []byte("Username:"), false, nil
	case l.username == nil:
		username := string(response)
		l.username = &username
		return []byte("Password:"), false, nil
	}
	return nil, true, l.authenticate(*l.username, string(response))
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	if _, err := s.resolveInbox(); err != nil {
		return err
	}
	if err := s.backend.begin(s.connection); err != nil {
		return err
	}
	s.sender, s.recipients = from, nil
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.recipients = append(s.recipients, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	inboxID, err := s.resolveInbox()
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	headers, body, err := content.Split(raw)
	if err != nil {
		s.backend.log.Warn("the headers of a Message could not be read", "inbox", inboxID, "error", err)
	}
	message := store.Message{
		ReceivedAt:         s.backend.now(),
		EnvelopeSender:     s.sender,
		EnvelopeRecipients: s.recipients,
		Subject:            enmime.DecodeRFC2047(headers.Get("Subject")),
		Preview:            content.Preview(headers, body),
		Sender:             strings.Join(displayed(headers.Get("From")), ", "),
		Recipients:         append(displayed(headers.Get("To")), displayed(headers.Get("Cc"))...),
		Raw:                raw,
	}
	if message.Sender == "" {
		message.Sender = s.sender
	}
	if len(message.Recipients) == 0 {
		message.Recipients = s.recipients
	}
	id, removed, err := s.backend.store.HoldMessage(context.Background(), inboxID, message, s.backend.rules.InboxCap)
	if err != nil {
		s.backend.log.Error("holding a Message failed", "inbox", inboxID, "error", err)
		return errUnavailable
	}
	s.backend.hub.Received(inboxID, id)
	for _, message := range removed {
		s.backend.hub.Removed(message.InboxID, message.ID)
	}
	return nil
}

func displayed(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	addresses, err := enmime.ParseAddressList(header)
	if err != nil {
		return []string{enmime.DecodeRFC2047(header)}
	}
	var shown []string
	for _, address := range addresses {
		if address.Name == "" {
			shown = append(shown, address.Address)
		} else {
			shown = append(shown, address.Name+" <"+address.Address+">")
		}
	}
	return shown
}

func (s *session) Reset() {
	s.sender, s.recipients = "", nil
	s.backend.end(s.connection)
}

func (s *session) Logout() error {
	return nil
}

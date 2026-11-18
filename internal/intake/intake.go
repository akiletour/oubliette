package intake

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime/v2"

	"oubliette/internal/store"
)

const maxLineLength = 1 << 20

var (
	errAuthRequired = &smtp.SMTPError{Code: 530, EnhancedCode: smtp.EnhancedCode{5, 7, 0}, Message: "Authentication required"}
	errUnavailable  = &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary failure, try again later"}
)

type backend struct {
	store *store.Store
	now   func() time.Time
	log   *slog.Logger
}

func NewServer(storage *store.Store, now func() time.Time, log *slog.Logger) *smtp.Server {
	server := smtp.NewServer(&backend{store: storage, now: now, log: log})
	server.Domain = "oubliette"
	server.AllowInsecureAuth = true
	server.EnableSMTPUTF8 = true
	server.MaxLineLength = maxLineLength
	server.ReadTimeout = time.Minute
	server.WriteTimeout = time.Minute
	server.ErrorLog = slog.NewLogLogger(log.Handler(), slog.LevelError)
	return server
}

func (b *backend) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &session{backend: b}, nil
}

type session struct {
	backend    *backend
	inbox      *store.Inbox
	sender     string
	recipients []string
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
	inbox, valid, err := s.backend.store.InboxByCredentials(context.Background(), username, password)
	if err != nil {
		s.backend.log.Error("authentication failed", "error", err)
		return errUnavailable
	}
	if !valid {
		return smtp.ErrAuthFailed
	}
	s.inbox = &inbox
	return nil
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
	if s.inbox == nil {
		return errAuthRequired
	}
	s.sender, s.recipients = from, nil
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.recipients = append(s.recipients, to)
	return nil
}

type unreported struct{}

func (unreported) AddError(string, string, ...any)   {}
func (unreported) AddWarning(string, string, ...any) {}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	headers, err := enmime.ReadHeader(bufio.NewReader(bytes.NewReader(raw)), unreported{})
	if err != nil {
		s.backend.log.Warn("the headers of a Message could not be read", "inbox", s.inbox.ID, "error", err)
	}
	message := store.Message{
		ReceivedAt:         s.backend.now(),
		EnvelopeSender:     s.sender,
		EnvelopeRecipients: s.recipients,
		Subject:            enmime.DecodeRFC2047(headers.Get("Subject")),
		Sender:             strings.Join(displayed(headers.Get("From")), ", "),
		Recipients:         append(displayed(headers.Get("To")), displayed(headers.Get("Cc"))...),
	}
	if message.Sender == "" {
		message.Sender = s.sender
	}
	if len(message.Recipients) == 0 {
		message.Recipients = s.recipients
	}
	if err := s.backend.store.HoldMessage(context.Background(), s.inbox.ID, message, raw); err != nil {
		s.backend.log.Error("holding a Message failed", "inbox", s.inbox.ID, "error", err)
		return errUnavailable
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
}

func (s *session) Logout() error {
	return nil
}

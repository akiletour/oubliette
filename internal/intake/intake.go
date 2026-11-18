package intake

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"oubliette/internal/store"
)

const (
	maxLineLength = 1 << 20
	previewSource = 256 << 10
	previewDepth  = 3
	previewParts  = 20
	previewText   = 4096
	previewLength = 200
)

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
	source := bufio.NewReader(bytes.NewReader(raw))
	headers, err := enmime.ReadHeader(source, unreported{})
	if err != nil {
		s.backend.log.Warn("the headers of a Message could not be read", "inbox", s.inbox.ID, "error", err)
	}
	message := store.Message{
		ReceivedAt:         s.backend.now(),
		EnvelopeSender:     s.sender,
		EnvelopeRecipients: s.recipients,
		Subject:            enmime.DecodeRFC2047(headers.Get("Subject")),
		Preview:            preview(headers, source),
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

var (
	invisible = map[string]bool{"head": true, "title": true, "style": true, "script": true, "noscript": true, "template": true, "svg": true}
	inline    = map[string]bool{
		"a": true, "abbr": true, "b": true, "code": true, "em": true, "font": true, "i": true,
		"small": true, "span": true, "strong": true, "sub": true, "sup": true, "u": true,
	}
)

func preview(header textproto.MIMEHeader, body io.Reader) string {
	plain, markup := bodyText(header, io.LimitReader(body, previewSource), previewDepth)
	words := strings.Fields(plain)
	if len(words) == 0 {
		words = strings.Fields(visibleText(markup))
	}
	beginning := []rune(strings.Join(words, " "))
	return string(beginning[:min(len(beginning), previewLength)])
}

func bodyText(header textproto.MIMEHeader, body io.Reader, depth int) (plain, markup string) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		mediaType = "text/plain"
	}
	attached := strings.HasPrefix(strings.ToLower(header.Get("Content-Disposition")), "attachment")
	switch {
	case attached:
	case strings.HasPrefix(mediaType, "multipart/") && depth > 0:
		parts := multipart.NewReader(body, params["boundary"])
		for range previewParts {
			part, err := parts.NextRawPart()
			if err != nil {
				break
			}
			partPlain, partMarkup := bodyText(part.Header, part, depth-1)
			plain, markup = cmp.Or(plain, partPlain), cmp.Or(markup, partMarkup)
			if plain != "" {
				break
			}
		}
	case mediaType == "text/plain":
		plain = strings.TrimSpace(decodedText(header, params["charset"], body))
	case mediaType == "text/html":
		markup = decodedText(header, params["charset"], body)
	}
	return plain, markup
}

func decodedText(header textproto.MIMEHeader, label string, body io.Reader) string {
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		body = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		body = quotedprintable.NewReader(body)
	}
	if converted, err := charset.NewReaderLabel(label, body); err == nil {
		body = converted
	}
	text, _ := io.ReadAll(io.LimitReader(body, previewText))
	return strings.ToValidUTF8(string(text), "")
}

func visibleText(document string) string {
	var text strings.Builder
	hidden := 0
	tokens := html.NewTokenizer(strings.NewReader(document))
	for text.Len() < previewText {
		token := tokens.Next()
		if token == html.ErrorToken {
			break
		}
		if token == html.TextToken {
			if hidden == 0 {
				text.Write(tokens.Text())
			}
			continue
		}
		name, _ := tokens.TagName()
		switch tag := string(name); {
		case invisible[tag] && token == html.StartTagToken:
			hidden++
		case invisible[tag] && token == html.EndTagToken && hidden > 0:
			hidden--
		case !inline[tag]:
			text.WriteByte(' ')
		}
	}
	return text.String()
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

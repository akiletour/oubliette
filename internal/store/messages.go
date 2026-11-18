package store

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const (
	pageSize        = 50
	maxName         = 100
	zeroWidthJoiner = '‍'
)

var (
	ErrPosition     = errors.New("malformed position in the Message list")
	ErrInboxName    = errors.New("an Inbox name needs 1 to 100 visible characters and no control character")
	ErrInboxUnknown = errors.New("no such Inbox")
)

var unknownInboxPassword = rand.Text()

type Inbox struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MessageCount int    `json:"messageCount"`
	Username     string `json:"-"`
	Password     string `json:"-"`
}

type Message struct {
	ID                 string    `json:"id"`
	ReceivedAt         time.Time `json:"receivedAt"`
	EnvelopeSender     string    `json:"envelopeSender"`
	EnvelopeRecipients []string  `json:"envelopeRecipients"`
	Subject            string    `json:"subject"`
	Preview            string    `json:"preview"`
	Sender             string    `json:"sender"`
	Recipients         []string  `json:"recipients"`
	Size               int       `json:"size"`
	Raw                []byte    `json:"-"`
}

func newID() string {
	return strings.ToLower(rand.Text())
}

func cleanName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	shown := strings.IndexFunc(name, func(r rune) bool {
		return unicode.In(r, unicode.L, unicode.N, unicode.P, unicode.S)
	}) >= 0
	hidden := strings.IndexFunc(name, func(r rune) bool {
		return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) && r != zeroWidthJoiner
	}) >= 0
	return name, shown && !hidden && utf8.RuneCountInString(name) <= maxName
}

func inboxName(name string) (string, error) {
	name, ok := cleanName(name)
	if !ok {
		return "", ErrInboxName
	}
	return name, nil
}

func createInbox(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, name string, now time.Time) (Inbox, error) {
	name, err := inboxName(name)
	if err != nil {
		return Inbox{}, err
	}
	inbox := Inbox{ID: newID(), Name: name, Username: newID()[:16], Password: rand.Text()}
	_, err = db.ExecContext(ctx,
		"INSERT INTO inboxes (id, name, smtp_username, smtp_password, created_at) VALUES (?, ?, ?, ?, ?)",
		inbox.ID, inbox.Name, inbox.Username, inbox.Password, now.UnixMilli())
	return inbox, err
}

func (s *Store) CreateInbox(ctx context.Context, name string, now time.Time) (Inbox, error) {
	return createInbox(ctx, s.db, name, now)
}

func (s *Store) CreateFirstInbox(ctx context.Context, name string, now time.Time) (Inbox, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Inbox{}, false, err
	}
	defer tx.Rollback()
	var created bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM first_inbox)").Scan(&created); err != nil || created {
		return Inbox{}, false, err
	}
	inbox, err := createInbox(ctx, tx, name, now)
	if err != nil {
		return Inbox{}, false, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO first_inbox (created_at) VALUES (?)", now.UnixMilli()); err != nil {
		return Inbox{}, false, err
	}
	return inbox, true, tx.Commit()
}

func (s *Store) RenameInbox(ctx context.Context, id, name string) error {
	name, err := inboxName(name)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, "UPDATE inboxes SET name = ? WHERE id = ?", name, id)
	return whenNoRow(result, err, ErrInboxUnknown)
}

func (s *Store) DeleteInbox(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM inboxes WHERE id = ?", id)
	return whenNoRow(result, err, ErrInboxUnknown)
}

func (s *Store) Inboxes(ctx context.Context) ([]Inbox, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, "+messageCount+" FROM inboxes ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	inboxes := []Inbox{}
	for rows.Next() {
		var inbox Inbox
		if err := rows.Scan(&inbox.ID, &inbox.Name, &inbox.MessageCount); err != nil {
			return nil, err
		}
		inboxes = append(inboxes, inbox)
	}
	byName := collate.New(language.Und, collate.IgnoreCase)
	slices.SortStableFunc(inboxes, func(a, b Inbox) int { return byName.CompareString(a.Name, b.Name) })
	return inboxes, rows.Err()
}

func (s *Store) Inbox(ctx context.Context, id string) (Inbox, bool, error) {
	inbox := Inbox{ID: id}
	err := s.db.QueryRowContext(ctx, "SELECT name FROM inboxes WHERE id = ?", id).Scan(&inbox.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Inbox{}, false, nil
	}
	return inbox, err == nil, err
}

func (s *Store) InboxByCredentials(ctx context.Context, username, password string) (Inbox, bool, error) {
	inbox := Inbox{Username: username}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, smtp_password FROM inboxes WHERE smtp_username = ?", username).Scan(&inbox.ID, &inbox.Name, &inbox.Password)
	known := !errors.Is(err, sql.ErrNoRows)
	if known && err != nil {
		return Inbox{}, false, err
	}
	expected := inbox.Password
	if !known {
		expected = unknownInboxPassword
	}
	matches := subtle.ConstantTimeCompare([]byte(password), []byte(expected)) == 1
	if !known || !matches {
		return Inbox{}, false, nil
	}
	return inbox, true, nil
}

func encodeList(list []string) string {
	encoded, _ := json.Marshal(list)
	return string(encoded)
}

func (s *Store) HoldMessage(ctx context.Context, inboxID string, message Message) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages
			(id, inbox_id, received_at, envelope_sender, envelope_recipients, subject, preview, sender, recipients, size, raw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), inboxID, message.ReceivedAt.UnixMicro(), message.EnvelopeSender, encodeList(message.EnvelopeRecipients),
		message.Subject, message.Preview, message.Sender, encodeList(message.Recipients), len(message.Raw), message.Raw)
	return err
}

const messageCount = "(SELECT COUNT(*) FROM messages WHERE inbox_id = inboxes.id)"

const messageColumns = "id, received_at, envelope_sender, envelope_recipients, subject, preview, sender, recipients, size"

func scanMessage(row interface{ Scan(...any) error }, more ...any) (Message, error) {
	var message Message
	var receivedAt int64
	var envelopeRecipients, recipients string
	columns := []any{&message.ID, &receivedAt, &message.EnvelopeSender, &envelopeRecipients,
		&message.Subject, &message.Preview, &message.Sender, &recipients, &message.Size}
	if err := row.Scan(append(columns, more...)...); err != nil {
		return Message{}, err
	}
	message.ReceivedAt = time.UnixMicro(receivedAt).UTC()
	return message, errors.Join(
		json.Unmarshal([]byte(envelopeRecipients), &message.EnvelopeRecipients),
		json.Unmarshal([]byte(recipients), &message.Recipients),
	)
}

func (s *Store) Message(ctx context.Context, inboxID, id string) (Message, bool, error) {
	var raw []byte
	message, err := scanMessage(s.db.QueryRowContext(ctx,
		"SELECT "+messageColumns+", raw FROM messages WHERE inbox_id = ? AND id = ?", inboxID, id), &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, false, nil
	}
	message.Raw = raw
	return message, err == nil, err
}

func (s *Store) Messages(ctx context.Context, inboxID, before string) (page []Message, next string, err error) {
	var beforeTime int64
	var beforeID string
	if before != "" {
		at, id, found := strings.Cut(before, "-")
		beforeTime, err = strconv.ParseInt(at, 10, 64)
		if err != nil || !found {
			return nil, "", ErrPosition
		}
		beforeID = id
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+messageColumns+` FROM messages
		WHERE inbox_id = ?1 AND (?2 = '' OR (received_at, id) < (?3, ?4))
		ORDER BY received_at DESC, id DESC
		LIMIT ?5`, inboxID, before, beforeTime, beforeID, pageSize+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	page = []Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, "", err
		}
		page = append(page, message)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(page) > pageSize {
		page = page[:pageSize]
		last := page[pageSize-1]
		next = strconv.FormatInt(last.ReceivedAt.UnixMicro(), 10) + "-" + last.ID
	}
	return page, next, nil
}

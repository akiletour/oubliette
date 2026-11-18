package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"
)

var ErrShareLinkLabel = errors.New("a Share Link label needs 1 to 100 visible characters and no control character")

type ShareLink struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) CreateShareLink(ctx context.Context, inboxID, label string, now time.Time) (ShareLink, error) {
	label, ok := cleanName(label)
	if !ok {
		return ShareLink{}, ErrShareLinkLabel
	}
	link := ShareLink{ID: newID(), Label: label, Token: rand.Text(), CreatedAt: now.UTC()}
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO share_links (id, inbox_id, token, label, created_at) SELECT ?, id, ?, ?, ? FROM inboxes WHERE id = ?",
		link.ID, link.Token, link.Label, now.UnixMilli(), inboxID)
	return link, whenNoRow(result, err, ErrInboxUnknown)
}

func (s *Store) SharedInbox(ctx context.Context, token string) (Inbox, bool, error) {
	var inbox Inbox
	err := s.db.QueryRowContext(ctx,
		`SELECT inboxes.id, inboxes.name, `+messageCount+`
		FROM share_links JOIN inboxes ON inboxes.id = share_links.inbox_id
		WHERE share_links.token = ?`, token).Scan(&inbox.ID, &inbox.Name, &inbox.MessageCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Inbox{}, false, nil
	}
	return inbox, err == nil, err
}

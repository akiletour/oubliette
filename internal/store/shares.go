package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"
)

const openingPrecision = 10 * time.Minute

var (
	ErrShareLinkLabel   = errors.New("a Share Link label needs 1 to 100 visible characters and no control character")
	ErrShareLinkExpiry  = errors.New("a Share Link expiry must be in the future")
	ErrShareLinkUnknown = errors.New("no such Share Link")
)

type ShareLink struct {
	ID           string     `json:"id"`
	Label        string     `json:"label"`
	Token        string     `json:"token"`
	CreatedAt    time.Time  `json:"createdAt"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	LastOpenedAt *time.Time `json:"lastOpenedAt"`
	Expired      bool       `json:"expired"`
}

func fromMillis(stored int64) time.Time {
	return time.UnixMilli(stored).UTC()
}

func optionalToMillis(at *time.Time) any {
	if at == nil {
		return nil
	}
	return at.UnixMilli()
}

func optionalFromMillis(stored sql.NullInt64) *time.Time {
	if !stored.Valid {
		return nil
	}
	at := fromMillis(stored.Int64)
	return &at
}

func (s *Store) CreateShareLink(ctx context.Context, inboxID, label string, expiresAt *time.Time, now time.Time) (ShareLink, error) {
	label, ok := cleanName(label)
	if !ok {
		return ShareLink{}, ErrShareLinkLabel
	}
	if expiresAt != nil && !expiresAt.After(now) {
		return ShareLink{}, ErrShareLinkExpiry
	}
	link := ShareLink{ID: newID(), Label: label, Token: rand.Text(), CreatedAt: fromMillis(now.UnixMilli())}
	if expiresAt != nil {
		at := fromMillis(expiresAt.UnixMilli())
		link.ExpiresAt = &at
	}
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO share_links (id, inbox_id, token, label, created_at, expires_at)
		SELECT ?, id, ?, ?, ?, ? FROM inboxes WHERE id = ?`,
		link.ID, link.Token, link.Label, now.UnixMilli(), optionalToMillis(expiresAt), inboxID)
	return link, whenNoRow(result, err, ErrInboxUnknown)
}

func (s *Store) ShareLinks(ctx context.Context, inboxID string, now time.Time) ([]ShareLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, label, token, created_at, expires_at, last_opened_at FROM share_links
		WHERE inbox_id = ? AND revoked_at IS NULL ORDER BY created_at, id`, inboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []ShareLink{}
	for rows.Next() {
		var link ShareLink
		var created int64
		var expires, opened sql.NullInt64
		if err := rows.Scan(&link.ID, &link.Label, &link.Token, &created, &expires, &opened); err != nil {
			return nil, err
		}
		link.CreatedAt, link.ExpiresAt, link.LastOpenedAt = fromMillis(created), optionalFromMillis(expires), optionalFromMillis(opened)
		link.Expired = link.ExpiresAt != nil && !link.ExpiresAt.After(now)
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) RevokeShareLink(ctx context.Context, inboxID, id string, now time.Time) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE share_links SET revoked_at = ? WHERE id = ? AND inbox_id = ? AND revoked_at IS NULL", now.UnixMilli(), id, inboxID)
	return whenNoRow(result, err, ErrShareLinkUnknown)
}

func (s *Store) OpenShareLink(ctx context.Context, token string, now time.Time) (Inbox, bool, error) {
	var inbox Inbox
	var link string
	var opened sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT share_links.id, share_links.last_opened_at, inboxes.id, inboxes.name, `+messageCount+`
		FROM share_links JOIN inboxes ON inboxes.id = share_links.inbox_id
		WHERE share_links.token = ?1 AND share_links.revoked_at IS NULL
			AND (share_links.expires_at IS NULL OR share_links.expires_at > ?2)`,
		token, now.UnixMilli()).Scan(&link, &opened, &inbox.ID, &inbox.Name, &inbox.MessageCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Inbox{}, false, nil
	}
	if err != nil {
		return Inbox{}, false, err
	}
	if last := optionalFromMillis(opened); last == nil || now.Sub(*last) >= openingPrecision {
		if _, err := s.db.ExecContext(ctx, "UPDATE share_links SET last_opened_at = ? WHERE id = ?", now.UnixMilli(), link); err != nil {
			return Inbox{}, false, err
		}
	}
	return inbox, true, nil
}

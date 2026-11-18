package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/mail"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrEmailInvalid   = errors.New("invalid email address")
	ErrPasswordLength = errors.New("the password must be between 8 and 72 bytes long")
	ErrMemberExists   = errors.New("a Member with this email already exists")
	ErrMemberUnknown  = errors.New("no Member with this email")
)

const passwordCost = 12

type Store struct {
	db *sql.DB
}

type Member struct {
	id    int64
	Email string
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open data file %s: %w", path, err)
	}
	go unknownMemberHash()
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(files) {
		return errors.New("the data file was written by a newer version of oubliette")
	}
	for ; version < len(files); version++ {
		script, err := migrations.ReadFile(files[version])
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(script)); err != nil {
			return fmt.Errorf("%s: %w", files[version], err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 || len(password) > 72 {
		return "", ErrPasswordLength
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	return string(hash), err
}

var unknownMemberHash = sync.OnceValue(func() string {
	hash, _ := hashPassword(rand.Text())
	return hash
})

func (s *Store) CheckPassword(ctx context.Context, email, password string) (Member, bool, error) {
	member := Member{Email: NormalizeEmail(email)}
	var hash string
	err := s.db.QueryRowContext(ctx, "SELECT id, password_hash FROM members WHERE email = ?", member.Email).Scan(&member.id, &hash)
	known := !errors.Is(err, sql.ErrNoRows)
	if !known {
		hash, err = unknownMemberHash(), nil
	}
	if err != nil {
		return Member{}, false, err
	}
	matches := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	return member, known && matches, nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (s *Store) StartSession(ctx context.Context, member Member, now, expires time.Time) (string, error) {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", now.Unix()); err != nil {
		return "", err
	}
	token := rand.Text()
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, member_id, expires_at) VALUES (?, ?, ?)", hashToken(token), member.id, expires.Unix())
	return token, err
}

func (s *Store) SessionMember(ctx context.Context, token string, now time.Time) (Member, bool, error) {
	var member Member
	err := s.db.QueryRowContext(ctx,
		`SELECT members.id, members.email FROM sessions JOIN members ON members.id = sessions.member_id
		WHERE sessions.token_hash = ? AND sessions.expires_at > ?`, hashToken(token), now.Unix()).Scan(&member.id, &member.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, false, nil
	}
	return member, err == nil, err
}

func (s *Store) EndSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", hashToken(token))
	return err
}

func (s *Store) AddMember(ctx context.Context, email, password string) error {
	email = NormalizeEmail(email)
	if parsed, err := mail.ParseAddress(email); err != nil || parsed.Address != email {
		return ErrEmailInvalid
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO members (email, password_hash) VALUES (?, ?) ON CONFLICT (email) DO NOTHING", email, hash)
	return whenNoRow(result, err, ErrMemberExists)
}

func whenNoRow(result sql.Result, err, none error) error {
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed > 0 {
		return err
	}
	return none
}

func (s *Store) RemoveMember(ctx context.Context, email string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM members WHERE email = ?", NormalizeEmail(email))
	return whenNoRow(result, err, ErrMemberUnknown)
}

func (s *Store) SetMemberPassword(ctx context.Context, email, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx,
		"UPDATE members SET password_hash = ? WHERE email = ? RETURNING id", hash, NormalizeEmail(email)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMemberUnknown
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE member_id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Members(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT email FROM members ORDER BY email")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

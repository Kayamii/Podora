// Package store keeps Podora's own data (users, sessions, the audit log) in
// SQLite. It uses a pure-Go driver so the binary stays static.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned when a username is already taken.
	ErrConflict = errors.New("already exists")
)

type Store struct {
	db *sql.DB
}

var memCounter atomic.Int64

// Open opens (creating if needed) the database in dir. An empty dir keeps
// everything in memory, which tests use.
func Open(dir string) (*Store, error) {
	// Each in-memory store gets its own name, so tests do not share data.
	dsn := fmt.Sprintf("file:mem%d?mode=memory&cache=shared", memCounter.Add(1))
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("creating data directory: %w", err)
		}
		dsn = "file:" + filepath.ToSlash(filepath.Join(dir, "podora.db"))
	}
	// WAL lets reads continue during a write; busy_timeout waits out brief
	// locks instead of failing; foreign keys make session cleanup automatic.
	dsn += sep(dsn) + "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// SQLite allows one writer at a time; a single connection avoids
	// "database is locked" errors under concurrent requests.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func sep(dsn string) string {
	if strings.Contains(dsn, "?") {
		return "&"
	}
	return "?"
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL,
	namespaces    TEXT NOT NULL DEFAULT '[]',
	disabled      INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL,
	last_login    INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS audit (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	time      INTEGER NOT NULL,
	username  TEXT NOT NULL,
	action    TEXT NOT NULL,
	namespace TEXT NOT NULL DEFAULT '',
	target    TEXT NOT NULL DEFAULT '',
	detail    TEXT NOT NULL DEFAULT '',
	success   INTEGER NOT NULL,
	error     TEXT NOT NULL DEFAULT '',
	remote    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS audit_time ON audit(time);
`)
	if err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	return nil
}

// --- Users ----------------------------------------------------------------------

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	// Namespaces limits the user to these namespaces; empty means all.
	Namespaces []string
	Disabled   bool
	CreatedAt  time.Time
	LastLogin  time.Time
}

const userColumns = `id, username, password_hash, role, namespaces, disabled, created_at, last_login`

type scanner interface{ Scan(dest ...any) error }

func scanUser(row scanner) (*User, error) {
	var (
		u                  User
		namespaces         string
		disabled           int
		created, lastLogin int64
	)
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &namespaces, &disabled, &created, &lastLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(namespaces), &u.Namespaces); err != nil {
		return nil, fmt.Errorf("reading namespaces of %s: %w", u.Username, err)
	}
	if u.Namespaces == nil {
		u.Namespaces = []string{}
	}
	u.Disabled = disabled != 0
	u.CreatedAt = time.Unix(created, 0).UTC()
	if lastLogin > 0 {
		u.LastLogin = time.Unix(lastLogin, 0).UTC()
	}
	return &u, nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, u *User) error {
	ns, _ := json.Marshal(nonNil(u.Namespaces))
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role, namespaces, disabled, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.Role, string(ns), boolInt(u.Disabled), time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrConflict
		}
		return err
	}
	u.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UserByName(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username))
}

func (s *Store) User(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateUser saves role, namespaces, disabled and password hash.
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	ns, _ := json.Marshal(nonNil(u.Namespaces))
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, role = ?, namespaces = ?, disabled = ? WHERE id = ?`,
		u.PasswordHash, u.Role, string(ns), boolInt(u.Disabled), u.ID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

func (s *Store) RecordLogin(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

// CountActiveAdmins counts enabled admins, so the last one cannot be removed.
func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0`).Scan(&n)
	return n, err
}

// --- Sessions -------------------------------------------------------------------

type Session struct {
	TokenHash string
	UserID    int64
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, last_seen, expires_at) VALUES (?, ?, ?, ?, ?)`,
		sess.TokenHash, sess.UserID, sess.CreatedAt.Unix(), sess.LastSeen.Unix(), sess.ExpiresAt.Unix())
	return err
}

func (s *Store) Session(ctx context.Context, tokenHash string) (*Session, error) {
	var (
		sess                     Session
		created, seen, expiresAt int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT token_hash, user_id, created_at, last_seen, expires_at FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&sess.TokenHash, &sess.UserID, &created, &seen, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, sess.LastSeen, sess.ExpiresAt = time.Unix(created, 0), time.Unix(seen, 0), time.Unix(expiresAt, 0)
	return &sess, nil
}

func (s *Store) TouchSession(ctx context.Context, tokenHash string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen = ? WHERE token_hash = ?`, at.Unix(), tokenHash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteUserSessions signs a user out everywhere, except the session kept.
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64, keepTokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, keepTokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, idleBefore time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ? OR last_seen < ?`, time.Now().Unix(), idleBefore.Unix())
	return err
}

// --- Audit ----------------------------------------------------------------------

type AuditEntry struct {
	ID        int64     `json:"id"`
	Time      time.Time `json:"time"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Namespace string    `json:"namespace"`
	Target    string    `json:"target"`
	Detail    string    `json:"detail"`
	Success   bool      `json:"success"`
	Error     string    `json:"error"`
	Remote    string    `json:"remote"`
}

func (s *Store) AddAudit(ctx context.Context, e AuditEntry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit (time, username, action, namespace, target, detail, success, error, remote) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Time.UnixMilli(), e.Username, e.Action, e.Namespace, e.Target, e.Detail, boolInt(e.Success), e.Error, e.Remote)
	return err
}

// Audit returns the most recent entries, newest first.
func (s *Store) Audit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, time, username, action, namespace, target, detail, success, error, remote FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var (
			e       AuditEntry
			ms      int64
			success int
		)
		if err := rows.Scan(&e.ID, &ms, &e.Username, &e.Action, &e.Namespace, &e.Target, &e.Detail, &success, &e.Error, &e.Remote); err != nil {
			return nil, err
		}
		e.Time = time.UnixMilli(ms).UTC()
		e.Success = success != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneAudit deletes entries older than the cutoff.
func (s *Store) PruneAudit(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit WHERE time < ?`, before.UnixMilli())
	return err
}

// --- Helpers --------------------------------------------------------------------

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func mustAffect(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

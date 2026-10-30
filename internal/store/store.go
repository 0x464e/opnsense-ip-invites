package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("invite or grant not found")
	ErrExpired  = errors.New("this invite has expired")
	ErrRevoked  = errors.New("this invite has been revoked")
	ErrCapacity = errors.New("this invite has reached its active IP limit")
)

type Link struct {
	ID            string `json:"id"`
	ListID        string `json:"list_id"`
	Label         string `json:"label"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	AccessSeconds int64  `json:"access_seconds"`
	MaxIPs        int    `json:"max_ips"`
	RevokedAt     int64  `json:"revoked_at"`
	ActiveIPs     int    `json:"active_ips"`
}
type Grant struct {
	ID                 string `json:"id"`
	LinkID             string `json:"link_id"`
	ListID             string `json:"list_id"`
	Label              string `json:"label"`
	IP                 string `json:"ip"`
	AuthorizedAt       int64  `json:"authorized_at"`
	ExpiresAt          int64  `json:"expires_at"`
	RevokedAt          int64  `json:"revoked_at"`
	EffectiveExpiresAt int64  `json:"effective_expires_at"`
}
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}
type Store struct {
	db   *sql.DB
	lock *os.File
}

func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(abs+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("database is already owned by another application instance")
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		lock.Close()
		return nil, err
	}
	f.Close()
	if err = os.Chmod(abs, 0600); err != nil {
		lock.Close()
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	for _, p := range []string{"foreign_keys(1)", "busy_timeout(5000)", "journal_mode(WAL)"} {
		q.Add("_pragma", p)
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, lock: lock}
	if err = s.migrate(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { err := s.db.Close(); s.lock.Close(); return err }

func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 2 {
		return errors.New("database was created by a newer application version")
	}
	if version == 0 {
		_, err = tx.Exec(`
CREATE TABLE links (
 id TEXT PRIMARY KEY, token_hash TEXT NOT NULL UNIQUE, list_id TEXT NOT NULL,
 label TEXT NOT NULL, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 access_seconds INTEGER NOT NULL CHECK(access_seconds>0), max_ips INTEGER NOT NULL CHECK(max_ips>=0), revoked_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE grants (
 id TEXT PRIMARY KEY, link_id TEXT NOT NULL REFERENCES links(id), ip TEXT NOT NULL,
 authorized_at INTEGER NOT NULL, expires_at INTEGER NOT NULL, revoked_at INTEGER NOT NULL DEFAULT 0,
 UNIQUE(link_id,ip)
);
CREATE INDEX grants_active ON grants(expires_at,revoked_at);
CREATE INDEX links_list ON links(list_id);
CREATE TABLE managed_aliases (alias TEXT PRIMARY KEY);
PRAGMA user_version=1;`)
		if err != nil {
			return err
		}
	}
	if version < 2 {
		if _, err = tx.Exec(`CREATE TABLE access_lists (
          id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL,
          alias TEXT NOT NULL UNIQUE, deleted_at INTEGER NOT NULL DEFAULT 0
        ); PRAGMA user_version=2;`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func randomID(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Store) RegisterAliases(ctx context.Context, aliases []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, a := range aliases {
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO managed_aliases(alias) VALUES(?)", a); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) Aliases(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT alias FROM managed_aliases ORDER BY alias")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	a := []string{}
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			return nil, err
		}
		a = append(a, v)
	}
	return a, rows.Err()
}

func (s *Store) CreateLink(ctx context.Context, l Link, now time.Time) (Link, string, error) {
	l.ID = randomID(12)
	l.CreatedAt = now.Unix()
	token := randomID(32)
	_, err := s.db.ExecContext(ctx, `INSERT INTO links(id,token_hash,list_id,label,created_at,expires_at,access_seconds,max_ips) VALUES(?,?,?,?,?,?,?,?)`, l.ID, tokenHash(token), l.ListID, l.Label, l.CreatedAt, l.ExpiresAt, l.AccessSeconds, l.MaxIPs)
	return l, token, err
}

const linkCols = "id,list_id,label,created_at,expires_at,access_seconds,max_ips,revoked_at"

type scanner interface{ Scan(...any) error }

func scanLink(row scanner) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.ListID, &l.Label, &l.CreatedAt, &l.ExpiresAt, &l.AccessSeconds, &l.MaxIPs, &l.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return l, err
}
func (s *Store) LookupToken(ctx context.Context, token string) (Link, error) {
	return scanLink(s.db.QueryRowContext(ctx, "SELECT "+linkCols+" FROM links WHERE token_hash=?", tokenHash(token)))
}
func ValidLink(l Link, now time.Time) error {
	if l.RevokedAt != 0 {
		return ErrRevoked
	}
	if l.ExpiresAt <= now.Unix() {
		return ErrExpired
	}
	return nil
}

// Authorize is transactional, but callers must also serialize it with firewall reconciliation.
func (s *Store) Authorize(ctx context.Context, id, ip string, now time.Time) (Grant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Grant{}, err
	}
	defer tx.Rollback()
	l, err := scanLink(tx.QueryRowContext(ctx, "SELECT "+linkCols+" FROM links WHERE id=?", id))
	if err != nil {
		return Grant{}, err
	}
	if err = ValidLink(l, now); err != nil {
		return Grant{}, err
	}
	var exists, count int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(ip=?),0) FROM grants WHERE link_id=? AND revoked_at=0 AND expires_at>?", ip, id, now.Unix()).Scan(&count, &exists); err != nil {
		return Grant{}, err
	}
	if l.MaxIPs == 1 && exists == 0 {
		if _, err = tx.ExecContext(ctx, "UPDATE grants SET revoked_at=? WHERE link_id=? AND revoked_at=0 AND expires_at>?", now.Unix(), id, now.Unix()); err != nil {
			return Grant{}, err
		}
	} else if l.MaxIPs > 1 && exists == 0 && count >= l.MaxIPs {
		return Grant{}, ErrCapacity
	}
	g := Grant{ID: randomID(12), LinkID: id, ListID: l.ListID, Label: l.Label, IP: ip, AuthorizedAt: now.Unix(), ExpiresAt: now.Unix() + l.AccessSeconds}
	err = tx.QueryRowContext(ctx, `INSERT INTO grants(id,link_id,ip,authorized_at,expires_at) VALUES(?,?,?,?,?) ON CONFLICT(link_id,ip) DO UPDATE SET authorized_at=excluded.authorized_at,expires_at=excluded.expires_at,revoked_at=0 RETURNING id`, g.ID, id, ip, g.AuthorizedAt, g.ExpiresAt).Scan(&g.ID)
	if err != nil {
		return Grant{}, err
	}
	err = tx.QueryRowContext(ctx, `SELECT MAX(g.expires_at) FROM grants g JOIN links l ON l.id=g.link_id WHERE l.list_id=? AND g.ip=? AND g.revoked_at=0 AND g.expires_at>?`, l.ListID, ip, now.Unix()).Scan(&g.EffectiveExpiresAt)
	if err != nil {
		return Grant{}, err
	}
	return g, tx.Commit()
}

func (s *Store) RevokeLink(ctx context.Context, id string, grants bool, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, "UPDATE links SET revoked_at=CASE WHEN revoked_at=0 THEN ? ELSE revoked_at END WHERE id=?", now.Unix(), id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if grants {
		if _, err = tx.ExecContext(ctx, "UPDATE grants SET revoked_at=? WHERE link_id=? AND revoked_at=0", now.Unix(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) RevokeGrant(ctx context.Context, id string, now time.Time) error {
	r, err := s.db.ExecContext(ctx, "UPDATE grants SET revoked_at=CASE WHEN revoked_at=0 THEN ? ELSE revoked_at END WHERE id=?", now.Unix(), id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) Desired(ctx context.Context, listID string, now time.Time) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT g.ip FROM grants g JOIN links l ON l.id=g.link_id WHERE l.list_id=? AND g.revoked_at=0 AND g.expires_at>?`, listID, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var ip string
		if err = rows.Scan(&ip); err != nil {
			return nil, err
		}
		result[ip] = true
	}
	return result, rows.Err()
}

func filter(status string, now int64, alias string) (string, []any) {
	switch status {
	case "active":
		return alias + "revoked_at=0 AND " + alias + "expires_at>?", []any{now}
	case "expired":
		return alias + "revoked_at=0 AND " + alias + "expires_at<=?", []any{now}
	case "revoked":
		return alias + "revoked_at!=0", nil
	default:
		return "1=1", nil
	}
}
func (s *Store) Links(ctx context.Context, status string, page, size int, now time.Time) (Page[Link], error) {
	p := Page[Link]{Items: []Link{}, Page: page, PageSize: size}
	where, args := filter(status, now.Unix(), "")
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM links WHERE "+where, args...).Scan(&p.Total); err != nil {
		return p, err
	}
	query := `SELECT ` + linkCols + `,(SELECT COUNT(*) FROM grants g WHERE g.link_id=links.id AND g.revoked_at=0 AND g.expires_at>?) FROM links WHERE ` + where + ` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`
	qargs := append([]any{now.Unix()}, args...)
	qargs = append(qargs, size, (page-1)*size)
	rows, err := s.db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var l Link
		if err = rows.Scan(&l.ID, &l.ListID, &l.Label, &l.CreatedAt, &l.ExpiresAt, &l.AccessSeconds, &l.MaxIPs, &l.RevokedAt, &l.ActiveIPs); err != nil {
			return p, err
		}
		p.Items = append(p.Items, l)
	}
	return p, rows.Err()
}
func (s *Store) Grants(ctx context.Context, status string, page, size int, now time.Time) (Page[Grant], error) {
	p := Page[Grant]{Items: []Grant{}, Page: page, PageSize: size}
	where, args := filter(status, now.Unix(), "g.")
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM grants g WHERE "+where, args...).Scan(&p.Total); err != nil {
		return p, err
	}
	query := `SELECT g.id,g.link_id,l.list_id,l.label,g.ip,g.authorized_at,g.expires_at,g.revoked_at,COALESCE((SELECT MAX(g2.expires_at) FROM grants g2 JOIN links l2 ON l2.id=g2.link_id WHERE l2.list_id=l.list_id AND g2.ip=g.ip AND g2.revoked_at=0 AND g2.expires_at>?),0) FROM grants g JOIN links l ON l.id=g.link_id WHERE ` + where + ` ORDER BY g.authorized_at DESC,g.id DESC LIMIT ? OFFSET ?`
	qargs := append([]any{now.Unix()}, args...)
	qargs = append(qargs, size, (page-1)*size)
	rows, err := s.db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var g Grant
		if err = rows.Scan(&g.ID, &g.LinkID, &g.ListID, &g.Label, &g.IP, &g.AuthorizedAt, &g.ExpiresAt, &g.RevokedAt, &g.EffectiveExpiresAt); err != nil {
			return p, err
		}
		p.Items = append(p.Items, g)
	}
	return p, rows.Err()
}

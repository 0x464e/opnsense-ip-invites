package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type List struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Alias       string `json:"alias"`
}

var ErrConflict = errors.New("this list ID or alias has already been used")

func (s *Store) Lists(ctx context.Context) ([]List, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,description,alias FROM access_lists WHERE deleted_at=0 ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []List{}
	for rows.Next() {
		var l List
		if err = rows.Scan(&l.ID, &l.Name, &l.Description, &l.Alias); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, rows.Err()
}
func (s *Store) List(ctx context.Context, id string) (List, error) {
	var l List
	err := s.db.QueryRowContext(ctx, "SELECT id,name,description,alias FROM access_lists WHERE id=? AND deleted_at=0", id).Scan(&l.ID, &l.Name, &l.Description, &l.Alias)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}
func (s *Store) CreateList(ctx context.Context, l List) (List, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return l, err
	}
	defer tx.Rollback()
	var used int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM access_lists WHERE id=? OR alias=?", l.ID, l.Alias).Scan(&used); err != nil {
		return l, err
	}
	if used > 0 {
		return l, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO access_lists(id,name,description,alias) VALUES(?,?,?,?)", l.ID, l.Name, l.Description, l.Alias); err != nil {
		return l, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO managed_aliases(alias) VALUES(?)", l.Alias); err != nil {
		return l, err
	}
	return l, tx.Commit()
}
func (s *Store) UpdateList(ctx context.Context, l List) error {
	r, err := s.db.ExecContext(ctx, "UPDATE access_lists SET name=?,description=? WHERE id=? AND deleted_at=0", l.Name, l.Description, l.ID)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) DeleteList(ctx context.Context, id string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, "UPDATE access_lists SET deleted_at=? WHERE id=? AND deleted_at=0", now.Unix(), id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, "UPDATE links SET revoked_at=? WHERE list_id=? AND revoked_at=0", now.Unix(), id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE grants SET revoked_at=? WHERE link_id IN (SELECT id FROM links WHERE list_id=?) AND revoked_at=0", now.Unix(), id); err != nil {
		return err
	}
	return tx.Commit()
}

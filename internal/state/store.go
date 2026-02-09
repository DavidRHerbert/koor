package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

// Entry is a full state entry including its value.
type Entry struct {
	Key         string    `json:"key"`
	Value       []byte    `json:"-"`
	Version     int64     `json:"version"`
	Hash        string    `json:"hash"`
	ContentType string    `json:"content_type"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   string    `json:"updated_by"`
}

// Summary is a state entry without its value, used for listing.
type Summary struct {
	Key         string    `json:"key"`
	Version     int64     `json:"version"`
	ContentType string    `json:"content_type"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store provides CRUD operations on the state table.
type Store struct {
	db *sql.DB
}

// New creates a new Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// List returns summaries of all state keys (no values).
func (s *Store) List(ctx context.Context) ([]Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, version, content_type, updated_at FROM state ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("query state list: %w", err)
	}
	defer rows.Close()

	var items []Summary
	for rows.Next() {
		var item Summary
		var updatedAt string
		if err := rows.Scan(&item.Key, &item.Version, &item.ContentType, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan state row: %w", err)
		}
		item.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

// Get retrieves a state entry by key. Returns sql.ErrNoRows if not found.
func (s *Store) Get(ctx context.Context, key string) (*Entry, error) {
	var e Entry
	var updatedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT key, value, version, hash, content_type, updated_at, updated_by
		 FROM state WHERE key = ?`, key).
		Scan(&e.Key, &e.Value, &e.Version, &e.Hash, &e.ContentType, &updatedAt, &e.UpdatedBy)
	if err != nil {
		return nil, err
	}
	e.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &e, nil
}

// Put creates or updates a state entry. Version auto-increments on update.
func (s *Store) Put(ctx context.Context, key string, value []byte, contentType, updatedBy string) (*Entry, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(value))

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO state (key, value, version, hash, content_type, updated_at, updated_by)
		 VALUES (?, ?, 1, ?, ?, datetime('now'), ?)
		 ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			version = state.version + 1,
			hash = excluded.hash,
			content_type = excluded.content_type,
			updated_at = datetime('now'),
			updated_by = excluded.updated_by`,
		key, value, hash, contentType, updatedBy)
	if err != nil {
		return nil, fmt.Errorf("upsert state: %w", err)
	}

	return s.Get(ctx, key)
}

// Delete removes a state entry by key.
func (s *Store) Delete(ctx context.Context, key string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM state WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

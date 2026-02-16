package templates

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Template is a reusable bundle of rules, contracts, or both.
type Template struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"` // "rules", "contracts", "bundle"
	Data        []byte    `json:"data"`
	Tags        []string  `json:"tags"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Summary is a Template without the full data payload, for listing.
type Summary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"`
	Tags        []string  `json:"tags"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store provides CRUD operations for templates.
type Store struct {
	db *sql.DB
}

// New creates a new template Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new template.
func (s *Store) Create(ctx context.Context, id, name, description, kind string, data []byte, tags []string) (*Template, error) {
	if kind == "" {
		kind = "rules"
	}
	tagsJSON, _ := json.Marshal(tags)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO templates (id, name, description, kind, data, tags, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 1, datetime('now'), datetime('now'))`,
		id, name, description, kind, data, string(tagsJSON))
	if err != nil {
		return nil, fmt.Errorf("insert template: %w", err)
	}
	return s.Get(ctx, id)
}

// Get retrieves a template by ID.
func (s *Store) Get(ctx context.Context, id string) (*Template, error) {
	var t Template
	var tagsStr, createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, kind, data, tags, version, created_at, updated_at
		 FROM templates WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.Description, &t.Kind, &t.Data, &tagsStr, &t.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(tagsStr), &t.Tags)
	if t.Tags == nil {
		t.Tags = []string{}
	}
	t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	t.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &t, nil
}

// List returns template summaries, optionally filtered by kind and tag.
func (s *Store) List(ctx context.Context, kind, tag string) ([]Summary, error) {
	query := `SELECT id, name, description, kind, tags, version, created_at, updated_at FROM templates WHERE 1=1`
	args := []any{}

	if kind != "" {
		query += ` AND kind = ?`
		args = append(args, kind)
	}
	if tag != "" {
		query += ` AND tags LIKE ?`
		args = append(args, `%"`+tag+`"%`)
	}
	query += ` ORDER BY updated_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var items []Summary
	for rows.Next() {
		var item Summary
		var tagsStr, createdAt, updatedAt string
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Kind, &tagsStr, &item.Version, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		json.Unmarshal([]byte(tagsStr), &item.Tags)
		if item.Tags == nil {
			item.Tags = []string{}
		}
		item.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		item.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

// Delete removes a template by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Apply reads a template's data and returns it for the caller to apply
// to the target project (rules import, contract creation, etc.).
func (s *Store) Apply(ctx context.Context, id string) ([]byte, string, error) {
	t, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return t.Data, t.Kind, nil
}

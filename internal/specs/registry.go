package specs

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

// Spec is a full specification entry including its data.
type Spec struct {
	Project   string    `json:"project"`
	Name      string    `json:"name"`
	Data      []byte    `json:"-"`
	Version   int64     `json:"version"`
	Hash      string    `json:"hash"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Summary is a spec entry without its data, used for listing.
type Summary struct {
	Name      string    `json:"name"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Registry provides CRUD operations on the specs table.
type Registry struct {
	db *sql.DB
}

// New creates a new Registry.
func New(db *sql.DB) *Registry {
	return &Registry{db: db}
}

// List returns summaries of all specs for a project (no data blobs).
func (r *Registry) List(ctx context.Context, project string) ([]Summary, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT name, version, updated_at FROM specs WHERE project = ? ORDER BY name`, project)
	if err != nil {
		return nil, fmt.Errorf("query specs list: %w", err)
	}
	defer rows.Close()

	var items []Summary
	for rows.Next() {
		var item Summary
		var updatedAt string
		if err := rows.Scan(&item.Name, &item.Version, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan specs row: %w", err)
		}
		item.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

// Get retrieves a spec by project and name. Returns sql.ErrNoRows if not found.
func (r *Registry) Get(ctx context.Context, project, name string) (*Spec, error) {
	var s Spec
	var updatedAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT project, name, data, version, hash, updated_at
		 FROM specs WHERE project = ? AND name = ?`, project, name).
		Scan(&s.Project, &s.Name, &s.Data, &s.Version, &s.Hash, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &s, nil
}

// Put creates or updates a spec. Version auto-increments on update.
func (r *Registry) Put(ctx context.Context, project, name string, data []byte) (*Spec, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO specs (project, name, data, version, hash, updated_at)
		 VALUES (?, ?, ?, 1, ?, datetime('now'))
		 ON CONFLICT(project, name) DO UPDATE SET
			data = excluded.data,
			version = specs.version + 1,
			hash = excluded.hash,
			updated_at = datetime('now')`,
		project, name, data, hash)
	if err != nil {
		return nil, fmt.Errorf("upsert spec: %w", err)
	}

	return r.Get(ctx, project, name)
}

// Delete removes a spec by project and name.
func (r *Registry) Delete(ctx context.Context, project, name string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM specs WHERE project = ? AND name = ?`, project, name)
	if err != nil {
		return fmt.Errorf("delete spec: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

package instances

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Instance represents a registered agent instance.
type Instance struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Workspace    string    `json:"workspace"`
	Intent       string    `json:"intent"`
	Stack        string    `json:"stack"`
	Token        string    `json:"token,omitempty"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeen     time.Time `json:"last_seen"`
}

// Summary is an instance without the token, used for listing/discovery.
type Summary struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Workspace    string    `json:"workspace"`
	Intent       string    `json:"intent"`
	Stack        string    `json:"stack"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeen     time.Time `json:"last_seen"`
}

// Registry provides CRUD operations on the instances table.
type Registry struct {
	db *sql.DB
}

// New creates a new instance Registry.
func New(db *sql.DB) *Registry {
	return &Registry{db: db}
}

// Register creates a new instance entry and returns it.
func (r *Registry) Register(ctx context.Context, name, workspace, intent, stack string) (*Instance, error) {
	id := uuid.New().String()
	token := uuid.New().String()

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO instances (id, name, workspace, intent, stack, token, registered_at, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, name, workspace, intent, stack, token)
	if err != nil {
		return nil, fmt.Errorf("register instance: %w", err)
	}

	return r.Get(ctx, id)
}

// Get retrieves an instance by ID. Returns sql.ErrNoRows if not found.
func (r *Registry) Get(ctx context.Context, id string) (*Instance, error) {
	var inst Instance
	var registeredAt, lastSeen string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, workspace, intent, stack, token, registered_at, last_seen
		 FROM instances WHERE id = ?`, id).
		Scan(&inst.ID, &inst.Name, &inst.Workspace, &inst.Intent, &inst.Stack, &inst.Token, &registeredAt, &lastSeen)
	if err != nil {
		return nil, err
	}
	inst.RegisteredAt, _ = time.Parse("2006-01-02 15:04:05", registeredAt)
	inst.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
	return &inst, nil
}

// List returns summaries of all registered instances (no tokens).
func (r *Registry) List(ctx context.Context) ([]Summary, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, workspace, intent, stack, registered_at, last_seen
		 FROM instances ORDER BY last_seen DESC`)
	if err != nil {
		return nil, fmt.Errorf("query instances: %w", err)
	}
	defer rows.Close()

	var items []Summary
	for rows.Next() {
		var item Summary
		var registeredAt, lastSeen string
		if err := rows.Scan(&item.ID, &item.Name, &item.Workspace, &item.Intent, &item.Stack, &registeredAt, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		item.RegisteredAt, _ = time.Parse("2006-01-02 15:04:05", registeredAt)
		item.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
		items = append(items, item)
	}
	return items, rows.Err()
}

// Discover returns instances matching optional name, workspace, and stack filters.
func (r *Registry) Discover(ctx context.Context, name, workspace, stack string) ([]Summary, error) {
	query := `SELECT id, name, workspace, intent, stack, registered_at, last_seen FROM instances WHERE 1=1`
	args := []any{}

	if name != "" {
		query += ` AND name = ?`
		args = append(args, name)
	}
	if workspace != "" {
		query += ` AND workspace = ?`
		args = append(args, workspace)
	}
	if stack != "" {
		query += ` AND stack = ?`
		args = append(args, stack)
	}
	query += ` ORDER BY last_seen DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("discover instances: %w", err)
	}
	defer rows.Close()

	var items []Summary
	for rows.Next() {
		var item Summary
		var registeredAt, lastSeen string
		if err := rows.Scan(&item.ID, &item.Name, &item.Workspace, &item.Intent, &item.Stack, &registeredAt, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		item.RegisteredAt, _ = time.Parse("2006-01-02 15:04:05", registeredAt)
		item.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
		items = append(items, item)
	}
	return items, rows.Err()
}

// SetIntent updates the intent for an instance and refreshes last_seen.
func (r *Registry) SetIntent(ctx context.Context, id, intent string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE instances SET intent = ?, last_seen = datetime('now') WHERE id = ?`,
		intent, id)
	if err != nil {
		return fmt.Errorf("set intent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Heartbeat updates the last_seen timestamp for an instance.
func (r *Registry) Heartbeat(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE instances SET last_seen = datetime('now') WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Deregister removes an instance by ID.
func (r *Registry) Deregister(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deregister instance: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

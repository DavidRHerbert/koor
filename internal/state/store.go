package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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
// Before overwriting, the current value is archived to state_history.
func (s *Store) Put(ctx context.Context, key string, value []byte, contentType, updatedBy string) (*Entry, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(value))

	// Archive current version before overwrite.
	s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO state_history (key, version, value, hash, content_type, updated_at, updated_by)
		 SELECT key, version, value, hash, content_type, updated_at, updated_by
		 FROM state WHERE key = ?`, key)

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

// HistoryEntry is a summary of a historical state version.
type HistoryEntry struct {
	Key         string    `json:"key"`
	Version     int64     `json:"version"`
	Hash        string    `json:"hash"`
	ContentType string    `json:"content_type"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   string    `json:"updated_by"`
}

// History returns version history for a key (most recent first).
// It includes both archived versions and the current version.
func (s *Store) History(ctx context.Context, key string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	// Combine current + archived versions using UNION.
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, version, hash, content_type, updated_at, updated_by FROM state WHERE key = ?
		 UNION ALL
		 SELECT key, version, hash, content_type, updated_at, updated_by FROM state_history WHERE key = ?
		 ORDER BY version DESC LIMIT ?`, key, key, limit)
	if err != nil {
		return nil, fmt.Errorf("query state history: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var updatedAt string
		if err := rows.Scan(&e.Key, &e.Version, &e.Hash, &e.ContentType, &updatedAt, &e.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan state history: %w", err)
		}
		e.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetVersion retrieves a specific version of a state key.
// If the requested version matches the current version, it returns the current entry.
// Otherwise it looks in state_history. Returns sql.ErrNoRows if not found.
func (s *Store) GetVersion(ctx context.Context, key string, version int64) (*Entry, error) {
	// Try current first.
	current, err := s.Get(ctx, key)
	if err == nil && current.Version == version {
		return current, nil
	}

	// Look in history.
	var e Entry
	var updatedAt string
	err = s.db.QueryRowContext(ctx,
		`SELECT key, value, version, hash, content_type, updated_at, updated_by
		 FROM state_history WHERE key = ? AND version = ?`, key, version).
		Scan(&e.Key, &e.Value, &e.Version, &e.Hash, &e.ContentType, &updatedAt, &e.UpdatedBy)
	if err != nil {
		return nil, err
	}
	e.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &e, nil
}

// Rollback restores a key to a previous version. The current value is archived
// first, then the historical version becomes the new current value.
// Returns the new entry. Returns sql.ErrNoRows if version not found.
func (s *Store) Rollback(ctx context.Context, key string, version int64) (*Entry, error) {
	old, err := s.GetVersion(ctx, key, version)
	if err != nil {
		return nil, err
	}
	return s.Put(ctx, key, old.Value, old.ContentType, "rollback:v"+fmt.Sprint(version))
}

// DiffEntry represents a single field difference between two versions.
type DiffEntry struct {
	Path string `json:"path"`
	Old  any    `json:"old,omitempty"`
	New  any    `json:"new,omitempty"`
	Kind string `json:"kind"` // "added", "removed", "changed"
}

// Diff computes a JSON diff between two versions of a key.
// Both versions must exist. Returns a list of differences.
func (s *Store) Diff(ctx context.Context, key string, v1, v2 int64) ([]DiffEntry, error) {
	e1, err := s.GetVersion(ctx, key, v1)
	if err != nil {
		return nil, fmt.Errorf("get version %d: %w", v1, err)
	}
	e2, err := s.GetVersion(ctx, key, v2)
	if err != nil {
		return nil, fmt.Errorf("get version %d: %w", v2, err)
	}

	var m1, m2 map[string]any
	if err := json.Unmarshal(e1.Value, &m1); err != nil {
		return nil, fmt.Errorf("version %d is not a JSON object: %w", v1, err)
	}
	if err := json.Unmarshal(e2.Value, &m2); err != nil {
		return nil, fmt.Errorf("version %d is not a JSON object: %w", v2, err)
	}

	return diffMaps("", m1, m2), nil
}

// diffMaps recursively compares two maps and returns differences.
func diffMaps(prefix string, old, new map[string]any) []DiffEntry {
	var diffs []DiffEntry

	// Collect all keys from both maps.
	keys := map[string]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range new {
		keys[k] = true
	}

	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, k := range sorted {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		oldVal, inOld := old[k]
		newVal, inNew := new[k]

		if !inOld {
			diffs = append(diffs, DiffEntry{Path: path, New: newVal, Kind: "added"})
		} else if !inNew {
			diffs = append(diffs, DiffEntry{Path: path, Old: oldVal, Kind: "removed"})
		} else {
			// Both exist — check for nested objects.
			oldMap, oldIsMap := oldVal.(map[string]any)
			newMap, newIsMap := newVal.(map[string]any)
			if oldIsMap && newIsMap {
				diffs = append(diffs, diffMaps(path, oldMap, newMap)...)
			} else {
				oldJSON, _ := json.Marshal(oldVal)
				newJSON, _ := json.Marshal(newVal)
				if string(oldJSON) != string(newJSON) {
					diffs = append(diffs, DiffEntry{Path: path, Old: oldVal, New: newVal, Kind: "changed"})
				}
			}
		}
	}
	return diffs
}

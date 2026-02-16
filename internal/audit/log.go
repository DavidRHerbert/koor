package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Entry is a single audit log record.
type Entry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Detail    string    `json:"detail"`
	Outcome   string    `json:"outcome"`
}

// Summary is an aggregated view of audit activity.
type Summary struct {
	TotalEntries   int            `json:"total_entries"`
	ActionCounts   map[string]int `json:"action_counts"`
	OutcomeCounts  map[string]int `json:"outcome_counts"`
	UniqueActors   int            `json:"unique_actors"`
	UniqueResources int           `json:"unique_resources"`
}

// Log provides append-only audit logging backed by SQLite.
type Log struct {
	db *sql.DB
}

// New creates a new audit Log.
func New(db *sql.DB) *Log {
	return &Log{db: db}
}

// Append writes a single audit entry. Detail should be a JSON string.
func (l *Log) Append(ctx context.Context, actor, action, resource, detail, outcome string) error {
	if detail == "" {
		detail = "{}"
	}
	if outcome == "" {
		outcome = "success"
	}
	_, err := l.db.ExecContext(ctx,
		`INSERT INTO audit_log (actor, action, resource, detail, outcome)
		 VALUES (?, ?, ?, ?, ?)`,
		actor, action, resource, detail, outcome)
	if err != nil {
		return fmt.Errorf("audit append: %w", err)
	}
	return nil
}

// Query returns audit entries matching the given filters.
// All filter parameters are optional (empty string = no filter).
func (l *Log) Query(ctx context.Context, actor, action, from, to string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, timestamp, actor, action, resource, detail, outcome FROM audit_log WHERE 1=1`
	args := []any{}

	if actor != "" {
		query += ` AND actor = ?`
		args = append(args, actor)
	}
	if action != "" {
		query += ` AND action = ?`
		args = append(args, action)
	}
	if from != "" {
		query += ` AND timestamp >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND timestamp <= ?`
		args = append(args, to)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit query: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Actor, &e.Action, &e.Resource, &e.Detail, &e.Outcome); err != nil {
			return nil, fmt.Errorf("audit scan: %w", err)
		}
		e.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// QuerySummary returns aggregated audit statistics for the given time range.
func (l *Log) QuerySummary(ctx context.Context, from, to string) (*Summary, error) {
	baseWhere := ` WHERE 1=1`
	args := []any{}
	if from != "" {
		baseWhere += ` AND timestamp >= ?`
		args = append(args, from)
	}
	if to != "" {
		baseWhere += ` AND timestamp <= ?`
		args = append(args, to)
	}

	s := &Summary{
		ActionCounts:  map[string]int{},
		OutcomeCounts: map[string]int{},
	}

	// Total entries.
	var total int
	err := l.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log`+baseWhere, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("audit summary count: %w", err)
	}
	s.TotalEntries = total

	// Action counts.
	rows, err := l.db.QueryContext(ctx,
		`SELECT action, COUNT(*) FROM audit_log`+baseWhere+` GROUP BY action`, args...)
	if err != nil {
		return nil, fmt.Errorf("audit summary actions: %w", err)
	}
	for rows.Next() {
		var action string
		var count int
		rows.Scan(&action, &count)
		s.ActionCounts[action] = count
	}
	rows.Close()

	// Outcome counts.
	rows, err = l.db.QueryContext(ctx,
		`SELECT outcome, COUNT(*) FROM audit_log`+baseWhere+` GROUP BY outcome`, args...)
	if err != nil {
		return nil, fmt.Errorf("audit summary outcomes: %w", err)
	}
	for rows.Next() {
		var outcome string
		var count int
		rows.Scan(&outcome, &count)
		s.OutcomeCounts[outcome] = count
	}
	rows.Close()

	// Unique actors.
	l.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT actor) FROM audit_log`+baseWhere, args...).Scan(&s.UniqueActors)

	// Unique resources.
	l.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT resource) FROM audit_log`+baseWhere, args...).Scan(&s.UniqueResources)

	return s, nil
}

// DetailJSON is a helper to create a JSON detail string from a map.
func DetailJSON(m map[string]any) string {
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}

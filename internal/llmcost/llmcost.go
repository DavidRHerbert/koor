package llmcost

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func parseTime(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// UsageRecord is a single LLM API call record.
type UsageRecord struct {
	ID          int64     `json:"id"`
	InstanceID  string    `json:"instance_id"`
	Project     string    `json:"project"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	TokensIn    int64     `json:"tokens_in"`
	TokensOut   int64     `json:"tokens_out"`
	CostUSD     float64   `json:"cost_usd"`
	RequestType string    `json:"request_type"`
	SessionTag  string    `json:"session_tag"`
	CreatedAt   time.Time `json:"created_at"`
}

// UsageSummary is an aggregated view of LLM usage.
type UsageSummary struct {
	TotalTokensIn  int64   `json:"total_tokens_in"`
	TotalTokensOut int64   `json:"total_tokens_out"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	RequestCount   int64   `json:"request_count"`
}

// Store provides LLM usage recording and querying.
type Store struct {
	db *sql.DB
}

// New creates a new llmcost Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Record writes a single LLM usage entry and returns the inserted ID.
func (s *Store) Record(ctx context.Context, rec UsageRecord) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO llm_usage (instance_id, project, provider, model, tokens_in, tokens_out, cost_usd, request_type, session_tag)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.InstanceID, rec.Project, rec.Provider, rec.Model,
		rec.TokensIn, rec.TokensOut, rec.CostUSD, rec.RequestType, rec.SessionTag)
	if err != nil {
		return 0, fmt.Errorf("record llm usage: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// QueryByInstance returns usage records for a specific agent.
func (s *Store) QueryByInstance(ctx context.Context, instanceID string, from, to string, limit int) ([]UsageRecord, error) {
	query := `SELECT id, instance_id, project, provider, model, tokens_in, tokens_out, cost_usd, request_type, session_tag, created_at
		FROM llm_usage WHERE instance_id = ?`
	args := []any{instanceID}
	query, args = appendTimeAndLimit(query, args, from, to, limit)
	return s.queryRecords(ctx, query, args)
}

// QueryByProject returns usage records for a specific project.
func (s *Store) QueryByProject(ctx context.Context, project string, from, to string, limit int) ([]UsageRecord, error) {
	query := `SELECT id, instance_id, project, provider, model, tokens_in, tokens_out, cost_usd, request_type, session_tag, created_at
		FROM llm_usage WHERE project = ?`
	args := []any{project}
	query, args = appendTimeAndLimit(query, args, from, to, limit)
	return s.queryRecords(ctx, query, args)
}

// QueryBySessionTag returns usage records for a specific session tag.
func (s *Store) QueryBySessionTag(ctx context.Context, sessionTag string, from, to string, limit int) ([]UsageRecord, error) {
	query := `SELECT id, instance_id, project, provider, model, tokens_in, tokens_out, cost_usd, request_type, session_tag, created_at
		FROM llm_usage WHERE session_tag = ?`
	args := []any{sessionTag}
	query, args = appendTimeAndLimit(query, args, from, to, limit)
	return s.queryRecords(ctx, query, args)
}

// QueryAll returns recent usage records.
func (s *Store) QueryAll(ctx context.Context, from, to string, limit int) ([]UsageRecord, error) {
	query := `SELECT id, instance_id, project, provider, model, tokens_in, tokens_out, cost_usd, request_type, session_tag, created_at
		FROM llm_usage WHERE 1=1`
	var args []any
	query, args = appendTimeAndLimit(query, args, from, to, limit)
	return s.queryRecords(ctx, query, args)
}

// SummarizeByInstance returns aggregated totals per agent for a time range.
func (s *Store) SummarizeByInstance(ctx context.Context, from, to string) (map[string]UsageSummary, error) {
	return s.summarizeBy(ctx, "instance_id", from, to)
}

// SummarizeByProject returns aggregated totals per project for a time range.
func (s *Store) SummarizeByProject(ctx context.Context, from, to string) (map[string]UsageSummary, error) {
	return s.summarizeBy(ctx, "project", from, to)
}

// SummarizeByModel returns aggregated totals per model for a time range.
func (s *Store) SummarizeByModel(ctx context.Context, from, to string) (map[string]UsageSummary, error) {
	return s.summarizeBy(ctx, "model", from, to)
}

// SummarizeBySessionTag returns aggregated totals per session tag.
func (s *Store) SummarizeBySessionTag(ctx context.Context, from, to string) (map[string]UsageSummary, error) {
	return s.summarizeBy(ctx, "session_tag", from, to)
}

// Total returns a single aggregate for all usage in a time range.
func (s *Store) Total(ctx context.Context, from, to string) (*UsageSummary, error) {
	query := `SELECT COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost_usd),0), COUNT(*)
		FROM llm_usage WHERE 1=1`
	var args []any
	if from != "" {
		query += ` AND created_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND created_at <= ?`
		args = append(args, to)
	}

	var sum UsageSummary
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&sum.TotalTokensIn, &sum.TotalTokensOut, &sum.TotalCostUSD, &sum.RequestCount)
	if err != nil {
		return nil, fmt.Errorf("total llm usage: %w", err)
	}
	return &sum, nil
}

// summarizeBy groups usage by the given column and returns aggregated totals.
func (s *Store) summarizeBy(ctx context.Context, groupCol string, from, to string) (map[string]UsageSummary, error) {
	query := fmt.Sprintf(
		`SELECT %s, COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost_usd),0), COUNT(*)
		 FROM llm_usage WHERE 1=1`, groupCol)
	var args []any
	if from != "" {
		query += ` AND created_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND created_at <= ?`
		args = append(args, to)
	}
	query += fmt.Sprintf(` GROUP BY %s ORDER BY %s`, groupCol, groupCol)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("summarize by %s: %w", groupCol, err)
	}
	defer rows.Close()

	result := map[string]UsageSummary{}
	for rows.Next() {
		var key string
		var sum UsageSummary
		if err := rows.Scan(&key, &sum.TotalTokensIn, &sum.TotalTokensOut, &sum.TotalCostUSD, &sum.RequestCount); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		result[key] = sum
	}
	return result, rows.Err()
}

func (s *Store) queryRecords(ctx context.Context, query string, args []any) ([]UsageRecord, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query llm usage: %w", err)
	}
	defer rows.Close()

	var items []UsageRecord
	for rows.Next() {
		var rec UsageRecord
		var ts string
		if err := rows.Scan(&rec.ID, &rec.InstanceID, &rec.Project, &rec.Provider, &rec.Model,
			&rec.TokensIn, &rec.TokensOut, &rec.CostUSD, &rec.RequestType, &rec.SessionTag, &ts); err != nil {
			return nil, fmt.Errorf("scan llm usage: %w", err)
		}
		rec.CreatedAt = parseTime(ts)
		items = append(items, rec)
	}
	return items, rows.Err()
}

func appendTimeAndLimit(query string, args []any, from, to string, limit int) (string, []any) {
	if from != "" {
		query += ` AND created_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND created_at <= ?`
		args = append(args, to)
	}
	query += ` ORDER BY created_at DESC`
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(` LIMIT %d`, limit)
	return query, args
}

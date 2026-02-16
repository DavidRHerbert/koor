package observability

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AgentMetric is a single metric record for an agent in a time period.
type AgentMetric struct {
	InstanceID  string `json:"instance_id"`
	MetricName  string `json:"metric_name"`
	MetricValue int64  `json:"metric_value"`
	Period      string `json:"period"`
}

// AgentSummary is an aggregate view of all metrics for one agent.
type AgentSummary struct {
	InstanceID string         `json:"instance_id"`
	Metrics    map[string]int64 `json:"metrics"`
}

// Store provides per-agent metric aggregation in hourly buckets.
type Store struct {
	db *sql.DB
}

// New creates a new observability Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// currentPeriod returns the current hourly bucket as "2006-01-02T15".
func currentPeriod() string {
	return time.Now().UTC().Format("2006-01-02T15")
}

// Increment adds 1 to the named metric for the given instance in the current hourly bucket.
func (s *Store) Increment(ctx context.Context, instanceID, metricName string) error {
	return s.IncrementBy(ctx, instanceID, metricName, 1)
}

// IncrementBy adds delta to the named metric for the given instance in the current hourly bucket.
func (s *Store) IncrementBy(ctx context.Context, instanceID, metricName string, delta int64) error {
	period := currentPeriod()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_metrics (instance_id, metric_name, metric_value, period)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (instance_id, metric_name, period)
		 DO UPDATE SET metric_value = agent_metrics.metric_value + ?`,
		instanceID, metricName, delta, period, delta)
	if err != nil {
		return fmt.Errorf("increment metric: %w", err)
	}
	return nil
}

// QueryAgent returns all metrics for a specific agent, optionally filtered by period prefix.
// If period is empty, returns all periods. If period is e.g. "2026-02-16", returns all hours that day.
func (s *Store) QueryAgent(ctx context.Context, instanceID, period string) ([]AgentMetric, error) {
	query := `SELECT instance_id, metric_name, metric_value, period FROM agent_metrics WHERE instance_id = ?`
	args := []any{instanceID}

	if period != "" {
		query += ` AND period LIKE ?`
		args = append(args, period+"%")
	}
	query += ` ORDER BY period DESC, metric_name`

	return s.queryMetrics(ctx, query, args)
}

// QueryAll returns metrics for all agents, optionally filtered by period prefix.
func (s *Store) QueryAll(ctx context.Context, period string) ([]AgentMetric, error) {
	query := `SELECT instance_id, metric_name, metric_value, period FROM agent_metrics WHERE 1=1`
	args := []any{}

	if period != "" {
		query += ` AND period LIKE ?`
		args = append(args, period+"%")
	}
	query += ` ORDER BY instance_id, period DESC, metric_name`

	return s.queryMetrics(ctx, query, args)
}

// Summarize returns aggregated metric totals per agent (across all periods, or filtered by period prefix).
func (s *Store) Summarize(ctx context.Context, period string) ([]AgentSummary, error) {
	query := `SELECT instance_id, metric_name, SUM(metric_value) FROM agent_metrics WHERE 1=1`
	args := []any{}

	if period != "" {
		query += ` AND period LIKE ?`
		args = append(args, period+"%")
	}
	query += ` GROUP BY instance_id, metric_name ORDER BY instance_id, metric_name`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("summarize metrics: %w", err)
	}
	defer rows.Close()

	summaryMap := map[string]*AgentSummary{}
	var order []string

	for rows.Next() {
		var id, name string
		var total int64
		if err := rows.Scan(&id, &name, &total); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		if _, ok := summaryMap[id]; !ok {
			summaryMap[id] = &AgentSummary{InstanceID: id, Metrics: map[string]int64{}}
			order = append(order, id)
		}
		summaryMap[id].Metrics[name] = total
	}

	var result []AgentSummary
	for _, id := range order {
		result = append(result, *summaryMap[id])
	}
	return result, rows.Err()
}

func (s *Store) queryMetrics(ctx context.Context, query string, args []any) ([]AgentMetric, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var items []AgentMetric
	for rows.Next() {
		var m AgentMetric
		if err := rows.Scan(&m.InstanceID, &m.MetricName, &m.MetricValue, &m.Period); err != nil {
			return nil, fmt.Errorf("scan metric: %w", err)
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

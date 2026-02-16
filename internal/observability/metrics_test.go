package observability_test

import (
	"context"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/observability"
)

func testStore(t *testing.T) *observability.Store {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return observability.New(database)
}

func TestIncrementAndQuery(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	err := s.Increment(ctx, "agent-1", "state.put")
	if err != nil {
		t.Fatal(err)
	}
	err = s.Increment(ctx, "agent-1", "state.put")
	if err != nil {
		t.Fatal(err)
	}

	metrics, err := s.QueryAgent(ctx, "agent-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].MetricValue != 2 {
		t.Errorf("expected value 2, got %d", metrics[0].MetricValue)
	}
	if metrics[0].MetricName != "state.put" {
		t.Errorf("expected state.put, got %s", metrics[0].MetricName)
	}
}

func TestIncrementBy(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.IncrementBy(ctx, "agent-1", "tokens", 300)
	s.IncrementBy(ctx, "agent-1", "tokens", 200)

	metrics, err := s.QueryAgent(ctx, "agent-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].MetricValue != 500 {
		t.Errorf("expected 500, got %d", metrics[0].MetricValue)
	}
}

func TestQueryAll(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Increment(ctx, "agent-1", "state.put")
	s.Increment(ctx, "agent-2", "spec.put")

	metrics, err := s.QueryAll(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}
}

func TestSummarize(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Increment(ctx, "agent-1", "state.put")
	s.Increment(ctx, "agent-1", "state.put")
	s.Increment(ctx, "agent-1", "spec.put")
	s.Increment(ctx, "agent-2", "state.put")

	summaries, err := s.Summarize(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 agent summaries, got %d", len(summaries))
	}

	// Find agent-1.
	var a1 *observability.AgentSummary
	for i := range summaries {
		if summaries[i].InstanceID == "agent-1" {
			a1 = &summaries[i]
		}
	}
	if a1 == nil {
		t.Fatal("agent-1 not found in summaries")
	}
	if a1.Metrics["state.put"] != 2 {
		t.Errorf("expected agent-1 state.put=2, got %d", a1.Metrics["state.put"])
	}
	if a1.Metrics["spec.put"] != 1 {
		t.Errorf("expected agent-1 spec.put=1, got %d", a1.Metrics["spec.put"])
	}
}

func TestQueryAgentEmpty(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	metrics, err := s.QueryAgent(ctx, "nonexistent", "")
	if err != nil {
		t.Fatal(err)
	}
	if metrics != nil {
		t.Errorf("expected nil for empty result, got %v", metrics)
	}
}

package llmcost

import (
	"context"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return New(database)
}

func TestRecordAndQueryByInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	id, err := s.Record(ctx, UsageRecord{
		InstanceID: "agent-1", Project: "proj-a", Provider: "anthropic",
		Model: "claude-sonnet-4-20250514", TokensIn: 100, TokensOut: 200, CostUSD: 0.01,
		RequestType: "completion", SessionTag: "test-session",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	records, err := s.QueryByInstance(ctx, "agent-1", "", "", 10)
	if err != nil {
		t.Fatalf("query by instance: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Provider != "anthropic" {
		t.Errorf("expected provider anthropic, got %s", records[0].Provider)
	}
	if records[0].TokensIn != 100 {
		t.Errorf("expected tokens_in 100, got %d", records[0].TokensIn)
	}
}

func TestQueryByProject(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1", Project: "proj-a"})
	s.Record(ctx, UsageRecord{Provider: "openai", Model: "m2", Project: "proj-b"})
	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m3", Project: "proj-a"})

	records, err := s.QueryByProject(ctx, "proj-a", "", "", 50)
	if err != nil {
		t.Fatalf("query by project: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records for proj-a, got %d", len(records))
	}
}

func TestQueryBySessionTag(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1", SessionTag: "refactor"})
	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1", SessionTag: "bugfix"})
	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1", SessionTag: "refactor"})

	records, err := s.QueryBySessionTag(ctx, "refactor", "", "", 50)
	if err != nil {
		t.Fatalf("query by session tag: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records for refactor, got %d", len(records))
	}
}

func TestQueryAllAndLimit(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1"})
	}

	records, err := s.QueryAll(ctx, "", "", 3)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records (limit), got %d", len(records))
	}
}

func TestSummarizeByProject(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1", Project: "p1", TokensIn: 100, TokensOut: 200, CostUSD: 0.01})
	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1", Project: "p1", TokensIn: 50, TokensOut: 100, CostUSD: 0.005})
	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "m1", Project: "p2", TokensIn: 300, TokensOut: 600, CostUSD: 0.05})

	groups, err := s.SummarizeByProject(ctx, "", "")
	if err != nil {
		t.Fatalf("summarize by project: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups["p1"].TotalTokensIn != 150 {
		t.Errorf("expected p1 tokens_in 150, got %d", groups["p1"].TotalTokensIn)
	}
	if groups["p1"].RequestCount != 2 {
		t.Errorf("expected p1 request_count 2, got %d", groups["p1"].RequestCount)
	}
}

func TestSummarizeByModel(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.Record(ctx, UsageRecord{Provider: "anthropic", Model: "claude", TokensIn: 100, CostUSD: 0.01})
	s.Record(ctx, UsageRecord{Provider: "openai", Model: "gpt4", TokensIn: 200, CostUSD: 0.02})

	groups, err := s.SummarizeByModel(ctx, "", "")
	if err != nil {
		t.Fatalf("summarize by model: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups["claude"].TotalTokensIn != 100 {
		t.Errorf("expected claude tokens_in 100, got %d", groups["claude"].TotalTokensIn)
	}
}

func TestTotal(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", TokensIn: 100, TokensOut: 200, CostUSD: 0.01})
	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", TokensIn: 50, TokensOut: 100, CostUSD: 0.005})

	total, err := s.Total(ctx, "", "")
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total.TotalTokensIn != 150 {
		t.Errorf("expected total tokens_in 150, got %d", total.TotalTokensIn)
	}
	if total.RequestCount != 2 {
		t.Errorf("expected request_count 2, got %d", total.RequestCount)
	}
}

func TestEmptyResults(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	records, err := s.QueryAll(ctx, "", "", 50)
	if err != nil {
		t.Fatalf("query all empty: %v", err)
	}
	if records != nil {
		t.Errorf("expected nil for empty results, got %d records", len(records))
	}

	total, err := s.Total(ctx, "", "")
	if err != nil {
		t.Fatalf("total empty: %v", err)
	}
	if total.RequestCount != 0 {
		t.Errorf("expected 0 request count, got %d", total.RequestCount)
	}

	groups, err := s.SummarizeByProject(ctx, "", "")
	if err != nil {
		t.Fatalf("summarize empty: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected empty groups, got %d", len(groups))
	}
}

func TestSummarizeByInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", InstanceID: "agent-1", TokensIn: 100, CostUSD: 0.01})
	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", InstanceID: "agent-1", TokensIn: 50, CostUSD: 0.005})
	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", InstanceID: "agent-2", TokensIn: 200, CostUSD: 0.03})

	groups, err := s.SummarizeByInstance(ctx, "", "")
	if err != nil {
		t.Fatalf("summarize by instance: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups["agent-1"].TotalTokensIn != 150 {
		t.Errorf("expected agent-1 tokens_in 150, got %d", groups["agent-1"].TotalTokensIn)
	}
	if groups["agent-2"].RequestCount != 1 {
		t.Errorf("expected agent-2 request_count 1, got %d", groups["agent-2"].RequestCount)
	}
}

func TestSummarizeBySessionTag(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", SessionTag: "workflow-1", TokensIn: 100, CostUSD: 0.01})
	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", SessionTag: "workflow-1", TokensIn: 200, CostUSD: 0.02})
	s.Record(ctx, UsageRecord{Provider: "a", Model: "m", SessionTag: "workflow-2", TokensIn: 50, CostUSD: 0.005})

	groups, err := s.SummarizeBySessionTag(ctx, "", "")
	if err != nil {
		t.Fatalf("summarize by session tag: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups["workflow-1"].TotalTokensIn != 300 {
		t.Errorf("expected workflow-1 tokens_in 300, got %d", groups["workflow-1"].TotalTokensIn)
	}
}

package audit_test

import (
	"context"
	"testing"

	"github.com/DavidRHerbert/koor/internal/audit"
	"github.com/DavidRHerbert/koor/internal/db"
)

func testLog(t *testing.T) *audit.Log {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return audit.New(database)
}

func TestAppendAndQuery(t *testing.T) {
	l := testLog(t)
	ctx := context.Background()

	err := l.Append(ctx, "agent-1", "state.put", "Truck-Wash/status", `{"version":1}`, "success")
	if err != nil {
		t.Fatal(err)
	}
	err = l.Append(ctx, "agent-2", "spec.put", "Truck-Wash/contract", "{}", "success")
	if err != nil {
		t.Fatal(err)
	}

	entries, err := l.Query(ctx, "", "", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Results ordered by id DESC, so newest first.
	if entries[0].Action != "spec.put" {
		t.Errorf("expected spec.put first, got %s", entries[0].Action)
	}
}

func TestQueryByActor(t *testing.T) {
	l := testLog(t)
	ctx := context.Background()

	l.Append(ctx, "agent-1", "state.put", "key1", "{}", "success")
	l.Append(ctx, "agent-2", "state.put", "key2", "{}", "success")
	l.Append(ctx, "agent-1", "state.delete", "key3", "{}", "success")

	entries, err := l.Query(ctx, "agent-1", "", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for agent-1, got %d", len(entries))
	}
}

func TestQueryByAction(t *testing.T) {
	l := testLog(t)
	ctx := context.Background()

	l.Append(ctx, "agent-1", "state.put", "key1", "{}", "success")
	l.Append(ctx, "agent-2", "state.delete", "key2", "{}", "success")

	entries, err := l.Query(ctx, "", "state.delete", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 delete entry, got %d", len(entries))
	}
	if entries[0].Actor != "agent-2" {
		t.Errorf("expected agent-2, got %s", entries[0].Actor)
	}
}

func TestQueryLimit(t *testing.T) {
	l := testLog(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		l.Append(ctx, "agent", "state.put", "key", "{}", "success")
	}

	entries, err := l.Query(ctx, "", "", "", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries with limit=3, got %d", len(entries))
	}
}

func TestQuerySummary(t *testing.T) {
	l := testLog(t)
	ctx := context.Background()

	l.Append(ctx, "agent-1", "state.put", "key1", "{}", "success")
	l.Append(ctx, "agent-2", "state.put", "key2", "{}", "success")
	l.Append(ctx, "agent-1", "state.delete", "key1", "{}", "success")
	l.Append(ctx, "agent-3", "spec.put", "project/spec", "{}", "error")

	summary, err := l.QuerySummary(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalEntries != 4 {
		t.Errorf("expected 4 total, got %d", summary.TotalEntries)
	}
	if summary.ActionCounts["state.put"] != 2 {
		t.Errorf("expected 2 state.put, got %d", summary.ActionCounts["state.put"])
	}
	if summary.OutcomeCounts["success"] != 3 {
		t.Errorf("expected 3 success, got %d", summary.OutcomeCounts["success"])
	}
	if summary.OutcomeCounts["error"] != 1 {
		t.Errorf("expected 1 error, got %d", summary.OutcomeCounts["error"])
	}
	if summary.UniqueActors != 3 {
		t.Errorf("expected 3 unique actors, got %d", summary.UniqueActors)
	}
	if summary.UniqueResources != 3 {
		t.Errorf("expected 3 unique resources, got %d", summary.UniqueResources)
	}
}

func TestDefaultOutcome(t *testing.T) {
	l := testLog(t)
	ctx := context.Background()

	err := l.Append(ctx, "agent", "test.action", "resource", "", "")
	if err != nil {
		t.Fatal(err)
	}

	entries, err := l.Query(ctx, "", "", "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("expected 1 entry")
	}
	if entries[0].Outcome != "success" {
		t.Errorf("expected default outcome 'success', got %s", entries[0].Outcome)
	}
	if entries[0].Detail != "{}" {
		t.Errorf("expected default detail '{}', got %s", entries[0].Detail)
	}
}

func TestEmptyQuery(t *testing.T) {
	l := testLog(t)
	ctx := context.Background()

	entries, err := l.Query(ctx, "", "", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty result, got %v", entries)
	}
}

func TestDetailJSON(t *testing.T) {
	got := audit.DetailJSON(map[string]any{"version": 5, "key": "test"})
	if got == "{}" {
		t.Error("expected non-empty JSON")
	}
}

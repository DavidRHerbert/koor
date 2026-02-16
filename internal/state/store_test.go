package state_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/state"
)

func testStore(t *testing.T) *state.Store {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return state.New(database)
}

func TestPutAndGet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	entry, err := s.Put(ctx, "test-key", []byte(`{"foo":"bar"}`), "application/json", "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Version != 1 {
		t.Errorf("expected version 1, got %d", entry.Version)
	}
	if entry.Hash == "" {
		t.Error("expected non-empty hash")
	}

	got, err := s.Get(ctx, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Value) != `{"foo":"bar"}` {
		t.Errorf("unexpected value: %s", got.Value)
	}
	if got.ContentType != "application/json" {
		t.Errorf("unexpected content type: %s", got.ContentType)
	}
}

func TestVersionIncrement(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "key", []byte("v1"), "text/plain", "")
	entry, _ := s.Put(ctx, "key", []byte("v2"), "text/plain", "")

	if entry.Version != 2 {
		t.Errorf("expected version 2, got %d", entry.Version)
	}

	entry, _ = s.Put(ctx, "key", []byte("v3"), "text/plain", "")
	if entry.Version != 3 {
		t.Errorf("expected version 3, got %d", entry.Version)
	}
}

func TestList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "alpha", []byte("a"), "text/plain", "")
	s.Put(ctx, "beta", []byte("b"), "text/plain", "")

	items, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Key != "alpha" || items[1].Key != "beta" {
		t.Errorf("unexpected keys: %s, %s", items[0].Key, items[1].Key)
	}
}

func TestGetNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "to-delete", []byte("data"), "text/plain", "")

	if err := s.Delete(ctx, "to-delete"); err != nil {
		t.Fatal(err)
	}

	_, err := s.Get(ctx, "to-delete")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	err := s.Delete(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestHashChangesOnUpdate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	e1, _ := s.Put(ctx, "key", []byte("value1"), "text/plain", "")
	e2, _ := s.Put(ctx, "key", []byte("value2"), "text/plain", "")

	if e1.Hash == e2.Hash {
		t.Error("expected different hashes for different values")
	}
}

// --- Phase 10: State History + Rollback tests ---

func TestHistoryTracksVersions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "k", []byte(`{"v":1}`), "application/json", "agent-a")
	s.Put(ctx, "k", []byte(`{"v":2}`), "application/json", "agent-b")
	s.Put(ctx, "k", []byte(`{"v":3}`), "application/json", "agent-c")

	history, err := s.History(ctx, "k", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}
	// Most recent first.
	if history[0].Version != 3 {
		t.Errorf("expected version 3 first, got %d", history[0].Version)
	}
	if history[2].Version != 1 {
		t.Errorf("expected version 1 last, got %d", history[2].Version)
	}
}

func TestHistoryLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		s.Put(ctx, "k", []byte(fmt.Sprintf(`{"v":%d}`, i)), "application/json", "")
	}

	history, err := s.History(ctx, "k", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 entries with limit=2, got %d", len(history))
	}
	if history[0].Version != 5 {
		t.Errorf("expected latest version 5, got %d", history[0].Version)
	}
}

func TestHistoryEmptyKey(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	history, err := s.History(ctx, "nonexistent", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 entries for nonexistent key, got %d", len(history))
	}
}

func TestGetVersion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "k", []byte(`{"v":1}`), "application/json", "")
	s.Put(ctx, "k", []byte(`{"v":2}`), "application/json", "")
	s.Put(ctx, "k", []byte(`{"v":3}`), "application/json", "")

	// Get historical version.
	e1, err := s.GetVersion(ctx, "k", 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(e1.Value) != `{"v":1}` {
		t.Errorf("expected v1 value, got %s", e1.Value)
	}

	// Get current version.
	e3, err := s.GetVersion(ctx, "k", 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(e3.Value) != `{"v":3}` {
		t.Errorf("expected v3 value, got %s", e3.Value)
	}

	// Non-existent version.
	_, err = s.GetVersion(ctx, "k", 99)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows for version 99, got %v", err)
	}
}

func TestRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "k", []byte(`{"v":1}`), "application/json", "agent-a")
	s.Put(ctx, "k", []byte(`{"v":2}`), "application/json", "agent-b")
	s.Put(ctx, "k", []byte(`{"bad":"data"}`), "application/json", "rogue-agent")

	// Rollback to version 1.
	entry, err := s.Rollback(ctx, "k", 1)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Version != 4 {
		t.Errorf("expected new version 4 after rollback, got %d", entry.Version)
	}
	if string(entry.Value) != `{"v":1}` {
		t.Errorf("expected rolled-back value, got %s", entry.Value)
	}
	if entry.UpdatedBy != "rollback:v1" {
		t.Errorf("expected updated_by rollback:v1, got %s", entry.UpdatedBy)
	}
}

func TestRollbackNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "k", []byte(`{"v":1}`), "application/json", "")

	_, err := s.Rollback(ctx, "k", 99)
	if err == nil {
		t.Error("expected error for nonexistent rollback version")
	}
}

func TestDiff(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "k", []byte(`{"name":"Alice","age":30,"city":"London"}`), "application/json", "")
	s.Put(ctx, "k", []byte(`{"name":"Alice","age":31,"country":"UK"}`), "application/json", "")

	diffs, err := s.Diff(ctx, "k", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Expect: age changed, city removed, country added.
	if len(diffs) != 3 {
		t.Fatalf("expected 3 diffs, got %d: %+v", len(diffs), diffs)
	}

	diffMap := map[string]string{}
	for _, d := range diffs {
		diffMap[d.Path] = d.Kind
	}

	if diffMap["age"] != "changed" {
		t.Errorf("expected age changed, got %s", diffMap["age"])
	}
	if diffMap["city"] != "removed" {
		t.Errorf("expected city removed, got %s", diffMap["city"])
	}
	if diffMap["country"] != "added" {
		t.Errorf("expected country added, got %s", diffMap["country"])
	}
}

func TestDiffNonJSON(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "k", []byte("plain text v1"), "text/plain", "")
	s.Put(ctx, "k", []byte("plain text v2"), "text/plain", "")

	_, err := s.Diff(ctx, "k", 1, 2)
	if err == nil {
		t.Error("expected error diffing non-JSON values")
	}
}

func TestRollbackPreservesHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Put(ctx, "k", []byte(`{"v":1}`), "application/json", "")
	s.Put(ctx, "k", []byte(`{"v":2}`), "application/json", "")
	s.Rollback(ctx, "k", 1) // creates version 3

	history, err := s.History(ctx, "k", 50)
	if err != nil {
		t.Fatal(err)
	}
	// Should have versions 3, 2, 1.
	if len(history) != 3 {
		t.Fatalf("expected 3 versions after rollback, got %d", len(history))
	}
	if history[0].Version != 3 {
		t.Errorf("expected latest version 3, got %d", history[0].Version)
	}
}

package state_test

import (
	"context"
	"database/sql"
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

package instances_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/instances"
)

func testRegistry(t *testing.T) *instances.Registry {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return instances.New(database)
}

func TestRegisterAndGet(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	inst, err := reg.Register(ctx, "claude-frontend", "/workspace/proj", "building UI", "goth")
	if err != nil {
		t.Fatal(err)
	}
	if inst.ID == "" {
		t.Error("expected non-empty ID")
	}
	if inst.Token == "" {
		t.Error("expected non-empty token")
	}
	if inst.Name != "claude-frontend" {
		t.Errorf("expected name claude-frontend, got %s", inst.Name)
	}
	if inst.Stack != "goth" {
		t.Errorf("expected stack goth, got %s", inst.Stack)
	}

	got, err := reg.Get(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != inst.Name {
		t.Errorf("expected %s, got %s", inst.Name, got.Name)
	}
	if got.Stack != "goth" {
		t.Errorf("expected stack goth, got %s", got.Stack)
	}
}

func TestList(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	reg.Register(ctx, "agent-a", "", "", "")
	reg.Register(ctx, "agent-b", "", "", "")

	items, err := reg.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(items))
	}
}

func TestDiscover(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	reg.Register(ctx, "claude", "/ws/alpha", "", "")
	reg.Register(ctx, "claude", "/ws/beta", "", "")
	reg.Register(ctx, "cursor", "/ws/alpha", "", "")

	// Filter by name.
	items, err := reg.Discover(ctx, "claude", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 claude instances, got %d", len(items))
	}

	// Filter by workspace.
	items, err = reg.Discover(ctx, "", "/ws/alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 alpha instances, got %d", len(items))
	}

	// Filter by both.
	items, err = reg.Discover(ctx, "cursor", "/ws/alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(items))
	}
}

func TestDiscoverByStack(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	reg.Register(ctx, "scanner-a", "/ws/proj", "", "goth")
	reg.Register(ctx, "scanner-b", "/ws/proj", "", "goth")
	reg.Register(ctx, "scanner-c", "/ws/proj", "", "react")

	// Filter by stack.
	items, err := reg.Discover(ctx, "", "", "goth")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 goth instances, got %d", len(items))
	}
	for _, item := range items {
		if item.Stack != "goth" {
			t.Errorf("expected stack goth, got %s", item.Stack)
		}
	}

	// Filter by stack + name.
	items, err = reg.Discover(ctx, "scanner-c", "", "react")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 react instance, got %d", len(items))
	}

	// No matches.
	items, err = reg.Discover(ctx, "", "", "vue")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 vue instances, got %d", len(items))
	}
}

func TestSetIntent(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	inst, _ := reg.Register(ctx, "agent", "", "initial task", "")
	err := reg.SetIntent(ctx, inst.ID, "new task")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := reg.Get(ctx, inst.ID)
	if got.Intent != "new task" {
		t.Errorf("expected 'new task', got %s", got.Intent)
	}
}

func TestSetIntentNotFound(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	err := reg.SetIntent(ctx, "nonexistent", "task")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestDeregister(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	inst, _ := reg.Register(ctx, "temp", "", "", "")
	err := reg.Deregister(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = reg.Get(ctx, inst.ID)
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after deregister, got %v", err)
	}
}

func TestDeregisterNotFound(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	err := reg.Deregister(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestGetNotFound(t *testing.T) {
	reg := testRegistry(t)
	ctx := context.Background()

	_, err := reg.Get(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

package templates_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/templates"
)

func testStore(t *testing.T) *templates.Store {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return templates.New(database)
}

func TestCreateAndGet(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	data, _ := json.Marshal([]map[string]string{{"rule_id": "no-eval", "pattern": "eval"}})
	tmpl, err := store.Create(ctx, "tpl-1", "No Eval Rules", "Blocks eval usage", "rules", data, []string{"security", "js"})
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.ID != "tpl-1" {
		t.Errorf("expected id tpl-1, got %s", tmpl.ID)
	}
	if tmpl.Kind != "rules" {
		t.Errorf("expected kind rules, got %s", tmpl.Kind)
	}
	if len(tmpl.Tags) != 2 || tmpl.Tags[0] != "security" {
		t.Errorf("unexpected tags: %v", tmpl.Tags)
	}

	got, err := store.Get(ctx, "tpl-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "No Eval Rules" {
		t.Errorf("expected name 'No Eval Rules', got %s", got.Name)
	}
}

func TestGetNotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestList(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	store.Create(ctx, "tpl-rules", "Rules", "", "rules", []byte(`[]`), []string{"security"})
	store.Create(ctx, "tpl-contract", "Contract", "", "contracts", []byte(`{}`), []string{"api"})

	// List all.
	items, err := store.List(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(items))
	}

	// Filter by kind.
	items, err = store.List(ctx, "rules", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 rules template, got %d", len(items))
	}
	if items[0].ID != "tpl-rules" {
		t.Errorf("expected tpl-rules, got %s", items[0].ID)
	}

	// Filter by tag.
	items, err = store.List(ctx, "", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 api-tagged template, got %d", len(items))
	}
	if items[0].ID != "tpl-contract" {
		t.Errorf("expected tpl-contract, got %s", items[0].ID)
	}
}

func TestDelete(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	store.Create(ctx, "tpl-del", "Temp", "", "rules", []byte(`[]`), []string{})
	err := store.Delete(ctx, "tpl-del")
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Get(ctx, "tpl-del")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestApply(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	data := []byte(`[{"rule_id":"no-eval","pattern":"eval"}]`)
	store.Create(ctx, "tpl-apply", "Apply Test", "", "rules", data, []string{})

	got, kind, err := store.Apply(ctx, "tpl-apply")
	if err != nil {
		t.Fatal(err)
	}
	if kind != "rules" {
		t.Errorf("expected kind rules, got %s", kind)
	}
	if string(got) != string(data) {
		t.Errorf("expected data match, got %s", got)
	}
}

func TestApplyNotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, _, err := store.Apply(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

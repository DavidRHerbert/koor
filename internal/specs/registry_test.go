package specs_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/specs"
)

func testRegistry(t *testing.T) *specs.Registry {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return specs.New(database)
}

func TestSpecPutAndGet(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()

	spec, err := r.Put(ctx, "myproject", "states", []byte(`{"open":{"transitions":["closed"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != 1 {
		t.Errorf("expected version 1, got %d", spec.Version)
	}

	got, err := r.Get(ctx, "myproject", "states")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != `{"open":{"transitions":["closed"]}}` {
		t.Errorf("unexpected data: %s", got.Data)
	}
	if got.Project != "myproject" || got.Name != "states" {
		t.Errorf("unexpected project/name: %s/%s", got.Project, got.Name)
	}
}

func TestSpecVersionIncrement(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()

	r.Put(ctx, "proj", "spec", []byte("v1"))
	spec, _ := r.Put(ctx, "proj", "spec", []byte("v2"))

	if spec.Version != 2 {
		t.Errorf("expected version 2, got %d", spec.Version)
	}
}

func TestSpecList(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()

	r.Put(ctx, "proj", "alpha", []byte("a"))
	r.Put(ctx, "proj", "beta", []byte("b"))
	r.Put(ctx, "other", "gamma", []byte("c"))

	items, err := r.List(ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "alpha" || items[1].Name != "beta" {
		t.Errorf("unexpected names: %s, %s", items[0].Name, items[1].Name)
	}
}

func TestSpecGetNotFound(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()

	_, err := r.Get(ctx, "nope", "nada")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestSpecDelete(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()

	r.Put(ctx, "proj", "spec", []byte("data"))

	if err := r.Delete(ctx, "proj", "spec"); err != nil {
		t.Fatal(err)
	}

	_, err := r.Get(ctx, "proj", "spec")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestSpecDeleteNotFound(t *testing.T) {
	r := testRegistry(t)
	ctx := context.Background()

	err := r.Delete(ctx, "nope", "nada")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

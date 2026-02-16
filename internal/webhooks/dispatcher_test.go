package webhooks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/webhooks"
)

type testEnv struct {
	db   *sql.DB
	bus  *events.Bus
	disp *webhooks.Dispatcher
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	bus := events.New(database, 100)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	disp := webhooks.New(database, bus, logger)
	return &testEnv{db: database, bus: bus, disp: disp}
}

func TestRegisterAndList(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	wh, err := env.disp.Register(ctx, "wh-1", "http://example.com/hook", []string{"agent.*"}, "mysecret")
	if err != nil {
		t.Fatal(err)
	}
	if wh.ID != "wh-1" {
		t.Errorf("expected id wh-1, got %s", wh.ID)
	}
	if !wh.Active {
		t.Error("expected webhook to be active")
	}
	if len(wh.Patterns) != 1 || wh.Patterns[0] != "agent.*" {
		t.Errorf("unexpected patterns: %v", wh.Patterns)
	}

	hooks, err := env.disp.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(hooks))
	}
}

func TestDelete(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.disp.Register(ctx, "wh-del", "http://example.com/hook", []string{"*"}, "")
	err := env.disp.Delete(ctx, "wh-del")
	if err != nil {
		t.Fatal(err)
	}

	hooks, _ := env.disp.List(ctx)
	if len(hooks) != 0 {
		t.Errorf("expected 0 webhooks after delete, got %d", len(hooks))
	}
}

func TestDeleteNotFound(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	err := env.disp.Delete(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestDispatchToMatchingWebhook(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	var received atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if payload["topic"] != "agent.stale" {
			t.Errorf("expected topic agent.stale, got %v", payload["topic"])
		}
		if r.Header.Get("X-Koor-Event") != "true" {
			t.Error("expected X-Koor-Event header")
		}
		w.WriteHeader(200)
	}))
	defer backend.Close()

	env.disp.Register(ctx, "wh-match", backend.URL, []string{"agent.*"}, "")
	env.disp.Start()
	defer env.disp.Stop()

	env.bus.Publish(ctx, "agent.stale", json.RawMessage(`{"instance_id":"abc"}`), "liveness")

	// Give dispatcher time to process.
	time.Sleep(200 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("expected 1 webhook call, got %d", received.Load())
	}
}

func TestDispatchSkipsNonMatching(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	var received atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer backend.Close()

	// Only subscribe to compliance.* pattern.
	env.disp.Register(ctx, "wh-nomatch", backend.URL, []string{"compliance.*"}, "")
	env.disp.Start()
	defer env.disp.Stop()

	// Publish an agent event — should not match.
	env.bus.Publish(ctx, "agent.stale", json.RawMessage(`{}`), "")
	time.Sleep(200 * time.Millisecond)

	if received.Load() != 0 {
		t.Errorf("expected 0 webhook calls for non-matching topic, got %d", received.Load())
	}
}

func TestHMACSignature(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	var gotSig string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Koor-Signature")
		w.WriteHeader(200)
	}))
	defer backend.Close()

	env.disp.Register(ctx, "wh-hmac", backend.URL, []string{"*"}, "secret123")
	env.disp.Start()
	defer env.disp.Stop()

	env.bus.Publish(ctx, "test.event", json.RawMessage(`{}`), "")
	time.Sleep(200 * time.Millisecond)

	if gotSig == "" {
		t.Error("expected non-empty HMAC signature")
	}
}

func TestFailCountIncrement(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Backend returns 500.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer backend.Close()

	env.disp.Register(ctx, "wh-fail", backend.URL, []string{"*"}, "")
	env.disp.Start()
	defer env.disp.Stop()

	env.bus.Publish(ctx, "test.event", json.RawMessage(`{}`), "")
	time.Sleep(200 * time.Millisecond)

	wh, err := env.disp.Get(ctx, "wh-fail")
	if err != nil {
		t.Fatal(err)
	}
	if wh.FailCount < 1 {
		t.Errorf("expected fail_count >= 1, got %d", wh.FailCount)
	}
}

func TestTestFire(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	var received atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if payload["topic"] != "webhook.test" {
			t.Errorf("expected topic webhook.test, got %v", payload["topic"])
		}
		w.WriteHeader(200)
	}))
	defer backend.Close()

	env.disp.Register(ctx, "wh-test", backend.URL, []string{"*"}, "")

	err := env.disp.TestFire(ctx, "wh-test")
	if err != nil {
		t.Fatal(err)
	}
	if received.Load() != 1 {
		t.Errorf("expected 1 test fire, got %d", received.Load())
	}
}

func TestTestFireNotFound(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	err := env.disp.TestFire(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent webhook")
	}
}

package liveness_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/liveness"
)

type testEnv struct {
	db       *sql.DB
	registry *instances.Registry
	bus      *events.Bus
	logger   *slog.Logger
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return &testEnv{
		db:       database,
		registry: instances.New(database),
		bus:      events.New(database, 100),
		logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
}

func (e *testEnv) registerActive(t *testing.T, name string) *instances.Instance {
	t.Helper()
	ctx := context.Background()
	inst, err := e.registry.Register(ctx, name, "/ws", "task", "go")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.registry.Activate(ctx, inst.ID); err != nil {
		t.Fatal(err)
	}
	return inst
}

func (e *testEnv) backdateLastSeen(t *testing.T, id string, minutes int) {
	t.Helper()
	_, err := e.db.Exec(
		`UPDATE instances SET last_seen = datetime('now', ? || ' minutes') WHERE id = ?`,
		-minutes, id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckNowNoStaleAgents(t *testing.T) {
	env := setup(t)
	env.registerActive(t, "agent-a")

	mon := liveness.New(env.registry, env.bus, time.Hour, time.Minute, env.logger)
	marked := mon.CheckNow(context.Background())
	if len(marked) != 0 {
		t.Errorf("expected 0 stale, got %d", len(marked))
	}
}

func TestCheckNowDetectsStale(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	inst := env.registerActive(t, "agent-stale")
	env.backdateLastSeen(t, inst.ID, 10)

	mon := liveness.New(env.registry, env.bus, 5*time.Minute, time.Minute, env.logger)
	marked := mon.CheckNow(ctx)
	if len(marked) != 1 {
		t.Fatalf("expected 1 stale, got %d", len(marked))
	}
	if marked[0].ID != inst.ID {
		t.Errorf("expected stale instance %s, got %s", inst.ID, marked[0].ID)
	}
	if marked[0].Status != "stale" {
		t.Errorf("expected status stale, got %s", marked[0].Status)
	}

	got, err := env.registry.Get(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stale" {
		t.Errorf("expected DB status stale, got %s", got.Status)
	}
}

func TestCheckNowEmitsEvent(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	inst := env.registerActive(t, "agent-events")
	env.backdateLastSeen(t, inst.ID, 10)

	sub := env.bus.Subscribe("agent.*")
	defer env.bus.Unsubscribe(sub)

	mon := liveness.New(env.registry, env.bus, 5*time.Minute, time.Minute, env.logger)
	mon.CheckNow(ctx)

	select {
	case ev := <-sub.Ch:
		if ev.Topic != "agent.stale" {
			t.Errorf("expected topic agent.stale, got %s", ev.Topic)
		}
		var data map[string]any
		json.Unmarshal(ev.Data, &data)
		if data["instance_id"] != inst.ID {
			t.Errorf("expected instance_id %s in event data", inst.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for agent.stale event")
	}
}

func TestCheckNowIgnoresPendingInstances(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	inst, err := env.registry.Register(ctx, "pending-agent", "/ws", "task", "go")
	if err != nil {
		t.Fatal(err)
	}
	env.backdateLastSeen(t, inst.ID, 10)

	mon := liveness.New(env.registry, env.bus, 5*time.Minute, time.Minute, env.logger)
	marked := mon.CheckNow(ctx)
	if len(marked) != 0 {
		t.Errorf("expected 0 stale (pending agents ignored), got %d", len(marked))
	}
}

func TestCheckNowIgnoresAlreadyStale(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	inst := env.registerActive(t, "agent-already-stale")
	env.backdateLastSeen(t, inst.ID, 10)

	mon := liveness.New(env.registry, env.bus, 5*time.Minute, time.Minute, env.logger)

	marked := mon.CheckNow(ctx)
	if len(marked) != 1 {
		t.Fatalf("first check: expected 1, got %d", len(marked))
	}

	marked = mon.CheckNow(ctx)
	if len(marked) != 0 {
		t.Errorf("second check: expected 0, got %d", len(marked))
	}
}

func TestHeartbeatReactivatesStale(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	inst := env.registerActive(t, "agent-reactivate")
	env.backdateLastSeen(t, inst.ID, 10)

	mon := liveness.New(env.registry, env.bus, 5*time.Minute, time.Minute, env.logger)
	mon.CheckNow(ctx)

	got, _ := env.registry.Get(ctx, inst.ID)
	if got.Status != "stale" {
		t.Fatalf("expected stale, got %s", got.Status)
	}

	if err := env.registry.Heartbeat(ctx, inst.ID); err != nil {
		t.Fatal(err)
	}

	got, _ = env.registry.Get(ctx, inst.ID)
	if got.Status != "active" {
		t.Errorf("expected active after heartbeat, got %s", got.Status)
	}
}

func TestStartAndStop(t *testing.T) {
	env := setup(t)

	mon := liveness.New(env.registry, env.bus, 5*time.Minute, 50*time.Millisecond, env.logger)
	mon.Start()
	time.Sleep(100 * time.Millisecond)
	mon.Stop()
}

func TestListByStatus(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.registerActive(t, "active-agent")
	env.registry.Register(ctx, "pending-agent", "", "", "")

	active, err := env.registry.ListByStatus(ctx, "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active, got %d", len(active))
	}

	pending, err := env.registry.ListByStatus(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending, got %d", len(pending))
	}
}

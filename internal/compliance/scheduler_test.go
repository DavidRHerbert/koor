package compliance_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/DavidRHerbert/koor/internal/compliance"
	koordb "github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/specs"
)

type testEnv struct {
	db          *sql.DB
	instanceReg *instances.Registry
	specReg     *specs.Registry
	eventBus    *events.Bus
	sched       *compliance.Scheduler
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	database, err := koordb.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	instanceReg := instances.New(database)
	specReg := specs.New(database)
	eventBus := events.New(database, 100)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sched := compliance.New(database, instanceReg, specReg, eventBus, 1*time.Hour, logger)

	return &testEnv{
		db:          database,
		instanceReg: instanceReg,
		specReg:     specReg,
		eventBus:    eventBus,
		sched:       sched,
	}
}

func TestRunAllNoActiveInstances(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	runs := env.sched.RunAll(ctx)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs with no active instances, got %d", len(runs))
	}
}

func TestRunAllWithContractPass(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Register and activate an instance.
	inst, err := env.instanceReg.Register(ctx, "agent-1", "MyProject", "testing", "go")
	if err != nil {
		t.Fatal(err)
	}
	env.instanceReg.Activate(ctx, inst.ID)

	// Store a valid contract spec in the project.
	contract := `{"kind":"contract","version":1,"endpoints":{"GET /api/items":{"response_status":200,"response":{"id":{"type":"string"}}}}}`
	env.specReg.Put(ctx, "MyProject", "api-contract", []byte(contract))

	runs := env.sched.RunAll(ctx)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if !runs[0].Pass {
		t.Errorf("expected pass=true, got false. violations: %s", runs[0].Violations)
	}
	if runs[0].InstanceID != inst.ID {
		t.Errorf("expected instance_id %s, got %s", inst.ID, runs[0].InstanceID)
	}
	if runs[0].Project != "MyProject" {
		t.Errorf("expected project MyProject, got %s", runs[0].Project)
	}
}

func TestRunAllWithContractFail(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Register and activate an instance.
	inst, err := env.instanceReg.Register(ctx, "agent-1", "MyProject", "testing", "go")
	if err != nil {
		t.Fatal(err)
	}
	env.instanceReg.Activate(ctx, inst.ID)

	// Store a contract with an empty endpoint definition — should fail validation.
	contract := `{"kind":"contract","version":1,"endpoints":{"GET /api/empty":{"response_status":200}}}`
	env.specReg.Put(ctx, "MyProject", "bad-contract", []byte(contract))

	runs := env.sched.RunAll(ctx)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Pass {
		t.Error("expected pass=false for contract with no endpoints")
	}
	if string(runs[0].Violations) == "[]" {
		t.Error("expected non-empty violations")
	}
}

func TestRunAllSkipsNonContractSpecs(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Register and activate an instance.
	inst, err := env.instanceReg.Register(ctx, "agent-1", "MyProject", "testing", "go")
	if err != nil {
		t.Fatal(err)
	}
	env.instanceReg.Activate(ctx, inst.ID)

	// Store a non-contract spec (plain JSON, no "kind":"contract").
	env.specReg.Put(ctx, "MyProject", "states", []byte(`{"open":{"transitions":["closed"]}}`))

	runs := env.sched.RunAll(ctx)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs for non-contract specs, got %d", len(runs))
	}
}

func TestRunAllSkipsPendingInstances(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Register but don't activate.
	env.instanceReg.Register(ctx, "agent-pending", "MyProject", "testing", "go")

	// Store a contract.
	contract := `{"kind":"contract","version":1,"endpoints":{"GET /api/items":{"response_status":200}}}`
	env.specReg.Put(ctx, "MyProject", "api-contract", []byte(contract))

	runs := env.sched.RunAll(ctx)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs for pending instances, got %d", len(runs))
	}
}

func TestRunAllEmitsViolationEvent(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Subscribe to compliance events before we run.
	sub := env.eventBus.Subscribe("compliance.*")
	defer env.eventBus.Unsubscribe(sub)

	// Register and activate an instance.
	inst, err := env.instanceReg.Register(ctx, "agent-1", "MyProject", "testing", "go")
	if err != nil {
		t.Fatal(err)
	}
	env.instanceReg.Activate(ctx, inst.ID)

	// Store a contract with an empty endpoint definition — should fail.
	contract := `{"kind":"contract","version":1,"endpoints":{"GET /api/empty":{"response_status":200}}}`
	env.specReg.Put(ctx, "MyProject", "bad-contract", []byte(contract))

	env.sched.RunAll(ctx)

	// Check that a violation event was emitted.
	select {
	case ev := <-sub.Ch:
		if ev.Topic != "compliance.violation" {
			t.Errorf("expected topic compliance.violation, got %s", ev.Topic)
		}
		var data map[string]any
		json.Unmarshal(ev.Data, &data)
		if data["instance_id"] != inst.ID {
			t.Errorf("event data should contain instance_id %s", inst.ID)
		}
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for compliance.violation event")
	}
}

func TestHistoryEmpty(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	runs, err := env.sched.History(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}

func TestHistoryAfterRun(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Register, activate, add contract, run.
	inst, _ := env.instanceReg.Register(ctx, "agent-1", "MyProject", "testing", "go")
	env.instanceReg.Activate(ctx, inst.ID)
	contract := `{"kind":"contract","version":1,"endpoints":{"GET /api/items":{"response_status":200,"response":{"id":{"type":"string"}}}}}`
	env.specReg.Put(ctx, "MyProject", "api-contract", []byte(contract))
	env.sched.RunAll(ctx)

	// History should show the run.
	runs, err := env.sched.History(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run in history, got %d", len(runs))
	}
	if runs[0].Project != "MyProject" {
		t.Errorf("expected project MyProject, got %s", runs[0].Project)
	}

	// Filter by instance_id.
	runs2, err := env.sched.History(ctx, inst.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs2) != 1 {
		t.Fatalf("expected 1 run for instance filter, got %d", len(runs2))
	}

	// Filter by nonexistent instance.
	runs3, err := env.sched.History(ctx, "nonexistent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs3) != 0 {
		t.Errorf("expected 0 runs for nonexistent instance, got %d", len(runs3))
	}
}

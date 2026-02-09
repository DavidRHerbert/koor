package events_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/events"
)

func testBus(t *testing.T) *events.Bus {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return events.New(database, 100)
}

func TestPublishAndHistory(t *testing.T) {
	bus := testBus(t)
	ctx := context.Background()

	ev, err := bus.Publish(ctx, "api.change", json.RawMessage(`{"field":"name"}`), "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if ev.Topic != "api.change" {
		t.Errorf("expected topic api.change, got %s", ev.Topic)
	}

	history, err := bus.History(ctx, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 event, got %d", len(history))
	}
	if string(history[0].Data) != `{"field":"name"}` {
		t.Errorf("unexpected data: %s", history[0].Data)
	}
}

func TestHistoryTopicFilter(t *testing.T) {
	bus := testBus(t)
	ctx := context.Background()

	bus.Publish(ctx, "api.change", json.RawMessage(`"a"`), "")
	bus.Publish(ctx, "ui.update", json.RawMessage(`"b"`), "")
	bus.Publish(ctx, "api.delete", json.RawMessage(`"c"`), "")

	history, err := bus.History(ctx, 10, "api.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 api.* events, got %d", len(history))
	}
}

func TestHistoryLimit(t *testing.T) {
	bus := testBus(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		bus.Publish(ctx, "test", json.RawMessage(`"x"`), "")
	}

	history, err := bus.History(ctx, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 events, got %d", len(history))
	}
}

func TestSubscribeReceivesEvents(t *testing.T) {
	bus := testBus(t)
	ctx := context.Background()

	sub := bus.Subscribe("*")
	defer bus.Unsubscribe(sub)

	bus.Publish(ctx, "test.event", json.RawMessage(`{"msg":"hello"}`), "src")

	select {
	case ev := <-sub.Ch:
		if ev.Topic != "test.event" {
			t.Errorf("expected topic test.event, got %s", ev.Topic)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribePatternFiltering(t *testing.T) {
	bus := testBus(t)
	ctx := context.Background()

	sub := bus.Subscribe("api.*")
	defer bus.Unsubscribe(sub)

	bus.Publish(ctx, "ui.update", json.RawMessage(`"ignored"`), "")
	bus.Publish(ctx, "api.change", json.RawMessage(`"received"`), "")

	select {
	case ev := <-sub.Ch:
		if ev.Topic != "api.change" {
			t.Errorf("expected api.change, got %s", ev.Topic)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	// Verify no extra events arrived.
	select {
	case ev := <-sub.Ch:
		t.Errorf("unexpected event: %s", ev.Topic)
	case <-time.After(50 * time.Millisecond):
		// Good — no extra events.
	}
}

func TestPruning(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	bus := events.New(database, 5) // Keep only 5.
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		bus.Publish(ctx, "test", json.RawMessage(`"x"`), "")
	}

	history, err := bus.History(ctx, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) > 5 {
		t.Errorf("expected at most 5 events after pruning, got %d", len(history))
	}
}

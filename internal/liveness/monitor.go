package liveness

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
)

// Monitor periodically checks for stale agent instances and marks them accordingly.
type Monitor struct {
	registry   *instances.Registry
	eventBus   *events.Bus
	staleAfter time.Duration
	checkEvery time.Duration
	stop       chan struct{}
	logger     *slog.Logger
}

// New creates a new liveness Monitor.
func New(registry *instances.Registry, eventBus *events.Bus, staleAfter, checkEvery time.Duration, logger *slog.Logger) *Monitor {
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	if checkEvery <= 0 {
		checkEvery = 60 * time.Second
	}
	return &Monitor{
		registry:   registry,
		eventBus:   eventBus,
		staleAfter: staleAfter,
		checkEvery: checkEvery,
		stop:       make(chan struct{}),
		logger:     logger,
	}
}

// Start begins periodic staleness checks in a background goroutine.
func (m *Monitor) Start() {
	go func() {
		ticker := time.NewTicker(m.checkEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.CheckNow(context.Background())
			case <-m.stop:
				return
			}
		}
	}()
}

// Stop shuts down the background monitor goroutine.
func (m *Monitor) Stop() {
	select {
	case m.stop <- struct{}{}:
	default:
	}
}

// CheckNow runs a single staleness check and returns newly-staled instances.
func (m *Monitor) CheckNow(ctx context.Context) []instances.Summary {
	stale, err := m.registry.ListStale(ctx, m.staleAfter)
	if err != nil {
		m.logger.Error("liveness check failed", "error", err)
		return nil
	}

	var marked []instances.Summary
	for _, inst := range stale {
		if err := m.registry.MarkStale(ctx, inst.ID); err != nil {
			m.logger.Error("failed to mark instance stale", "id", inst.ID, "name", inst.Name, "error", err)
			continue
		}

		m.logger.Warn("agent marked stale", "id", inst.ID, "name", inst.Name, "last_seen", inst.LastSeen)

		// Publish agent.stale event.
		data, _ := json.Marshal(map[string]any{
			"instance_id": inst.ID,
			"name":        inst.Name,
			"workspace":   inst.Workspace,
			"stack":       inst.Stack,
			"last_seen":   inst.LastSeen,
		})
		m.eventBus.Publish(ctx, "agent.stale", json.RawMessage(data), "liveness-monitor")

		inst.Status = "stale"
		marked = append(marked, inst)
	}
	return marked
}

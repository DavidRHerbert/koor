package compliance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/DavidRHerbert/koor/internal/contracts"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/specs"
)

// Run represents a single compliance check result.
type Run struct {
	ID         int64           `json:"id"`
	InstanceID string          `json:"instance_id"`
	Project    string          `json:"project"`
	Contract   string          `json:"contract"`
	Pass       bool            `json:"pass"`
	Violations json.RawMessage `json:"violations"`
	RunAt      time.Time       `json:"run_at"`
}

// Scheduler periodically validates active agents against their contracts.
type Scheduler struct {
	db          *sql.DB
	instanceReg *instances.Registry
	specReg     *specs.Registry
	eventBus    *events.Bus
	interval    time.Duration
	logger      *slog.Logger
	stop        chan struct{}
}

// New creates a new compliance Scheduler.
func New(db *sql.DB, instanceReg *instances.Registry, specReg *specs.Registry, eventBus *events.Bus, interval time.Duration, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		db:          db,
		instanceReg: instanceReg,
		specReg:     specReg,
		eventBus:    eventBus,
		interval:    interval,
		logger:      logger,
		stop:        make(chan struct{}),
	}
}

// Start launches the background compliance check goroutine.
func (s *Scheduler) Start() {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.RunAll(context.Background())
			case <-s.stop:
				return
			}
		}
	}()
}

// Stop shuts down the scheduler.
func (s *Scheduler) Stop() {
	select {
	case s.stop <- struct{}{}:
	default:
	}
}

// RunAll validates all active instances against their project contracts.
// Returns the list of runs performed.
func (s *Scheduler) RunAll(ctx context.Context) []Run {
	active, err := s.instanceReg.ListByStatus(ctx, "active")
	if err != nil {
		s.logger.Error("compliance: list active instances", "error", err)
		return nil
	}

	var runs []Run
	for _, inst := range active {
		instRuns := s.checkInstance(ctx, inst)
		runs = append(runs, instRuns...)
	}
	return runs
}

// checkInstance validates a single instance against all contracts in its workspace/project.
func (s *Scheduler) checkInstance(ctx context.Context, inst instances.Summary) []Run {
	// Determine project from workspace (convention: workspace contains project prefix).
	// Fall back to checking all specs for contracts.
	project := inst.Workspace
	if project == "" {
		return nil
	}

	specList, err := s.specReg.List(ctx, project)
	if err != nil {
		return nil
	}

	var runs []Run
	for _, sp := range specList {
		specData, err := s.specReg.Get(ctx, project, sp.Name)
		if err != nil {
			continue
		}

		contract, err := contracts.Parse(specData.Data)
		if err != nil {
			continue // Not a contract spec — skip.
		}

		// Validate: check that the contract is well-formed.
		violations := []contracts.Violation{}
		if len(contract.Endpoints) == 0 {
			violations = append(violations, contracts.Violation{
				Path:    "endpoints",
				Message: "contract has no endpoints defined",
			})
		}
		for ep, def := range contract.Endpoints {
			if len(def.Request) == 0 && len(def.Response) == 0 && len(def.ResponseArray) == 0 && len(def.Query) == 0 {
				violations = append(violations, contracts.Violation{
					Path:    "endpoints." + ep,
					Message: "endpoint has no request, response, or query schema defined",
				})
			}
		}

		pass := len(violations) == 0
		violationsJSON, _ := json.Marshal(violations)

		run := s.storeRun(ctx, inst.ID, project, sp.Name, pass, violationsJSON)
		if run != nil {
			runs = append(runs, *run)
		}

		if !pass {
			data, _ := json.Marshal(map[string]any{
				"instance_id": inst.ID,
				"project":     project,
				"contract":    sp.Name,
				"violations":  violations,
			})
			s.eventBus.Publish(ctx, "compliance.violation", data, "compliance-scheduler")
		}
	}
	return runs
}

// storeRun persists a compliance run result.
func (s *Scheduler) storeRun(ctx context.Context, instanceID, project, contract string, pass bool, violations json.RawMessage) *Run {
	passInt := 0
	if pass {
		passInt = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO compliance_runs (instance_id, project, contract, pass, violations, run_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		instanceID, project, contract, passInt, string(violations))
	if err != nil {
		s.logger.Error("store compliance run", "error", err)
		return nil
	}
	id, _ := res.LastInsertId()
	return &Run{
		ID:         id,
		InstanceID: instanceID,
		Project:    project,
		Contract:   contract,
		Pass:       pass,
		Violations: violations,
		RunAt:      time.Now().UTC(),
	}
}

// History returns recent compliance runs, optionally filtered by instance_id.
func (s *Scheduler) History(ctx context.Context, instanceID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 50
	}

	var rows *sql.Rows
	var err error
	if instanceID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, instance_id, project, contract, pass, violations, run_at
			 FROM compliance_runs WHERE instance_id = ? ORDER BY id DESC LIMIT ?`,
			instanceID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, instance_id, project, contract, pass, violations, run_at
			 FROM compliance_runs ORDER BY id DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("query compliance runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		var passInt int
		var runAt, violations string
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.Project, &r.Contract, &passInt, &violations, &runAt); err != nil {
			return nil, fmt.Errorf("scan compliance run: %w", err)
		}
		r.Pass = passInt == 1
		r.Violations = json.RawMessage(violations)
		r.RunAt, _ = time.Parse("2006-01-02 15:04:05", runAt)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

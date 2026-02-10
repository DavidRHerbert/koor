package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"encoding/json"
	"strconv"
	"strings"

	"github.com/DavidRHerbert/koor/internal/contracts"
	"github.com/DavidRHerbert/koor/internal/dashboard"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/specs"
	"github.com/DavidRHerbert/koor/internal/state"
)

// Config holds server configuration.
type Config struct {
	Bind          string
	DashboardBind string
	DataDir       string
	AuthToken     string
}

// Server is the Koor HTTP server.
type Server struct {
	config      Config
	stateStore  *state.Store
	specReg     *specs.Registry
	eventBus    *events.Bus
	instanceReg *instances.Registry
	mcpHandler  http.Handler
	startTime   time.Time
	logger      *slog.Logger
	mcpCalls    atomic.Int64 // MCP tool calls (go through LLM context)
	restCalls   atomic.Int64 // REST/CLI calls (bypass LLM context)
}

// New creates a new Server.
func New(cfg Config, stateStore *state.Store, specReg *specs.Registry, eventBus *events.Bus, instanceReg *instances.Registry, mcpHandler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		config:      cfg,
		stateStore:  stateStore,
		specReg:     specReg,
		eventBus:    eventBus,
		instanceReg: instanceReg,
		mcpHandler:  mcpHandler,
		startTime:   time.Now(),
		logger:      logger,
	}
}

// countREST wraps a handler to count REST/CLI calls.
func (s *Server) countREST(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.restCalls.Add(1)
		next(w, r)
	}
}

// countMCP wraps a handler to count MCP calls.
func (s *Server) countMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mcpCalls.Add(1)
		next.ServeHTTP(w, r)
	})
}

// Handler returns the root HTTP handler with all routes and auth middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health (no auth required, registered on outer mux below).
	// State endpoints.
	mux.HandleFunc("GET /api/state", s.countREST(s.handleStateList))
	mux.HandleFunc("GET /api/state/{key...}", s.countREST(s.handleStateGet))
	mux.HandleFunc("PUT /api/state/{key...}", s.countREST(s.handleStatePut))
	mux.HandleFunc("DELETE /api/state/{key...}", s.countREST(s.handleStateDelete))

	// Specs endpoints.
	mux.HandleFunc("GET /api/specs/{project}", s.countREST(s.handleSpecsList))
	mux.HandleFunc("GET /api/specs/{project}/{name}", s.countREST(s.handleSpecsGet))
	mux.HandleFunc("PUT /api/specs/{project}/{name}", s.countREST(s.handleSpecsPut))
	mux.HandleFunc("DELETE /api/specs/{project}/{name}", s.countREST(s.handleSpecsDelete))

	// Events endpoints.
	mux.HandleFunc("POST /api/events/publish", s.countREST(s.handleEventsPublish))
	mux.HandleFunc("GET /api/events/history", s.countREST(s.handleEventsHistory))
	mux.Handle("GET /api/events/subscribe", events.ServeSubscribe(s.eventBus, s.logger))

	// Instance endpoints.
	mux.HandleFunc("GET /api/instances", s.countREST(s.handleInstancesList))
	mux.HandleFunc("GET /api/instances/{id}", s.countREST(s.handleInstanceGet))
	mux.HandleFunc("POST /api/instances/register", s.countREST(s.handleInstanceRegister))
	mux.HandleFunc("POST /api/instances/{id}/heartbeat", s.countREST(s.handleInstanceHeartbeat))
	mux.HandleFunc("DELETE /api/instances/{id}", s.countREST(s.handleInstanceDeregister))

	// Validation endpoints.
	mux.HandleFunc("GET /api/validate/{project}/rules", s.countREST(s.handleValidateRulesList))
	mux.HandleFunc("PUT /api/validate/{project}/rules", s.countREST(s.handleValidateRulesPut))
	mux.HandleFunc("POST /api/validate/{project}", s.countREST(s.handleValidate))

	// Contract validation endpoints.
	mux.HandleFunc("POST /api/contracts/{project}/{name}/validate", s.countREST(s.handleContractValidate))
	mux.HandleFunc("POST /api/contracts/{project}/{name}/test", s.countREST(s.handleContractTest))

	// Rules management endpoints.
	mux.HandleFunc("POST /api/rules/propose", s.countREST(s.handleRulesPropose))
	mux.HandleFunc("POST /api/rules/{project}/{ruleID}/accept", s.countREST(s.handleRulesAccept))
	mux.HandleFunc("POST /api/rules/{project}/{ruleID}/reject", s.countREST(s.handleRulesReject))
	mux.HandleFunc("GET /api/rules/export", s.countREST(s.handleRulesExport))
	mux.HandleFunc("POST /api/rules/import", s.countREST(s.handleRulesImport))

	// Metrics endpoint (NOT counted — infrastructure, not agent calls).
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)

	// MCP endpoint (StreamableHTTP) — counted as MCP calls.
	if s.mcpHandler != nil {
		mux.Handle("/mcp", s.countMCP(s.mcpHandler))
	}

	// Outer mux: health is public, everything else goes through auth.
	outer := http.NewServeMux()
	outer.HandleFunc("GET /health", s.handleHealth)
	outer.Handle("/", authMiddleware(s.config.AuthToken, mux))

	return outer
}

// DashboardHandler returns the HTTP handler for the dashboard (separate port).
// It proxies /api/* and /health to the API server, serves HTMX rules pages, and embedded static files.
func (s *Server) DashboardHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/", s.dashboardProxy)

	// Dashboard rules HTMX routes.
	mux.HandleFunc("GET /rules", s.handleDashboardRules)
	mux.HandleFunc("GET /rules/list", s.handleDashboardRulesList)
	mux.HandleFunc("GET /rules/form", s.handleDashboardRuleForm)
	mux.HandleFunc("POST /rules/save", s.handleDashboardRuleSave)
	mux.HandleFunc("DELETE /rules/{project}/{ruleID}", s.handleDashboardRuleDelete)
	mux.HandleFunc("POST /rules/{project}/{ruleID}/accept", s.handleDashboardRuleAccept)
	mux.HandleFunc("POST /rules/{project}/{ruleID}/reject", s.handleDashboardRuleReject)

	// Static files (CSS, JS, overview page).
	mux.Handle("GET /", dashboard.Handler())
	return mux
}

// ListenAndServe starts the API server and optionally the dashboard server.
// It blocks until the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	apiSrv := &http.Server{
		Addr:         s.config.Bind,
		Handler:      s.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("API server listening", "bind", s.config.Bind)
		errCh <- apiSrv.ListenAndServe()
	}()

	// Start dashboard on its own port if configured.
	var dashSrv *http.Server
	if s.config.DashboardBind != "" {
		dashSrv = &http.Server{
			Addr:         s.config.DashboardBind,
			Handler:      s.DashboardHandler(),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		go func() {
			s.logger.Info("dashboard listening", "bind", s.config.DashboardBind)
			if err := dashSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("dashboard: %w", err)
			}
		}()
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down servers")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if dashSrv != nil {
			dashSrv.Shutdown(shutdownCtx)
		}
		return apiSrv.Shutdown(shutdownCtx)
	}
}

// --- Dashboard proxy ---

// dashboardProxy forwards API requests from the dashboard port to the API handlers.
// This avoids CORS issues since the dashboard and API are on different ports.
func (s *Server) dashboardProxy(w http.ResponseWriter, r *http.Request) {
	// Re-dispatch through the API handler.
	s.Handler().ServeHTTP(w, r)
}

// --- Health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"uptime": time.Since(s.startTime).Truncate(time.Second).String(),
	})
}

// --- State handlers ---

func (s *Server) handleStateList(w http.ResponseWriter, r *http.Request) {
	items, err := s.stateStore.List(r.Context())
	if err != nil {
		s.logger.Error("state list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list state")
		return
	}
	if items == nil {
		items = []state.Summary{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleStateGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	entry, err := s.stateStore.Get(r.Context(), key)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "key not found: "+key)
		return
	}
	if err != nil {
		s.logger.Error("state get failed", "key", key, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get state")
		return
	}

	// Support ETag caching.
	w.Header().Set("ETag", `"`+entry.Hash+`"`)
	if match := r.Header.Get("If-None-Match"); match == `"`+entry.Hash+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("X-Koor-Version", formatInt(entry.Version))
	w.WriteHeader(http.StatusOK)
	w.Write(entry.Value)
}

func (s *Server) handleStatePut(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "empty body")
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}

	entry, err := s.stateStore.Put(r.Context(), key, body, ct, "")
	if err != nil {
		s.logger.Error("state put failed", "key", key, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to write state")
		return
	}

	s.logger.Info("state updated", "key", key, "version", entry.Version)
	writeJSON(w, http.StatusOK, map[string]any{
		"key":          entry.Key,
		"version":      entry.Version,
		"hash":         entry.Hash,
		"content_type": entry.ContentType,
		"updated_at":   entry.UpdatedAt,
	})
}

func (s *Server) handleStateDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	err := s.stateStore.Delete(r.Context(), key)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "key not found: "+key)
		return
	}
	if err != nil {
		s.logger.Error("state delete failed", "key", key, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete state")
		return
	}

	s.logger.Info("state deleted", "key", key)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": key})
}

// --- Specs handlers ---

func (s *Server) handleSpecsList(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	items, err := s.specReg.List(r.Context(), project)
	if err != nil {
		s.logger.Error("specs list failed", "project", project, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list specs")
		return
	}
	if items == nil {
		items = []specs.Summary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project": project,
		"specs":   items,
	})
}

func (s *Server) handleSpecsGet(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	name := r.PathValue("name")

	spec, err := s.specReg.Get(r.Context(), project, name)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "spec not found: "+project+"/"+name)
		return
	}
	if err != nil {
		s.logger.Error("specs get failed", "project", project, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get spec")
		return
	}

	// Support ETag caching.
	w.Header().Set("ETag", `"`+spec.Hash+`"`)
	if match := r.Header.Get("If-None-Match"); match == `"`+spec.Hash+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Koor-Version", formatInt(spec.Version))
	w.WriteHeader(http.StatusOK)
	w.Write(spec.Data)
}

func (s *Server) handleSpecsPut(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	name := r.PathValue("name")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "empty body")
		return
	}

	spec, err := s.specReg.Put(r.Context(), project, name, body)
	if err != nil {
		s.logger.Error("specs put failed", "project", project, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to write spec")
		return
	}

	s.logger.Info("spec updated", "project", project, "name", name, "version", spec.Version)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":    spec.Project,
		"name":       spec.Name,
		"version":    spec.Version,
		"hash":       spec.Hash,
		"updated_at": spec.UpdatedAt,
	})
}

func (s *Server) handleSpecsDelete(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	name := r.PathValue("name")

	err := s.specReg.Delete(r.Context(), project, name)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "spec not found: "+project+"/"+name)
		return
	}
	if err != nil {
		s.logger.Error("specs delete failed", "project", project, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete spec")
		return
	}

	s.logger.Info("spec deleted", "project", project, "name", name)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": project + "/" + name})
}

// --- Events handlers ---

func (s *Server) handleEventsPublish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Topic string          `json:"topic"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}

	ev, err := s.eventBus.Publish(r.Context(), req.Topic, req.Data, "")
	if err != nil {
		s.logger.Error("event publish failed", "topic", req.Topic, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to publish event")
		return
	}

	s.logger.Info("event published", "topic", req.Topic, "id", ev.ID)
	writeJSON(w, http.StatusOK, ev)
}

func (s *Server) handleEventsHistory(w http.ResponseWriter, r *http.Request) {
	last := 50
	if v := r.URL.Query().Get("last"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			last = n
		}
	}
	topic := r.URL.Query().Get("topic")

	history, err := s.eventBus.History(r.Context(), last, topic)
	if err != nil {
		s.logger.Error("event history failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get event history")
		return
	}
	if history == nil {
		history = []events.Event{}
	}
	writeJSON(w, http.StatusOK, history)
}

// --- Instance handlers ---

func (s *Server) handleInstancesList(w http.ResponseWriter, r *http.Request) {
	nameFilter := r.URL.Query().Get("name")
	workspaceFilter := r.URL.Query().Get("workspace")
	stackFilter := r.URL.Query().Get("stack")

	var items []instances.Summary
	var err error

	if nameFilter != "" || workspaceFilter != "" || stackFilter != "" {
		items, err = s.instanceReg.Discover(r.Context(), nameFilter, workspaceFilter, stackFilter)
	} else {
		items, err = s.instanceReg.List(r.Context())
	}
	if err != nil {
		s.logger.Error("instances list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list instances")
		return
	}
	if items == nil {
		items = []instances.Summary{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleInstanceGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	inst, err := s.instanceReg.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "instance not found: "+id)
		return
	}
	if err != nil {
		s.logger.Error("instance get failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get instance")
		return
	}

	// Don't expose token in GET responses.
	writeJSON(w, http.StatusOK, instances.Summary{
		ID:           inst.ID,
		Name:         inst.Name,
		Workspace:    inst.Workspace,
		Intent:       inst.Intent,
		Stack:        inst.Stack,
		RegisteredAt: inst.RegisteredAt,
		LastSeen:     inst.LastSeen,
	})
}

func (s *Server) handleInstanceRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Workspace string `json:"workspace"`
		Intent    string `json:"intent"`
		Stack     string `json:"stack"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	inst, err := s.instanceReg.Register(r.Context(), req.Name, req.Workspace, req.Intent, req.Stack)
	if err != nil {
		s.logger.Error("instance register failed", "name", req.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to register instance")
		return
	}

	s.logger.Info("instance registered", "id", inst.ID, "name", inst.Name)
	writeJSON(w, http.StatusOK, inst)
}

func (s *Server) handleInstanceHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.instanceReg.Heartbeat(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "instance not found: "+id)
		return
	}
	if err != nil {
		s.logger.Error("instance heartbeat failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to heartbeat")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "ok"})
}

func (s *Server) handleInstanceDeregister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.instanceReg.Deregister(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "instance not found: "+id)
		return
	}
	if err != nil {
		s.logger.Error("instance deregister failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to deregister")
		return
	}

	s.logger.Info("instance deregistered", "id", id)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// --- Validation handlers ---

func (s *Server) handleValidateRulesList(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	rules, err := s.specReg.ListRules(r.Context(), project)
	if err != nil {
		s.logger.Error("list rules failed", "project", project, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	if rules == nil {
		rules = []specs.Rule{}
	}

	// Optional stack filter.
	if stackFilter := r.URL.Query().Get("stack"); stackFilter != "" {
		var filtered []specs.Rule
		for _, rule := range rules {
			if rule.Stack == "" || rule.Stack == stackFilter {
				filtered = append(filtered, rule)
			}
		}
		if filtered == nil {
			filtered = []specs.Rule{}
		}
		rules = filtered
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project": project,
		"rules":   rules,
	})
}

func (s *Server) handleValidateRulesPut(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	var rules []specs.Rule
	if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Set project on all rules.
	for i := range rules {
		rules[i].Project = project
	}

	if err := s.specReg.PutRules(r.Context(), project, rules); err != nil {
		s.logger.Error("put rules failed", "project", project, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save rules")
		return
	}

	s.logger.Info("rules updated", "project", project, "count", len(rules))
	writeJSON(w, http.StatusOK, map[string]any{
		"project": project,
		"count":   len(rules),
	})
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	var req specs.ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	violations, err := s.specReg.Validate(r.Context(), project, req)
	if err != nil {
		s.logger.Error("validation failed", "project", project, "error", err)
		writeError(w, http.StatusInternalServerError, "validation failed")
		return
	}
	if violations == nil {
		violations = []specs.Violation{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project":    project,
		"violations": violations,
		"count":      len(violations),
	})
}

// --- Contract validation handlers ---

func (s *Server) handleContractValidate(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	name := r.PathValue("name")

	// Load the contract from specs.
	spec, err := s.specReg.Get(r.Context(), project, name)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "contract not found: "+project+"/"+name)
		return
	}
	if err != nil {
		s.logger.Error("contract get failed", "project", project, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get contract")
		return
	}

	contract, err := contracts.Parse(spec.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "stored spec is not a valid contract: "+err.Error())
		return
	}

	var req struct {
		Endpoint  string         `json:"endpoint"`
		Direction string         `json:"direction"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if req.Direction == "" {
		req.Direction = "request"
	}

	violations := contracts.ValidatePayload(contract, req.Endpoint, req.Direction, req.Payload)
	if violations == nil {
		violations = []contracts.Violation{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":      len(violations) == 0,
		"violations": violations,
	})
}

func (s *Server) handleContractTest(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	name := r.PathValue("name")

	// Load the contract from specs.
	spec, err := s.specReg.Get(r.Context(), project, name)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "contract not found: "+project+"/"+name)
		return
	}
	if err != nil {
		s.logger.Error("contract get failed", "project", project, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get contract")
		return
	}

	contract, err := contracts.Parse(spec.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "stored spec is not a valid contract: "+err.Error())
		return
	}

	var req struct {
		Endpoint string         `json:"endpoint"`
		BaseURL  string         `json:"base_url"`
		TestData map[string]any `json:"test_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if req.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "base_url is required")
		return
	}

	result, err := contracts.TestEndpoint(contract, req.Endpoint, req.BaseURL, req.TestData)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":               len(result.RequestViolations) == 0 && len(result.ResponseViolations) == 0 && result.Error == "",
		"endpoint":            result.Endpoint,
		"status_code":         result.StatusCode,
		"request_violations":  result.RequestViolations,
		"response_violations": result.ResponseViolations,
		"error":               result.Error,
	})
}

// --- Rules management handlers ---

func (s *Server) handleRulesPropose(w http.ResponseWriter, r *http.Request) {
	var rule specs.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if rule.Project == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}
	if rule.RuleID == "" {
		writeError(w, http.StatusBadRequest, "rule_id is required")
		return
	}
	if rule.Pattern == "" {
		writeError(w, http.StatusBadRequest, "pattern is required")
		return
	}

	if err := s.specReg.ProposeRule(r.Context(), rule); err != nil {
		s.logger.Error("propose rule failed", "project", rule.Project, "rule_id", rule.RuleID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to propose rule")
		return
	}

	s.logger.Info("rule proposed", "project", rule.Project, "rule_id", rule.RuleID, "proposed_by", rule.ProposedBy)
	writeJSON(w, http.StatusOK, map[string]any{
		"project": rule.Project,
		"rule_id": rule.RuleID,
		"status":  "proposed",
	})
}

func (s *Server) handleRulesAccept(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	ruleID := r.PathValue("ruleID")

	err := s.specReg.AcceptRule(r.Context(), project, ruleID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "proposed rule not found: "+project+"/"+ruleID)
		return
	}
	if err != nil {
		s.logger.Error("accept rule failed", "project", project, "rule_id", ruleID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to accept rule")
		return
	}

	s.logger.Info("rule accepted", "project", project, "rule_id", ruleID)
	writeJSON(w, http.StatusOK, map[string]any{
		"project": project,
		"rule_id": ruleID,
		"status":  "accepted",
	})
}

func (s *Server) handleRulesReject(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	ruleID := r.PathValue("ruleID")

	err := s.specReg.RejectRule(r.Context(), project, ruleID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "proposed rule not found: "+project+"/"+ruleID)
		return
	}
	if err != nil {
		s.logger.Error("reject rule failed", "project", project, "rule_id", ruleID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to reject rule")
		return
	}

	s.logger.Info("rule rejected", "project", project, "rule_id", ruleID)
	writeJSON(w, http.StatusOK, map[string]any{
		"project": project,
		"rule_id": ruleID,
		"status":  "rejected",
	})
}

func (s *Server) handleRulesExport(w http.ResponseWriter, r *http.Request) {
	sourceParam := r.URL.Query().Get("source")
	var sources []string
	if sourceParam != "" {
		sources = strings.Split(sourceParam, ",")
	}

	rules, err := s.specReg.ExportRules(r.Context(), sources)
	if err != nil {
		s.logger.Error("export rules failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to export rules")
		return
	}
	if rules == nil {
		rules = []specs.Rule{}
	}

	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) handleRulesImport(w http.ResponseWriter, r *http.Request) {
	var rules []specs.Rule
	if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(rules) == 0 {
		writeError(w, http.StatusBadRequest, "empty rules array")
		return
	}

	count, err := s.specReg.ImportRules(r.Context(), rules)
	if err != nil {
		s.logger.Error("import rules failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to import rules")
		return
	}

	s.logger.Info("rules imported", "count", count)
	writeJSON(w, http.StatusOK, map[string]any{
		"imported": count,
	})
}

// --- Metrics handler ---

// estimatedTokensPerMCPCall is the estimated tokens consumed per MCP tool call
// (tool call + response flowing through the LLM context window).
const estimatedTokensPerMCPCall = 300

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Gather basic system metrics.
	stateItems, _ := s.stateStore.List(r.Context())
	instanceItems, _ := s.instanceReg.List(r.Context())
	recentEvents, _ := s.eventBus.History(r.Context(), 1, "")

	lastEventID := int64(0)
	if len(recentEvents) > 0 {
		lastEventID = recentEvents[0].ID
	}

	// Token tax calculations.
	mcpCount := s.mcpCalls.Load()
	restCount := s.restCalls.Load()
	total := mcpCount + restCount
	savingsPercent := 0.0
	if total > 0 {
		savingsPercent = float64(restCount) / float64(total) * 100
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"uptime":         time.Since(s.startTime).Truncate(time.Second).String(),
		"state_keys":     len(stateItems),
		"instances":      len(instanceItems),
		"last_event_id":  lastEventID,
		"api_bind":       s.config.Bind,
		"dashboard_bind": s.config.DashboardBind,
		"token_tax": map[string]any{
			"mcp_calls":            mcpCount,
			"rest_calls":           restCount,
			"total_calls":          total,
			"mcp_estimated_tokens": mcpCount * estimatedTokensPerMCPCall,
			"rest_tokens_saved":    restCount * estimatedTokensPerMCPCall,
			"savings_percent":      savingsPercent,
		},
	})
}

// --- Dashboard HTMX handlers ---

// handleDashboardRules renders the full rules page.
func (s *Server) handleDashboardRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	proposed, _ := s.specReg.ListAllRules(ctx, "", "", "", "proposed")
	accepted, _ := s.specReg.ListAllRules(ctx, "", "", "", "accepted")

	data := struct {
		Proposed []specs.Rule
		Rules    []specs.Rule
	}{proposed, accepted}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboard.Templates.ExecuteTemplate(w, "rules.html", data); err != nil {
		s.logger.Error("render rules page", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// handleDashboardRulesList renders the rules table fragment (HTMX partial).
func (s *Server) handleDashboardRulesList(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	stack := r.URL.Query().Get("stack")
	source := r.URL.Query().Get("source")
	status := r.URL.Query().Get("status")

	// Default to accepted rules if no status filter.
	if status == "" {
		status = "accepted"
	}

	rules, err := s.specReg.ListAllRules(r.Context(), project, stack, source, status)
	if err != nil {
		s.logger.Error("dashboard list rules", "error", err)
		http.Error(w, "failed to list rules", http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []specs.Rule{}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboard.Templates.ExecuteTemplate(w, "rules_table.html", rules); err != nil {
		s.logger.Error("render rules table", "error", err)
	}
}

// handleDashboardRuleForm renders the add/edit rule form (HTMX partial).
func (s *Server) handleDashboardRuleForm(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	ruleID := r.URL.Query().Get("rule_id")

	var rule specs.Rule
	if project != "" && ruleID != "" {
		got, err := s.specReg.GetRule(r.Context(), project, ruleID)
		if err == nil {
			rule = *got
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboard.Templates.ExecuteTemplate(w, "rule_form.html", rule); err != nil {
		s.logger.Error("render rule form", "error", err)
	}
}

// handleDashboardRuleSave saves a rule from the form and re-renders the table.
func (s *Server) handleDashboardRuleSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	rule := specs.Rule{
		Project:   r.FormValue("project"),
		RuleID:    r.FormValue("rule_id"),
		Severity:  r.FormValue("severity"),
		MatchType: r.FormValue("match_type"),
		Pattern:   r.FormValue("pattern"),
		Message:   r.FormValue("message"),
		Stack:     r.FormValue("stack"),
		Source:    r.FormValue("source"),
	}
	if rule.Source == "" {
		rule.Source = "local"
	}

	if _, err := s.specReg.ImportRules(r.Context(), []specs.Rule{rule}); err != nil {
		s.logger.Error("dashboard save rule", "error", err)
		http.Error(w, "failed to save rule", http.StatusInternalServerError)
		return
	}

	// Re-render the full table.
	rules, _ := s.specReg.ListAllRules(r.Context(), "", "", "", "accepted")
	if rules == nil {
		rules = []specs.Rule{}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dashboard.Templates.ExecuteTemplate(w, "rules_table.html", rules)
}

// handleDashboardRuleDelete deletes a rule and returns empty (HTMX removes the row).
func (s *Server) handleDashboardRuleDelete(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	ruleID := r.PathValue("ruleID")

	if err := s.specReg.DeleteRule(r.Context(), project, ruleID); err != nil {
		s.logger.Error("dashboard delete rule", "project", project, "rule_id", ruleID, "error", err)
		http.Error(w, "failed to delete rule", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleDashboardRuleAccept accepts a proposed rule (HTMX removes the proposed item).
func (s *Server) handleDashboardRuleAccept(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	ruleID := r.PathValue("ruleID")

	if err := s.specReg.AcceptRule(r.Context(), project, ruleID); err != nil {
		s.logger.Error("dashboard accept rule", "project", project, "rule_id", ruleID, "error", err)
		http.Error(w, "failed to accept rule", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleDashboardRuleReject rejects a proposed rule (HTMX removes the proposed item).
func (s *Server) handleDashboardRuleReject(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	ruleID := r.PathValue("ruleID")

	if err := s.specReg.RejectRule(r.Context(), project, ruleID); err != nil {
		s.logger.Error("dashboard reject rule", "project", project, "rule_id", ruleID, "error", err)
		http.Error(w, "failed to reject rule", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// formatInt converts an int64 to string for headers.
func formatInt(n int64) string {
	return strconv.FormatInt(n, 10)
}


package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"encoding/json"
	"strconv"

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

// Handler returns the root HTTP handler with all routes and auth middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health (no auth required, registered on outer mux below).
	// State endpoints.
	mux.HandleFunc("GET /api/state", s.handleStateList)
	mux.HandleFunc("GET /api/state/{key}", s.handleStateGet)
	mux.HandleFunc("PUT /api/state/{key}", s.handleStatePut)
	mux.HandleFunc("DELETE /api/state/{key}", s.handleStateDelete)

	// Specs endpoints.
	mux.HandleFunc("GET /api/specs/{project}", s.handleSpecsList)
	mux.HandleFunc("GET /api/specs/{project}/{name}", s.handleSpecsGet)
	mux.HandleFunc("PUT /api/specs/{project}/{name}", s.handleSpecsPut)
	mux.HandleFunc("DELETE /api/specs/{project}/{name}", s.handleSpecsDelete)

	// Events endpoints.
	mux.HandleFunc("POST /api/events/publish", s.handleEventsPublish)
	mux.HandleFunc("GET /api/events/history", s.handleEventsHistory)
	mux.Handle("GET /api/events/subscribe", events.ServeSubscribe(s.eventBus, s.logger))

	// Instance endpoints.
	mux.HandleFunc("GET /api/instances", s.handleInstancesList)
	mux.HandleFunc("GET /api/instances/{id}", s.handleInstanceGet)
	mux.HandleFunc("POST /api/instances/register", s.handleInstanceRegister)
	mux.HandleFunc("POST /api/instances/{id}/heartbeat", s.handleInstanceHeartbeat)
	mux.HandleFunc("DELETE /api/instances/{id}", s.handleInstanceDeregister)

	// Validation endpoints.
	mux.HandleFunc("GET /api/validate/{project}/rules", s.handleValidateRulesList)
	mux.HandleFunc("PUT /api/validate/{project}/rules", s.handleValidateRulesPut)
	mux.HandleFunc("POST /api/validate/{project}", s.handleValidate)

	// Metrics endpoint.
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)

	// MCP endpoint (StreamableHTTP).
	if s.mcpHandler != nil {
		mux.Handle("/mcp", s.mcpHandler)
	}

	// Outer mux: health is public, everything else goes through auth.
	outer := http.NewServeMux()
	outer.HandleFunc("GET /health", s.handleHealth)
	outer.Handle("/", authMiddleware(s.config.AuthToken, mux))

	return outer
}

// DashboardHandler returns the HTTP handler for the dashboard (separate port).
// It proxies /api/* and /health to the API server, and serves embedded static files for everything else.
func (s *Server) DashboardHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/", s.dashboardProxy)
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
	items, err := s.instanceReg.List(r.Context())
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
		RegisteredAt: inst.RegisteredAt,
		LastSeen:     inst.LastSeen,
	})
}

func (s *Server) handleInstanceRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Workspace string `json:"workspace"`
		Intent    string `json:"intent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	inst, err := s.instanceReg.Register(r.Context(), req.Name, req.Workspace, req.Intent)
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

// --- Metrics handler ---

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Gather basic system metrics.
	stateItems, _ := s.stateStore.List(r.Context())
	instanceItems, _ := s.instanceReg.List(r.Context())
	recentEvents, _ := s.eventBus.History(r.Context(), 1, "")

	lastEventID := int64(0)
	if len(recentEvents) > 0 {
		lastEventID = recentEvents[0].ID
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"uptime":          time.Since(s.startTime).Truncate(time.Second).String(),
		"state_keys":      len(stateItems),
		"instances":       len(instanceItems),
		"last_event_id":   lastEventID,
		"api_bind":        s.config.Bind,
		"dashboard_bind":  s.config.DashboardBind,
	})
}

// formatInt converts an int64 to string for headers.
func formatInt(n int64) string {
	return strconv.FormatInt(n, 10)
}


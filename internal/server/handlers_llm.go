package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/DavidRHerbert/koor/internal/llmcost"
)

// --- LLM cost tracking handlers ---

func (s *Server) handleLLMUsageRecord(w http.ResponseWriter, r *http.Request) {
	if s.llmCostStore == nil {
		writeError(w, http.StatusServiceUnavailable, "llm cost tracking not configured")
		return
	}

	var req struct {
		InstanceID  string  `json:"instance_id"`
		Project     string  `json:"project"`
		Provider    string  `json:"provider"`
		Model       string  `json:"model"`
		TokensIn    int64   `json:"tokens_in"`
		TokensOut   int64   `json:"tokens_out"`
		CostUSD     float64 `json:"cost_usd"`
		RequestType string  `json:"request_type"`
		SessionTag  string  `json:"session_tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	rec := llmcost.UsageRecord{
		InstanceID:  req.InstanceID,
		Project:     req.Project,
		Provider:    req.Provider,
		Model:       req.Model,
		TokensIn:    req.TokensIn,
		TokensOut:   req.TokensOut,
		CostUSD:     req.CostUSD,
		RequestType: req.RequestType,
		SessionTag:  req.SessionTag,
	}
	if rec.RequestType == "" {
		rec.RequestType = "completion"
	}

	id, err := s.llmCostStore.Record(r.Context(), rec)
	if err != nil {
		s.logger.Error("llm usage record failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to record llm usage")
		return
	}

	// Publish event.
	data, _ := json.Marshal(map[string]any{
		"instance_id": rec.InstanceID,
		"project":     rec.Project,
		"model":       rec.Model,
		"tokens_in":   rec.TokensIn,
		"tokens_out":  rec.TokensOut,
		"cost_usd":    rec.CostUSD,
		"session_tag": rec.SessionTag,
	})
	s.eventBus.Publish(r.Context(), "llm.usage.recorded", json.RawMessage(data), "llmcost")

	// Audit log.
	if s.auditLog != nil {
		detail, _ := json.Marshal(map[string]any{
			"tokens_in":  rec.TokensIn,
			"tokens_out": rec.TokensOut,
			"cost_usd":   rec.CostUSD,
		})
		s.auditLog.Append(r.Context(), rec.InstanceID, "llm.usage", rec.Project+"/"+rec.Model, string(detail), "success")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":       id,
		"recorded": true,
	})
}

func (s *Server) handleLLMUsageQuery(w http.ResponseWriter, r *http.Request) {
	if s.llmCostStore == nil {
		writeError(w, http.StatusServiceUnavailable, "llm cost tracking not configured")
		return
	}

	q := r.URL.Query()
	instance := q.Get("instance")
	project := q.Get("project")
	sessionTag := q.Get("session_tag")
	from := q.Get("from")
	to := q.Get("to")
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	var records []llmcost.UsageRecord
	var err error

	switch {
	case instance != "":
		records, err = s.llmCostStore.QueryByInstance(r.Context(), instance, from, to, limit)
	case project != "":
		records, err = s.llmCostStore.QueryByProject(r.Context(), project, from, to, limit)
	case sessionTag != "":
		records, err = s.llmCostStore.QueryBySessionTag(r.Context(), sessionTag, from, to, limit)
	default:
		records, err = s.llmCostStore.QueryAll(r.Context(), from, to, limit)
	}
	if err != nil {
		s.logger.Error("llm usage query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query llm usage")
		return
	}
	if records == nil {
		records = []llmcost.UsageRecord{}
	}

	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleLLMUsageSummary(w http.ResponseWriter, r *http.Request) {
	if s.llmCostStore == nil {
		writeError(w, http.StatusServiceUnavailable, "llm cost tracking not configured")
		return
	}

	q := r.URL.Query()
	groupBy := q.Get("group_by")
	if groupBy == "" {
		groupBy = "project"
	}
	from := q.Get("from")
	to := q.Get("to")

	var groups map[string]llmcost.UsageSummary
	var err error

	switch groupBy {
	case "instance":
		groups, err = s.llmCostStore.SummarizeByInstance(r.Context(), from, to)
	case "project":
		groups, err = s.llmCostStore.SummarizeByProject(r.Context(), from, to)
	case "model":
		groups, err = s.llmCostStore.SummarizeByModel(r.Context(), from, to)
	case "session_tag":
		groups, err = s.llmCostStore.SummarizeBySessionTag(r.Context(), from, to)
	default:
		writeError(w, http.StatusBadRequest, "invalid group_by: must be instance, project, model, or session_tag")
		return
	}
	if err != nil {
		s.logger.Error("llm usage summary failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to summarize llm usage")
		return
	}
	if groups == nil {
		groups = map[string]llmcost.UsageSummary{}
	}

	total, err := s.llmCostStore.Total(r.Context(), from, to)
	if err != nil {
		s.logger.Error("llm usage total failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get llm usage total")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"group_by": groupBy,
		"from":     from,
		"to":       to,
		"groups":   groups,
		"total":    total,
	})
}

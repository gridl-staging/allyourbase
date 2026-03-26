// Package server Admin HTTP handlers for AI management, providing endpoints for AI usage analytics, call logs, and full CRUD operations on prompt templates.
package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/google/uuid"
)

// aiLogStore reads AI call logs. ai.PgLogStore satisfies this interface.
type aiLogStore interface {
	List(ctx context.Context, filter ai.ListFilter) ([]ai.CallLog, int, error)
	UsageSummary(ctx context.Context, from, to time.Time) (ai.UsageSummary, error)
	DailyUsage(ctx context.Context, from, to time.Time) ([]ai.DailyUsage, error)
}

// promptStore manages AI prompt templates. ai.PgPromptStore satisfies this interface.
type promptStore interface {
	Create(ctx context.Context, req ai.CreatePromptRequest) (ai.Prompt, error)
	Get(ctx context.Context, id uuid.UUID) (ai.Prompt, error)
	List(ctx context.Context, page, perPage int) ([]ai.Prompt, int, error)
	Update(ctx context.Context, id uuid.UUID, req ai.UpdatePromptRequest) (ai.Prompt, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListVersions(ctx context.Context, promptID uuid.UUID) ([]ai.PromptVersion, error)
}

// --- AI call log endpoints ---

// handleAdminAILogs lists AI call logs with optional filtering by provider, status, and time range, supporting pagination via page and per_page query parameters.
func (s *Server) handleAdminAILogs(w http.ResponseWriter, r *http.Request) {
	if s.aiLogStore == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"logs": []ai.CallLog{}, "total": 0})
		return
	}

	q := r.URL.Query()
	filter := ai.ListFilter{
		Page:     parseIntParam(q.Get("page"), 1),
		PerPage:  parseIntParam(q.Get("per_page"), 20),
		Provider: q.Get("provider"),
		Status:   q.Get("status"),
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = t
		}
	}

	logs, total, err := s.aiLogStore.List(r.Context(), filter)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = []ai.CallLog{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"logs": logs, "total": total})
}

// handleAdminAIUsage retrieves aggregated AI usage statistics by provider within an optional time range specified via from and to query parameters.
func (s *Server) handleAdminAIUsage(w http.ResponseWriter, r *http.Request) {
	if s.aiLogStore == nil {
		httputil.WriteJSON(w, http.StatusOK, ai.UsageSummary{
			ByProvider: map[string]ai.ProviderUsage{},
		})
		return
	}

	q := r.URL.Query()
	from := time.Time{}
	to := time.Now().UTC()
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	summary, err := s.aiLogStore.UsageSummary(r.Context(), from, to)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if summary.ByProvider == nil {
		summary.ByProvider = map[string]ai.ProviderUsage{}
	}
	httputil.WriteJSON(w, http.StatusOK, summary)
}

// handleAdminAIUsageDaily returns daily breakdowns of AI usage within an optional time range specified via from and to query parameters.
func (s *Server) handleAdminAIUsageDaily(w http.ResponseWriter, r *http.Request) {
	if s.aiLogStore == nil {
		httputil.WriteJSON(w, http.StatusOK, []ai.DailyUsage{})
		return
	}

	q := r.URL.Query()
	from := time.Time{}
	to := time.Now().UTC()
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	rows, err := s.aiLogStore.DailyUsage(r.Context(), from, to)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []ai.DailyUsage{}
	}
	httputil.WriteJSON(w, http.StatusOK, rows)
}

// --- Prompt management endpoints ---

// handleAdminPromptList lists prompt templates with pagination controlled by page and per_page query parameters.
func (s *Server) handleAdminPromptList(w http.ResponseWriter, r *http.Request) {
	if s.promptStore == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"prompts": []ai.Prompt{}, "total": 0})
		return
	}

	q := r.URL.Query()
	page := parseIntParam(q.Get("page"), 1)
	perPage := parseIntParam(q.Get("per_page"), 20)

	prompts, total, err := s.promptStore.List(r.Context(), page, perPage)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if prompts == nil {
		prompts = []ai.Prompt{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"prompts": prompts, "total": total})
}

// handleAdminPromptCreate creates a new prompt template from a JSON request containing required name and template fields.
func (s *Server) handleAdminPromptCreate(w http.ResponseWriter, r *http.Request) {
	if s.promptStore == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "AI prompt store not configured")
		return
	}

	var req ai.CreatePromptRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.Template == "" {
		httputil.WriteError(w, http.StatusBadRequest, "name and template are required")
		return
	}

	p, err := s.promptStore.Create(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, p)
}

// handleAdminPromptGet retrieves a single prompt template by its ID from the URL path.
func (s *Server) handleAdminPromptGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParamWithLabel(w, r, "id", "prompt id")
	if !ok {
		return
	}
	if s.promptStore == nil {
		httputil.WriteError(w, http.StatusNotFound, "prompt not found")
		return
	}

	p, err := s.promptStore.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			httputil.WriteError(w, http.StatusNotFound, "prompt not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

// handleAdminPromptUpdate modifies an existing prompt template identified by ID in the URL path using fields from the JSON request body.
func (s *Server) handleAdminPromptUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParamWithLabel(w, r, "id", "prompt id")
	if !ok {
		return
	}
	if s.promptStore == nil {
		httputil.WriteError(w, http.StatusNotFound, "prompt not found")
		return
	}

	var req ai.UpdatePromptRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	p, err := s.promptStore.Update(r.Context(), id, req)
	if err != nil {
		if isNotFound(err) {
			httputil.WriteError(w, http.StatusNotFound, "prompt not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

// handleAdminPromptDelete removes a prompt template identified by ID from the URL path, returning no content on success.
func (s *Server) handleAdminPromptDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParamWithLabel(w, r, "id", "prompt id")
	if !ok {
		return
	}
	if s.promptStore == nil {
		httputil.WriteError(w, http.StatusNotFound, "prompt not found")
		return
	}

	if err := s.promptStore.Delete(r.Context(), id); err != nil {
		if isNotFound(err) {
			httputil.WriteError(w, http.StatusNotFound, "prompt not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminPromptVersions lists the complete version history of a prompt template identified by ID from the URL path.
func (s *Server) handleAdminPromptVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParamWithLabel(w, r, "id", "prompt id")
	if !ok {
		return
	}
	if s.promptStore == nil {
		httputil.WriteJSON(w, http.StatusOK, []ai.PromptVersion{})
		return
	}

	versions, err := s.promptStore.ListVersions(r.Context(), id)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if versions == nil {
		versions = []ai.PromptVersion{}
	}
	httputil.WriteJSON(w, http.StatusOK, versions)
}

type promptRenderRequest struct {
	Variables map[string]any `json:"variables"`
}

// handleAdminPromptRender renders a prompt template with variables provided in the request body, returning the rendered output and original prompt metadata.
func (s *Server) handleAdminPromptRender(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParamWithLabel(w, r, "id", "prompt id")
	if !ok {
		return
	}
	if s.promptStore == nil {
		httputil.WriteError(w, http.StatusNotFound, "prompt not found")
		return
	}

	var req promptRenderRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Variables == nil {
		req.Variables = map[string]any{}
	}

	p, err := s.promptStore.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			httputil.WriteError(w, http.StatusNotFound, "prompt not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	vars := ai.ApplyDefaults(p.Variables, req.Variables)
	rendered, err := ai.RenderPrompt(p.Template, vars)
	if err != nil {
		httputil.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"rendered": rendered, "prompt": p})
}

// --- helpers ---

func parseIntParam(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return def
	}
	return v
}

func isNotFound(err error) bool {
	return errors.Is(err, errNotFound) || (err != nil && err.Error() == "prompt not found")
}

var errNotFound = errors.New("not found")

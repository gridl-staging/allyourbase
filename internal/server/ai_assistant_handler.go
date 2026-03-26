// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/ai_assistant_handler.go.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/httputil"
)

const assistantMaxQueryLength = 2000

type assistantService interface {
	Execute(ctx context.Context, req ai.AssistantRequest) (ai.AssistantResponse, error)
	ExecuteStream(ctx context.Context, req ai.AssistantRequest, onChunk func(string) error) (ai.AssistantResponse, error)
	ListHistory(ctx context.Context, filter ai.AssistantHistoryFilter) ([]ai.AssistantHistoryEntry, int, error)
}

type aiAssistantRequest struct {
	Query    string `json:"query"`
	Mode     string `json:"mode,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// TODO: Document Server.handleAdminAIAssistant.
func (s *Server) handleAdminAIAssistant(w http.ResponseWriter, r *http.Request) {
	if s.assistantSvc == nil {
		httputil.WriteError(w, http.StatusNotFound, ai.ErrAssistantDisabled.Error())
		return
	}

	req, ok := decodeAssistantRequest(w, r)
	if !ok {
		return
	}

	resp, err := s.assistantSvc.Execute(r.Context(), req)
	if err != nil {
		handleAssistantServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// TODO: Document Server.handleAdminAIAssistantStream.
func (s *Server) handleAdminAIAssistantStream(w http.ResponseWriter, r *http.Request) {
	if s.assistantSvc == nil {
		httputil.WriteError(w, http.StatusNotFound, ai.ErrAssistantDisabled.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "streaming is not supported by this server")
		return
	}

	req, ok := decodeAssistantRequest(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if err := writeSSEJSON(w, flusher, "start", map[string]any{"mode": req.Mode, "provider": req.Provider, "model": req.Model}); err != nil {
		return
	}

	resp, err := s.assistantSvc.ExecuteStream(r.Context(), req, func(chunk string) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		if chunk == "" {
			return nil
		}
		return writeSSEJSON(w, flusher, "chunk", map[string]any{"text": chunk})
	})
	if err != nil {
		if isCanceledStreamRequest(r.Context()) {
			return
		}
		status, message := mapAssistantServiceError(err)
		_ = writeSSEJSON(w, flusher, "error", map[string]any{"code": status, "message": message})
		return
	}
	_ = writeSSEJSON(w, flusher, "done", resp)
}

// TODO: Document Server.handleAdminAIAssistantHistory.
func (s *Server) handleAdminAIAssistantHistory(w http.ResponseWriter, r *http.Request) {
	if s.assistantSvc == nil {
		httputil.WriteError(w, http.StatusNotFound, ai.ErrAssistantDisabled.Error())
		return
	}

	q := r.URL.Query()
	mode := ai.AssistantMode(strings.TrimSpace(strings.ToLower(q.Get("mode"))))
	if mode != "" && !ai.IsValidAssistantMode(mode) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid mode")
		return
	}
	filter := ai.AssistantHistoryFilter{
		Mode:    mode,
		Page:    parseIntParam(q.Get("page"), 1),
		PerPage: parseIntParam(q.Get("per_page"), 20),
	}

	history, total, err := s.assistantSvc.ListHistory(r.Context(), filter)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if history == nil {
		history = []ai.AssistantHistoryEntry{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"history": history, "total": total})
}

// TODO: Document decodeAssistantRequest.
func decodeAssistantRequest(w http.ResponseWriter, r *http.Request) (ai.AssistantRequest, bool) {
	var body aiAssistantRequest
	if !httputil.DecodeJSON(w, r, &body) {
		return ai.AssistantRequest{}, false
	}

	query := strings.TrimSpace(body.Query)
	if query == "" {
		httputil.WriteError(w, http.StatusBadRequest, "query is required")
		return ai.AssistantRequest{}, false
	}
	if utf8.RuneCountInString(query) > assistantMaxQueryLength {
		httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("query must be at most %d characters", assistantMaxQueryLength))
		return ai.AssistantRequest{}, false
	}

	mode := ai.AssistantMode(strings.TrimSpace(strings.ToLower(body.Mode)))
	if mode != "" && !ai.IsValidAssistantMode(mode) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid mode")
		return ai.AssistantRequest{}, false
	}

	return ai.AssistantRequest{
		Query:    query,
		Mode:     mode,
		Provider: strings.TrimSpace(body.Provider),
		Model:    strings.TrimSpace(body.Model),
	}, true
}

func handleAssistantServiceError(w http.ResponseWriter, err error) {
	status, message := mapAssistantServiceError(err)
	httputil.WriteError(w, status, message)
}

func mapAssistantServiceError(err error) (int, string) {
	switch {
	case errors.Is(err, ai.ErrAssistantDisabled):
		return http.StatusNotFound, ai.ErrAssistantDisabled.Error()
	case errors.Is(err, ai.ErrAssistantNotConfigured):
		return http.StatusServiceUnavailable, ai.ErrAssistantNotConfigured.Error()
	case errors.Is(err, ai.ErrAssistantSchemaCacheNotReady):
		return http.StatusServiceUnavailable, ai.ErrAssistantSchemaCacheNotReady.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}

func writeSSEJSON(w http.ResponseWriter, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func isCanceledStreamRequest(requestContext context.Context) bool {
	requestErr := requestContext.Err()
	return errors.Is(requestErr, context.Canceled) || errors.Is(requestErr, context.DeadlineExceeded)
}

// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/tenant_audit_handler.go.
package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type tenantAuditListResult struct {
	Items  []tenant.TenantAuditEvent `json:"items"`
	Count  int                       `json:"count"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

type sharedAuditFilters struct {
	from    *time.Time
	to      *time.Time
	action  string
	result  string
	actorID string
	limit   int
	offset  int
}

type auditFilterParseResult struct {
	tenantID string
	sharedAuditFilters
}

// parseAuditFilters extracts and validates audit query parameters from the HTTP request, including required tenant ID and optional filters for time range, pagination, and actor information.
func parseAuditFilters(r *http.Request) (*auditFilterParseResult, error) {
	tenantID := chi.URLParam(r, "tenantId")
	if tenantID == "" {
		return nil, &auditParseError{code: http.StatusBadRequest, message: "tenant id is required"}
	}
	if !httputil.IsValidUUID(tenantID) {
		return nil, &auditParseError{code: http.StatusBadRequest, message: "invalid tenant id format"}
	}

	filters, err := parseSharedAuditFilters(r)
	if err != nil {
		return nil, err
	}

	return &auditFilterParseResult{tenantID: tenantID, sharedAuditFilters: filters}, nil
}

// TODO: Document parseSharedAuditFilters.
func parseSharedAuditFilters(r *http.Request) (sharedAuditFilters, error) {
	result := sharedAuditFilters{
		limit:  50,
		offset: 0,
	}
	result.action = r.URL.Query().Get("action")
	result.result = r.URL.Query().Get("result")

	if err := parseAuditFrom(r, &result); err != nil {
		return sharedAuditFilters{}, err
	}
	if err := parseAuditTo(r, &result); err != nil {
		return sharedAuditFilters{}, err
	}
	if err := parseAuditLimit(r, &result); err != nil {
		return sharedAuditFilters{}, err
	}
	if err := parseAuditOffset(r, &result); err != nil {
		return sharedAuditFilters{}, err
	}
	if err := parseAuditActorID(r, &result); err != nil {
		return sharedAuditFilters{}, err
	}
	if result.from != nil && result.to != nil && result.to.Before(*result.from) {
		return sharedAuditFilters{}, &auditParseError{code: http.StatusBadRequest, message: "'to' must be after 'from'"}
	}
	return result, nil
}

func parseAuditFrom(r *http.Request, filters *sharedAuditFilters) error {
	fromStr := r.URL.Query().Get("from")
	if fromStr == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return &auditParseError{code: http.StatusBadRequest, message: "invalid 'from' timestamp format (use RFC3339)"}
	}
	filters.from = &t
	return nil
}

func parseAuditTo(r *http.Request, filters *sharedAuditFilters) error {
	toStr := r.URL.Query().Get("to")
	if toStr == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return &auditParseError{code: http.StatusBadRequest, message: "invalid 'to' timestamp format (use RFC3339)"}
	}
	filters.to = &t
	return nil
}

// parseLimit parses the limit query parameter, using a default of 50 and clamping valid values to at most 1000.
func parseAuditLimit(r *http.Request, filters *sharedAuditFilters) error {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return nil
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return &auditParseError{code: http.StatusBadRequest, message: "invalid 'limit' value"}
	}
	if limit < 0 {
		return &auditParseError{code: http.StatusBadRequest, message: "'limit' must be non-negative"}
	}
	if limit == 0 {
		filters.limit = 50
	} else if limit > 1000 {
		filters.limit = 1000
	} else {
		filters.limit = limit
	}
	return nil
}

func parseAuditOffset(r *http.Request, filters *sharedAuditFilters) error {
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		return nil
	}
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return &auditParseError{code: http.StatusBadRequest, message: "invalid 'offset' value"}
	}
	if offset < 0 {
		return &auditParseError{code: http.StatusBadRequest, message: "'offset' must be non-negative"}
	}
	filters.offset = offset
	return nil
}

func parseAuditActorID(r *http.Request, filters *sharedAuditFilters) error {
	actorID := r.URL.Query().Get("actor_id")
	if actorID == "" {
		return nil
	}
	if !httputil.IsValidUUID(actorID) {
		return &auditParseError{code: http.StatusBadRequest, message: "invalid 'actor_id' format"}
	}
	filters.actorID = actorID
	return nil
}

func (p *auditFilterParseResult) toQuery() tenant.AuditQuery {
	return tenant.AuditQuery{
		TenantID: p.tenantID,
		From:     p.from,
		To:       p.to,
		Action:   p.action,
		Result:   p.result,
		ActorID:  p.actorID,
		Limit:    p.limit,
		Offset:   p.offset,
	}
}

type auditParseError struct {
	code    int
	message string
}

func (e *auditParseError) Error() string {
	return e.message
}

// tenantAuditQueryHandler returns an HTTP handler that processes GET requests for tenant audit events, applying optional filters and returning paginated results as JSON.
func tenantAuditQueryHandler(svc tenantAuditQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httputil.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		parser, err := parseAuditFilters(r)
		if err != nil {
			if parseErr, ok := err.(*auditParseError); ok {
				httputil.WriteError(w, parseErr.code, parseErr.message)
			} else {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
			}
			return
		}

		query := parser.toQuery()
		events, err := svc.QueryAuditEvents(r.Context(), query)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to query audit events")
			return
		}

		result := tenantAuditListResult{
			Items:  events,
			Count:  len(events),
			Limit:  parser.limit,
			Offset: parser.offset,
		}
		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

type tenantAuditQueryService interface {
	QueryAuditEvents(ctx context.Context, query tenant.AuditQuery) ([]tenant.TenantAuditEvent, error)
}

func (s *Server) handleAdminTenantAudit(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.tenantSvc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "tenant service not configured")
		return
	}
	querySvc, ok := s.tenantSvc.(tenantAuditQueryService)
	if !ok || querySvc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "tenant audit service not configured")
		return
	}
	tenantAuditQueryHandler(querySvc).ServeHTTP(w, r)
}

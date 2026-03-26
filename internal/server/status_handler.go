// Package server provides HTTP handlers for the operational status and incident management API endpoints.
package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	statuspkg "github.com/allyourbase/ayb/internal/status"
	"github.com/go-chi/chi/v5"
)

type statusResponse struct {
	Status    statuspkg.ServiceStatus `json:"status"`
	Services  []statuspkg.ProbeResult `json:"services"`
	Incidents []statuspkg.Incident    `json:"incidents"`
	CheckedAt time.Time               `json:"checkedAt"`
}

type createIncidentRequest struct {
	Title            string   `json:"title"`
	Status           string   `json:"status"`
	AffectedServices []string `json:"affected_services"`
}

type updateIncidentRequest struct {
	Title  *string `json:"title"`
	Status *string `json:"status"`
}

type addIncidentUpdateRequest struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// handlePublicStatus returns an HTTP handler that serves the current operational status snapshot and list of active incidents.
func handlePublicStatus(history *statuspkg.StatusHistory, store statuspkg.IncidentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := statuspkg.StatusSnapshot{
			Status:    statuspkg.Operational,
			Services:  []statuspkg.ProbeResult{},
			CheckedAt: time.Now().UTC(),
		}
		if history != nil {
			if latest := history.Latest(); latest != nil {
				snapshot = *latest
				if snapshot.Services == nil {
					snapshot.Services = []statuspkg.ProbeResult{}
				}
			}
		}

		incidents := []statuspkg.Incident{}
		if store != nil {
			activeIncidents, err := store.ListIncidents(r.Context(), true)
			if err != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "failed to list incidents")
				return
			}
			incidents = activeIncidents
		}

		httputil.WriteJSON(w, http.StatusOK, statusResponse{
			Status:    snapshot.Status,
			Services:  snapshot.Services,
			Incidents: incidents,
			CheckedAt: snapshot.CheckedAt,
		})
	}
}

// handleCreateIncident returns an HTTP handler that creates a new incident with the provided title, status, and affected services.
func handleCreateIncident(store statuspkg.IncidentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireIncidentStore(w, store) {
			return
		}

		var req createIncidentRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			httputil.WriteError(w, http.StatusBadRequest, "title is required")
			return
		}

		incidentStatus := statuspkg.IncidentInvestigating
		if strings.TrimSpace(req.Status) != "" {
			parsed, ok := parseIncidentStatusOrWriteError(w, req.Status)
			if !ok {
				return
			}
			incidentStatus = parsed
		}

		incident := &statuspkg.Incident{
			Title:            req.Title,
			Status:           incidentStatus,
			AffectedServices: normalizeAffectedServices(req.AffectedServices),
		}
		if err := store.CreateIncident(r.Context(), incident); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create incident")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, incident)
	}
}

// handleListIncidents returns an HTTP handler that lists incidents, optionally filtered to active incidents only via the active query parameter.
func handleListIncidents(store statuspkg.IncidentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireIncidentStore(w, store) {
			return
		}

		activeOnly := false
		if raw := strings.TrimSpace(r.URL.Query().Get("active")); raw != "" {
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "invalid active query parameter")
				return
			}
			activeOnly = parsed
		}

		incidents, err := store.ListIncidents(r.Context(), activeOnly)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list incidents")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, incidents)
	}
}

// handleUpdateIncident returns an HTTP handler that updates an incident's title and status, automatically setting the resolved timestamp when the status becomes resolved.
func handleUpdateIncident(store statuspkg.IncidentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireIncidentStore(w, store) {
			return
		}

		incidentID, ok := parseIncidentIDOrWriteError(w, r)
		if !ok {
			return
		}

		var req updateIncidentRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		update := &statuspkg.IncidentUpdate{Title: req.Title}
		if req.Status != nil {
			parsed, ok := parseIncidentStatusOrWriteError(w, *req.Status)
			if !ok {
				return
			}
			update.Status = &parsed
			if parsed == statuspkg.IncidentResolved {
				now := time.Now().UTC()
				update.ResolvedAt = &now
			}
		}

		if err := store.UpdateIncident(r.Context(), incidentID, update); err != nil {
			if errors.Is(err, statuspkg.ErrIncidentNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update incident")
			return
		}

		incident, err := store.GetIncident(r.Context(), incidentID)
		if err != nil {
			if errors.Is(err, statuspkg.ErrIncidentNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to load incident")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, incident)
	}
}

// handleAddIncidentUpdate returns an HTTP handler that adds a timestamped update entry with a message and status to an existing incident.
func handleAddIncidentUpdate(store statuspkg.IncidentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireIncidentStore(w, store) {
			return
		}

		incidentID, ok := parseIncidentIDOrWriteError(w, r)
		if !ok {
			return
		}

		var req addIncidentUpdateRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.Message = strings.TrimSpace(req.Message)
		if req.Message == "" {
			httputil.WriteError(w, http.StatusBadRequest, "message is required")
			return
		}
		incidentStatus, ok := parseIncidentStatusOrWriteError(w, req.Status)
		if !ok {
			return
		}

		entry := &statuspkg.IncidentUpdateEntry{
			Message: req.Message,
			Status:  incidentStatus,
		}
		if err := store.AddIncidentUpdate(r.Context(), incidentID, entry); err != nil {
			if errors.Is(err, statuspkg.ErrIncidentNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "incident not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to add incident update")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, entry)
	}
}

// normalizeAffectedServices deduplicates and normalizes service names to lowercase, trimmed strings, skipping empty entries and returning an empty slice if input is empty.
func normalizeAffectedServices(services []string) []string {
	if len(services) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(services))
	out := make([]string, 0, len(services))
	for _, service := range services {
		normalized := strings.ToLower(strings.TrimSpace(service))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func requireIncidentStore(w http.ResponseWriter, store statuspkg.IncidentStore) bool {
	if store == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "status incident store not configured")
		return false
	}
	return true
}

func parseIncidentIDOrWriteError(w http.ResponseWriter, r *http.Request) (string, bool) {
	incidentID := strings.TrimSpace(chi.URLParam(r, "id"))
	if incidentID == "" || !httputil.IsValidUUID(incidentID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid incident id")
		return "", false
	}
	return incidentID, true
}

func parseIncidentStatusOrWriteError(w http.ResponseWriter, raw string) (statuspkg.IncidentStatus, bool) {
	incidentStatus, ok := statuspkg.ParseIncidentStatus(raw)
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "invalid status")
		return "", false
	}
	return incidentStatus, true
}

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	statuspkg "github.com/allyourbase/ayb/internal/status"
	"github.com/go-chi/chi/v5"
)

type fakeIncidentStore struct {
	incidents   map[string]*statuspkg.Incident
	updatesByID map[string][]statuspkg.IncidentUpdateEntry

	lastListActiveOnly bool
	lastIncidentUpdate *statuspkg.IncidentUpdate
}

func newFakeIncidentStore() *fakeIncidentStore {
	return &fakeIncidentStore{
		incidents:   map[string]*statuspkg.Incident{},
		updatesByID: map[string][]statuspkg.IncidentUpdateEntry{},
	}
}

func (f *fakeIncidentStore) CreateIncident(ctx context.Context, incident *statuspkg.Incident) error {
	_ = ctx
	id := incident.ID
	if id == "" {
		id = "00000000-0000-0000-0000-000000000101"
	}
	now := time.Now().UTC()
	incident.ID = id
	incident.CreatedAt = now
	incident.UpdatedAt = now
	if incident.Updates == nil {
		incident.Updates = []statuspkg.IncidentUpdateEntry{}
	}
	copied := *incident
	f.incidents[id] = &copied
	return nil
}

func (f *fakeIncidentStore) GetIncident(ctx context.Context, id string) (*statuspkg.Incident, error) {
	_ = ctx
	incident, ok := f.incidents[id]
	if !ok {
		return nil, statuspkg.ErrIncidentNotFound
	}
	out := *incident
	out.Updates = append([]statuspkg.IncidentUpdateEntry{}, f.updatesByID[id]...)
	return &out, nil
}

func (f *fakeIncidentStore) ListIncidents(ctx context.Context, activeOnly bool) ([]statuspkg.Incident, error) {
	_ = ctx
	f.lastListActiveOnly = activeOnly
	out := make([]statuspkg.Incident, 0)
	for _, incident := range f.incidents {
		if activeOnly && incident.Status == statuspkg.IncidentResolved {
			continue
		}
		copied := *incident
		copied.Updates = append([]statuspkg.IncidentUpdateEntry{}, f.updatesByID[incident.ID]...)
		out = append(out, copied)
	}
	return out, nil
}

func (f *fakeIncidentStore) UpdateIncident(ctx context.Context, id string, update *statuspkg.IncidentUpdate) error {
	_ = ctx
	incident, ok := f.incidents[id]
	if !ok {
		return statuspkg.ErrIncidentNotFound
	}
	f.lastIncidentUpdate = update
	if update.Title != nil {
		incident.Title = *update.Title
	}
	if update.Status != nil {
		incident.Status = *update.Status
		if *update.Status == statuspkg.IncidentResolved {
			if incident.ResolvedAt == nil {
				if update.ResolvedAt != nil {
					resolvedAt := *update.ResolvedAt
					incident.ResolvedAt = &resolvedAt
				} else {
					now := time.Now().UTC()
					incident.ResolvedAt = &now
				}
			}
		} else {
			incident.ResolvedAt = nil
		}
	}
	incident.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeIncidentStore) AddIncidentUpdate(ctx context.Context, incidentID string, update *statuspkg.IncidentUpdateEntry) error {
	_ = ctx
	incident, ok := f.incidents[incidentID]
	if !ok {
		return statuspkg.ErrIncidentNotFound
	}
	update.ID = "00000000-0000-0000-0000-000000000301"
	update.IncidentID = incidentID
	update.CreatedAt = time.Now().UTC()
	f.updatesByID[incidentID] = append(f.updatesByID[incidentID], *update)
	incident.Status = update.Status
	if update.Status == statuspkg.IncidentResolved {
		now := time.Now().UTC()
		incident.ResolvedAt = &now
	} else {
		incident.ResolvedAt = nil
	}
	incident.UpdatedAt = time.Now().UTC()
	return nil
}

func TestHandlePublicStatus(t *testing.T) {
	t.Run("no snapshots returns operational", func(t *testing.T) {
		store := newFakeIncidentStore()
		h := handlePublicStatus(statuspkg.NewStatusHistory(10), store)

		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var got statusResponse
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Status != statuspkg.Operational {
			t.Fatalf("status = %q, want %q", got.Status, statuspkg.Operational)
		}
		if len(got.Services) != 0 {
			t.Fatalf("services len = %d, want 0", len(got.Services))
		}
	})

	t.Run("snapshot status reflected", func(t *testing.T) {
		history := statuspkg.NewStatusHistory(10)
		history.Push(statuspkg.StatusSnapshot{
			Status: statuspkg.PartialOutage,
			Services: []statuspkg.ProbeResult{
				{Service: statuspkg.Database, Healthy: false, Error: "down", CheckedAt: time.Now().UTC()},
			},
			CheckedAt: time.Now().UTC(),
		})

		h := handlePublicStatus(history, newFakeIncidentStore())
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		var got statusResponse
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Status != statuspkg.PartialOutage {
			t.Fatalf("status = %q, want %q", got.Status, statuspkg.PartialOutage)
		}
	})

	t.Run("active incidents only", func(t *testing.T) {
		store := newFakeIncidentStore()
		store.incidents["00000000-0000-0000-0000-000000000401"] = &statuspkg.Incident{
			ID:     "00000000-0000-0000-0000-000000000401",
			Title:  "Active",
			Status: statuspkg.IncidentInvestigating,
		}
		store.incidents["00000000-0000-0000-0000-000000000402"] = &statuspkg.Incident{
			ID:     "00000000-0000-0000-0000-000000000402",
			Title:  "Resolved",
			Status: statuspkg.IncidentResolved,
		}

		h := handlePublicStatus(statuspkg.NewStatusHistory(10), store)
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		var got statusResponse
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !store.lastListActiveOnly {
			t.Fatal("expected activeOnly query for public status incidents")
		}
		if len(got.Incidents) != 1 {
			t.Fatalf("incidents len = %d, want 1", len(got.Incidents))
		}
		if got.Incidents[0].Status != statuspkg.IncidentInvestigating {
			t.Fatalf("incident status = %q, want investigating", got.Incidents[0].Status)
		}
	})
}

func TestAdminIncidentHandlers(t *testing.T) {
	store := newFakeIncidentStore()

	t.Run("create incident returns 201", func(t *testing.T) {
		h := handleCreateIncident(store)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/incidents", strings.NewReader(`{"title":"DB issue","status":"investigating","affected_services":["database"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("create invalid status returns 400", func(t *testing.T) {
		h := handleCreateIncident(store)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/incidents", strings.NewReader(`{"title":"Bad","status":"unknown"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("list incidents returns 200 array", func(t *testing.T) {
		h := handleListIncidents(store)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/incidents", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var got []statuspkg.Incident
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	})

	t.Run("update resolved sets resolved_at", func(t *testing.T) {
		incidentID := "00000000-0000-0000-0000-000000000501"
		store.incidents[incidentID] = &statuspkg.Incident{
			ID:     incidentID,
			Title:  "Incident",
			Status: statuspkg.IncidentInvestigating,
		}

		r := chi.NewRouter()
		r.Put("/api/admin/incidents/{id}", handleUpdateIncident(store))

		req := httptest.NewRequest(http.MethodPut, "/api/admin/incidents/"+incidentID, strings.NewReader(`{"status":"resolved"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
		}
		if store.lastIncidentUpdate == nil || store.lastIncidentUpdate.ResolvedAt == nil {
			t.Fatal("expected resolved_at to be set on update")
		}
	})

	t.Run("reopen clears resolved_at", func(t *testing.T) {
		incidentID := "00000000-0000-0000-0000-000000000502"
		resolvedAt := time.Now().UTC().Add(-time.Minute)
		store.incidents[incidentID] = &statuspkg.Incident{
			ID:         incidentID,
			Title:      "Resolved incident",
			Status:     statuspkg.IncidentResolved,
			ResolvedAt: &resolvedAt,
		}

		r := chi.NewRouter()
		r.Put("/api/admin/incidents/{id}", handleUpdateIncident(store))

		req := httptest.NewRequest(http.MethodPut, "/api/admin/incidents/"+incidentID, strings.NewReader(`{"status":"monitoring"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
		}
		if store.incidents[incidentID].Status != statuspkg.IncidentMonitoring {
			t.Fatalf("status = %q, want monitoring", store.incidents[incidentID].Status)
		}
		if store.incidents[incidentID].ResolvedAt != nil {
			t.Fatal("expected resolved_at to be cleared when incident is reopened")
		}
	})

	t.Run("update nonexistent returns 404", func(t *testing.T) {
		r := chi.NewRouter()
		r.Put("/api/admin/incidents/{id}", handleUpdateIncident(store))

		req := httptest.NewRequest(http.MethodPut, "/api/admin/incidents/00000000-0000-0000-0000-000000000999", strings.NewReader(`{"status":"resolved"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("add incident update returns 201", func(t *testing.T) {
		incidentID := "00000000-0000-0000-0000-000000000601"
		store.incidents[incidentID] = &statuspkg.Incident{
			ID:     incidentID,
			Title:  "Incident",
			Status: statuspkg.IncidentInvestigating,
		}

		r := chi.NewRouter()
		r.Post("/api/admin/incidents/{id}/updates", handleAddIncidentUpdate(store))

		req := httptest.NewRequest(http.MethodPost, "/api/admin/incidents/"+incidentID+"/updates", strings.NewReader(`{"message":"Mitigation applied","status":"monitoring"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201, body=%s", w.Code, w.Body.String())
		}
		if store.incidents[incidentID].Status != statuspkg.IncidentMonitoring {
			t.Fatalf("incident status = %q, want monitoring", store.incidents[incidentID].Status)
		}
	})
}

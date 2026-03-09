package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/matview"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeMatviewAdmin is an in-memory fake for testing matview admin handlers.
type fakeMatviewAdmin struct {
	registrations []matview.Registration
	registerErr   error
	updateErr     error
	deleteErr     error
	refreshErr    error
	lastRefreshID string
}

func (f *fakeMatviewAdmin) List(ctx context.Context) ([]matview.Registration, error) {
	if f.registrations == nil {
		return []matview.Registration{}, nil
	}
	return f.registrations, nil
}

func (f *fakeMatviewAdmin) Get(ctx context.Context, id string) (*matview.Registration, error) {
	for _, r := range f.registrations {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", matview.ErrRegistrationNotFound, id)
}

func (f *fakeMatviewAdmin) Register(ctx context.Context, schemaName, viewName string, mode matview.RefreshMode) (*matview.Registration, error) {
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	now := time.Now()
	reg := matview.Registration{
		ID:          "00000000-0000-0000-0000-000000000099",
		SchemaName:  schemaName,
		ViewName:    viewName,
		RefreshMode: mode,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	f.registrations = append(f.registrations, reg)
	return &reg, nil
}

func (f *fakeMatviewAdmin) Update(ctx context.Context, id string, mode matview.RefreshMode) (*matview.Registration, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	for i := range f.registrations {
		if f.registrations[i].ID == id {
			f.registrations[i].RefreshMode = mode
			return &f.registrations[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", matview.ErrRegistrationNotFound, id)
}

func (f *fakeMatviewAdmin) Delete(ctx context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for i := range f.registrations {
		if f.registrations[i].ID == id {
			f.registrations = append(f.registrations[:i], f.registrations[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", matview.ErrRegistrationNotFound, id)
}

func (f *fakeMatviewAdmin) RefreshNow(ctx context.Context, id string) (*matview.RefreshResult, error) {
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	f.lastRefreshID = id
	for _, r := range f.registrations {
		if r.ID == id {
			return &matview.RefreshResult{Registration: r, DurationMs: 42}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", matview.ErrRegistrationNotFound, id)
}

func newFakeMatviewAdmin() *fakeMatviewAdmin {
	now := time.Now()
	return &fakeMatviewAdmin{
		registrations: []matview.Registration{
			{
				ID:          "aaaa0000-0000-0000-0000-000000000001",
				SchemaName:  "public",
				ViewName:    "leaderboard",
				RefreshMode: matview.RefreshModeStandard,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				ID:          "aaaa0000-0000-0000-0000-000000000002",
				SchemaName:  "public",
				ViewName:    "stats",
				RefreshMode: matview.RefreshModeConcurrent,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}
}

// --- List ---

func TestHandleAdminListMatviews(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminListMatviews(svc)

	req := httptest.NewRequest("GET", "/api/admin/matviews", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []matview.Registration `json:"items"`
		Count int                    `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 2, resp.Count)
	testutil.Equal(t, 2, len(resp.Items))
}

func TestHandleAdminListMatviewsEmpty(t *testing.T) {
	svc := &fakeMatviewAdmin{}
	handler := handleAdminListMatviews(svc)

	req := httptest.NewRequest("GET", "/api/admin/matviews", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []matview.Registration `json:"items"`
		Count int                    `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 0, resp.Count)
}

// --- Get ---

func TestHandleAdminGetMatview(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminGetMatview(svc)

	r := chi.NewRouter()
	r.Get("/api/admin/matviews/{id}", handler)

	req := httptest.NewRequest("GET", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var reg matview.Registration
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &reg))
	testutil.Equal(t, "leaderboard", reg.ViewName)
}

func TestHandleAdminGetMatviewNotFound(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminGetMatview(svc)

	r := chi.NewRouter()
	r.Get("/api/admin/matviews/{id}", handler)

	req := httptest.NewRequest("GET", "/api/admin/matviews/99999999-9999-9999-9999-999999999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminGetMatviewInvalidUUID(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminGetMatview(svc)

	r := chi.NewRouter()
	r.Get("/api/admin/matviews/{id}", handler)

	req := httptest.NewRequest("GET", "/api/admin/matviews/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Register ---

func TestHandleAdminRegisterMatview(t *testing.T) {
	svc := &fakeMatviewAdmin{}
	handler := handleAdminRegisterMatview(svc)

	body := `{"schema":"public","viewName":"leaderboard","refreshMode":"standard"}`
	req := httptest.NewRequest("POST", "/api/admin/matviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var reg matview.Registration
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &reg))
	testutil.Equal(t, "public", reg.SchemaName)
	testutil.Equal(t, "leaderboard", reg.ViewName)
	testutil.Equal(t, matview.RefreshModeStandard, reg.RefreshMode)
}

func TestHandleAdminRegisterMatviewMissingViewName(t *testing.T) {
	svc := &fakeMatviewAdmin{}
	handler := handleAdminRegisterMatview(svc)

	body := `{"schema":"public"}`
	req := httptest.NewRequest("POST", "/api/admin/matviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminRegisterMatviewDefaultSchema(t *testing.T) {
	svc := &fakeMatviewAdmin{}
	handler := handleAdminRegisterMatview(svc)

	body := `{"viewName":"stats","refreshMode":"concurrent"}`
	req := httptest.NewRequest("POST", "/api/admin/matviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var reg matview.Registration
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &reg))
	testutil.Equal(t, "public", reg.SchemaName)
}

func TestHandleAdminRegisterMatviewInvalidMode(t *testing.T) {
	svc := &fakeMatviewAdmin{}
	handler := handleAdminRegisterMatview(svc)

	body := `{"viewName":"stats","refreshMode":"invalid"}`
	req := httptest.NewRequest("POST", "/api/admin/matviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminRegisterMatviewDuplicate(t *testing.T) {
	svc := &fakeMatviewAdmin{registerErr: fmt.Errorf("%w: public.leaderboard", matview.ErrDuplicateRegistration)}
	handler := handleAdminRegisterMatview(svc)

	body := `{"viewName":"leaderboard","refreshMode":"standard"}`
	req := httptest.NewRequest("POST", "/api/admin/matviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminRegisterMatviewNotExists(t *testing.T) {
	svc := &fakeMatviewAdmin{registerErr: matview.ErrNotMaterializedView}
	handler := handleAdminRegisterMatview(svc)

	body := `{"viewName":"nonexistent","refreshMode":"standard"}`
	req := httptest.NewRequest("POST", "/api/admin/matviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// --- Update ---

func TestHandleAdminUpdateMatview(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminUpdateMatview(svc)

	r := chi.NewRouter()
	r.Put("/api/admin/matviews/{id}", handler)

	body := `{"refreshMode":"concurrent"}`
	req := httptest.NewRequest("PUT", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var reg matview.Registration
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &reg))
	testutil.Equal(t, matview.RefreshModeConcurrent, reg.RefreshMode)
}

func TestHandleAdminUpdateMatviewNotFound(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminUpdateMatview(svc)

	r := chi.NewRouter()
	r.Put("/api/admin/matviews/{id}", handler)

	body := `{"refreshMode":"concurrent"}`
	req := httptest.NewRequest("PUT", "/api/admin/matviews/99999999-9999-9999-9999-999999999999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminUpdateMatviewInvalidMode(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminUpdateMatview(svc)

	r := chi.NewRouter()
	r.Put("/api/admin/matviews/{id}", handler)

	body := `{"refreshMode":"bogus"}`
	req := httptest.NewRequest("PUT", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Delete ---

func TestHandleAdminDeleteMatview(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminDeleteMatview(svc)

	r := chi.NewRouter()
	r.Delete("/api/admin/matviews/{id}", handler)

	req := httptest.NewRequest("DELETE", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandleAdminDeleteMatviewNotFound(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminDeleteMatview(svc)

	r := chi.NewRouter()
	r.Delete("/api/admin/matviews/{id}", handler)

	req := httptest.NewRequest("DELETE", "/api/admin/matviews/99999999-9999-9999-9999-999999999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// --- Refresh ---

func TestHandleAdminRefreshMatview(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminRefreshMatview(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/matviews/{id}/refresh", handler)

	req := httptest.NewRequest("POST", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "aaaa0000-0000-0000-0000-000000000001", svc.lastRefreshID)

	var result matview.RefreshResult
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	testutil.Equal(t, 42, result.DurationMs)
}

func TestHandleAdminRefreshMatviewInProgress(t *testing.T) {
	svc := newFakeMatviewAdmin()
	svc.refreshErr = matview.ErrRefreshInProgress
	handler := handleAdminRefreshMatview(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/matviews/{id}/refresh", handler)

	req := httptest.NewRequest("POST", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminRefreshMatviewMissingIndex(t *testing.T) {
	svc := newFakeMatviewAdmin()
	svc.refreshErr = matview.ErrConcurrentRefreshRequiresIndex
	handler := handleAdminRefreshMatview(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/matviews/{id}/refresh", handler)

	req := httptest.NewRequest("POST", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminRefreshMatviewRequiresPopulated(t *testing.T) {
	svc := newFakeMatviewAdmin()
	svc.refreshErr = matview.ErrConcurrentRefreshRequiresPopulated
	handler := handleAdminRefreshMatview(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/matviews/{id}/refresh", handler)

	req := httptest.NewRequest("POST", "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminRefreshMatviewNotFound(t *testing.T) {
	svc := newFakeMatviewAdmin()
	handler := handleAdminRefreshMatview(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/matviews/{id}/refresh", handler)

	req := httptest.NewRequest("POST", "/api/admin/matviews/99999999-9999-9999-9999-999999999999/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

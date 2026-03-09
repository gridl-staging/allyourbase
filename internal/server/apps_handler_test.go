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

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeAppManager is an in-memory fake for testing app admin handlers.
type fakeAppManager struct {
	apps      []auth.App
	listErr   error
	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

func (f *fakeAppManager) CreateApp(_ context.Context, name, description, ownerUserID string) (*auth.App, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	app := auth.App{
		ID:                     "00000000-0000-0000-0000-000000000099",
		Name:                   name,
		Description:            description,
		OwnerUserID:            ownerUserID,
		RateLimitRPS:           0,
		RateLimitWindowSeconds: 60,
		CreatedAt:              time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC),
		UpdatedAt:              time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC),
	}
	f.apps = append(f.apps, app)
	return &app, nil
}

func (f *fakeAppManager) GetApp(_ context.Context, id string) (*auth.App, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, a := range f.apps {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, auth.ErrAppNotFound
}

func (f *fakeAppManager) ListApps(_ context.Context, page, perPage int) (*auth.AppListResult, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	total := len(f.apps)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	items := f.apps[start:end]
	if items == nil {
		items = []auth.App{}
	}

	totalPages := total / perPage
	if total%perPage != 0 {
		totalPages++
	}

	return &auth.AppListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}, nil
}

func (f *fakeAppManager) UpdateApp(_ context.Context, id, name, description string, rateLimitRPS, rateLimitWindowSeconds int) (*auth.App, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	for i, a := range f.apps {
		if a.ID == id {
			f.apps[i].Name = name
			f.apps[i].Description = description
			f.apps[i].RateLimitRPS = rateLimitRPS
			f.apps[i].RateLimitWindowSeconds = rateLimitWindowSeconds
			f.apps[i].UpdatedAt = time.Now()
			return &f.apps[i], nil
		}
	}
	return nil, auth.ErrAppNotFound
}

func (f *fakeAppManager) DeleteApp(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for i, a := range f.apps {
		if a.ID == id {
			f.apps = append(f.apps[:i], f.apps[i+1:]...)
			return nil
		}
	}
	return auth.ErrAppNotFound
}

func sampleApps() []auth.App {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	return []auth.App{
		{ID: "00000000-0000-0000-0000-000000000001", Name: "Sigil Mobile", Description: "Flutter app", OwnerUserID: "00000000-0000-0000-0000-000000000011", RateLimitRPS: 100, RateLimitWindowSeconds: 60, CreatedAt: now, UpdatedAt: now},
		{ID: "00000000-0000-0000-0000-000000000002", Name: "Sigil Web", Description: "Web dashboard", OwnerUserID: "00000000-0000-0000-0000-000000000011", RateLimitRPS: 200, RateLimitWindowSeconds: 60, CreatedAt: now, UpdatedAt: now},
		{ID: "00000000-0000-0000-0000-000000000003", Name: "Third Party", Description: "External integration", OwnerUserID: "00000000-0000-0000-0000-000000000012", RateLimitRPS: 50, RateLimitWindowSeconds: 60, CreatedAt: now, UpdatedAt: now},
	}
}

// --- List apps ---

func TestAdminListAppsSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.AppListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 3, len(result.Items))
	testutil.Equal(t, "Sigil Mobile", result.Items[0].Name)
}

func TestAdminListAppsEmpty(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: nil}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.AppListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, result.TotalItems)
}

func TestAdminListAppsServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{listErr: fmt.Errorf("db down")}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to list apps")
}

func TestAdminListAppsWithPagination(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps?page=1&perPage=2", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.AppListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 2, len(result.Items))
	testutil.Equal(t, 2, result.TotalPages)

	// Page 2
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/apps?page=2&perPage=2", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	var result2 auth.AppListResult
	testutil.NoError(t, json.NewDecoder(w2.Body).Decode(&result2))
	testutil.Equal(t, 1, len(result2.Items))
	testutil.Equal(t, 2, result2.Page)
}

func TestAdminListAppsPaginationDefaults(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps?page=0&perPage=0", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.AppListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 1, result.Page)
	testutil.Equal(t, 20, result.PerPage)
}

func TestAdminListAppsPaginationClamp(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps?page=1&perPage=500", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.AppListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 100, result.PerPage)
}

func TestAdminListAppsBeyondLastPage(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps?page=999&perPage=20", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.AppListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 0, len(result.Items))
	testutil.Equal(t, 3, result.TotalItems)
}

func TestAdminListAppsNonNumericParams(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminListApps(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps?page=abc&perPage=xyz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.AppListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 1, result.Page)
	testutil.Equal(t, 20, result.PerPage)
	testutil.Equal(t, 3, len(result.Items))
}

// --- Get app ---

func TestAdminGetAppSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminGetApp(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var app auth.App
	err := json.NewDecoder(w.Body).Decode(&app)
	testutil.NoError(t, err)
	testutil.Equal(t, "Sigil Mobile", app.Name)
	testutil.Equal(t, 100, app.RateLimitRPS)
}

func TestAdminGetAppNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminGetApp(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps/00000000-0000-0000-0000-000000000099", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "app not found")
}

func TestAdminGetAppInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminGetApp(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid app id format")
}

// --- Create app ---

func TestAdminCreateAppSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: []auth.App{}}
	handler := handleAdminCreateApp(mgr)

	body := `{"name":"My App","description":"test app","ownerUserId":"00000000-0000-0000-0000-000000000011"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/apps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var app auth.App
	err := json.NewDecoder(w.Body).Decode(&app)
	testutil.NoError(t, err)
	testutil.Equal(t, "My App", app.Name)
	testutil.Equal(t, "test app", app.Description)
	testutil.Equal(t, "00000000-0000-0000-0000-000000000011", app.OwnerUserID)
}

func TestAdminCreateAppMissingName(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{}
	handler := handleAdminCreateApp(mgr)

	body := `{"ownerUserId":"00000000-0000-0000-0000-000000000011"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/apps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestAdminCreateAppMissingOwner(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{}
	handler := handleAdminCreateApp(mgr)

	body := `{"name":"My App"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/apps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "ownerUserId is required")
}

func TestAdminCreateAppInvalidOwnerIDFormat(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{}
	handler := handleAdminCreateApp(mgr)

	body := `{"name":"My App","ownerUserId":"not-a-uuid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/apps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid ownerUserId format")
}

func TestAdminCreateAppServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{createErr: fmt.Errorf("db error")}
	handler := handleAdminCreateApp(mgr)

	body := `{"name":"My App","ownerUserId":"00000000-0000-0000-0000-000000000011"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/apps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to create app")
}

func TestAdminCreateAppInvalidJSON(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{}
	handler := handleAdminCreateApp(mgr)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/apps", strings.NewReader(`{broken`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Update app ---

func TestAdminUpdateAppSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	body := `{"name":"Updated Name","description":"new desc","rateLimitRps":500,"rateLimitWindowSeconds":120}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var app auth.App
	err := json.NewDecoder(w.Body).Decode(&app)
	testutil.NoError(t, err)
	testutil.Equal(t, "Updated Name", app.Name)
	testutil.Equal(t, "new desc", app.Description)
	testutil.Equal(t, 500, app.RateLimitRPS)
	testutil.Equal(t, 120, app.RateLimitWindowSeconds)
}

func TestAdminUpdateAppNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	body := `{"name":"Updated","description":"new"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/00000000-0000-0000-0000-000000000099", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "app not found")
}

func TestAdminUpdateAppMissingName(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	body := `{"description":"no name"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestAdminUpdateAppInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/bad-id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid app id format")
}

func TestAdminUpdateAppInvalidJSON(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/00000000-0000-0000-0000-000000000001", strings.NewReader(`{broken`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Delete app ---

func TestAdminDeleteAppSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminDeleteApp(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/apps/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 2, len(mgr.apps))
}

func TestAdminDeleteAppNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminDeleteApp(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/apps/00000000-0000-0000-0000-000000000099", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "app not found")
}

func TestAdminDeleteAppInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminDeleteApp(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/apps/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid app id format")
}

func TestAdminDeleteAppServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps(), deleteErr: fmt.Errorf("db constraint")}
	handler := handleAdminDeleteApp(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/apps/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to delete app")
}

// --- GetApp service error (non-404) ---

func TestAdminGetAppServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps(), getErr: fmt.Errorf("db connection lost")}
	handler := handleAdminGetApp(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/apps/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/apps/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to get app")
}

// --- UpdateApp service error (non-404) ---

func TestAdminUpdateAppServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps(), updateErr: fmt.Errorf("db timeout")}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	body := `{"name":"Test","description":"test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to update app")
}

// --- CreateApp owner not found ---

func TestAdminCreateAppOwnerNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{createErr: auth.ErrAppOwnerNotFound}
	handler := handleAdminCreateApp(mgr)

	body := `{"name":"My App","ownerUserId":"00000000-0000-0000-0000-000000000099"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/apps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "owner user not found")
}

// --- UpdateApp negative rate limits ---

func TestAdminUpdateAppNegativeRateLimit(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	body := `{"name":"Test","description":"test","rateLimitRps":-1,"rateLimitWindowSeconds":60}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "rate limit values must be non-negative")
}

func TestAdminUpdateAppNegativeWindow(t *testing.T) {
	t.Parallel()
	mgr := &fakeAppManager{apps: sampleApps()}
	handler := handleAdminUpdateApp(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/apps/{id}", handler)

	body := `{"name":"Test","description":"test","rateLimitRps":100,"rateLimitWindowSeconds":-1}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/apps/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "rate limit values must be non-negative")
}

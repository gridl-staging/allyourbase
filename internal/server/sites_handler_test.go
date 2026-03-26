package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeSiteManager is an in-memory fake for testing site admin handlers.
type fakeSiteManager struct {
	sites   []sites.Site
	deploys []sites.Deploy

	createSiteErr   error
	getSiteErr      error
	listSitesErr    error
	updateSiteErr   error
	deleteSiteErr   error
	createDeployErr error
	getDeployErr    error
	ensureUploadErr error
	recordUploadErr error
	listDeploysErr  error
	promoteErr      error
	failErr         error
	rollbackErr     error
}

func (f *fakeSiteManager) CreateSite(_ context.Context, name, slug string, spaMode bool, customDomainID *string) (*sites.Site, error) {
	if f.createSiteErr != nil {
		return nil, f.createSiteErr
	}
	s := sites.Site{
		ID:             "site-001",
		Name:           name,
		Slug:           slug,
		SPAMode:        spaMode,
		CustomDomainID: customDomainID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	f.sites = append(f.sites, s)
	return &s, nil
}

func (f *fakeSiteManager) GetSite(_ context.Context, id string) (*sites.Site, error) {
	if f.getSiteErr != nil {
		return nil, f.getSiteErr
	}
	for _, s := range f.sites {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, sites.ErrSiteNotFound
}

func (f *fakeSiteManager) ListSites(_ context.Context, page, perPage int) (*sites.SiteListResult, error) {
	if f.listSitesErr != nil {
		return nil, f.listSitesErr
	}
	return &sites.SiteListResult{
		Sites:      f.sites,
		TotalCount: len(f.sites),
		Page:       page,
		PerPage:    perPage,
	}, nil
}

func (f *fakeSiteManager) UpdateSite(_ context.Context, id string, name *string, spaMode *bool, _ *string, _ bool) (*sites.Site, error) {
	if f.updateSiteErr != nil {
		return nil, f.updateSiteErr
	}
	for i, s := range f.sites {
		if s.ID == id {
			if name != nil {
				f.sites[i].Name = *name
			}
			if spaMode != nil {
				f.sites[i].SPAMode = *spaMode
			}
			return &f.sites[i], nil
		}
	}
	return nil, sites.ErrSiteNotFound
}

func (f *fakeSiteManager) DeleteSite(_ context.Context, id string) error {
	if f.deleteSiteErr != nil {
		return f.deleteSiteErr
	}
	for i, s := range f.sites {
		if s.ID == id {
			f.sites = append(f.sites[:i], f.sites[i+1:]...)
			return nil
		}
	}
	return sites.ErrSiteNotFound
}

func (f *fakeSiteManager) CreateDeploy(_ context.Context, siteID string) (*sites.Deploy, error) {
	if f.createDeployErr != nil {
		return nil, f.createDeployErr
	}
	d := sites.Deploy{
		ID:        "deploy-001",
		SiteID:    siteID,
		Status:    sites.StatusUploading,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	f.deploys = append(f.deploys, d)
	return &d, nil
}

func (f *fakeSiteManager) GetDeploy(_ context.Context, siteID, deployID string) (*sites.Deploy, error) {
	if f.getDeployErr != nil {
		return nil, f.getDeployErr
	}
	for _, d := range f.deploys {
		if d.ID == deployID && d.SiteID == siteID {
			return &d, nil
		}
	}
	return nil, sites.ErrDeployNotFound
}

func (f *fakeSiteManager) EnsureDeployUploading(ctx context.Context, siteID, deployID string) error {
	if f.ensureUploadErr != nil {
		return f.ensureUploadErr
	}
	deploy, err := f.GetDeploy(ctx, siteID, deployID)
	if err != nil {
		return err
	}
	if deploy.Status != sites.StatusUploading {
		return sites.ErrInvalidTransition
	}
	return nil
}

func (f *fakeSiteManager) RecordDeployFileUpload(_ context.Context, siteID, deployID string, fileSize int64) (*sites.Deploy, error) {
	if f.recordUploadErr != nil {
		return nil, f.recordUploadErr
	}
	for i := range f.deploys {
		if f.deploys[i].ID != deployID || f.deploys[i].SiteID != siteID {
			continue
		}
		if f.deploys[i].Status != sites.StatusUploading {
			return nil, sites.ErrInvalidTransition
		}
		f.deploys[i].FileCount++
		f.deploys[i].TotalBytes += fileSize
		return &f.deploys[i], nil
	}
	return nil, sites.ErrDeployNotFound
}

func (f *fakeSiteManager) ListDeploys(_ context.Context, siteID string, page, perPage int) (*sites.DeployListResult, error) {
	if f.listDeploysErr != nil {
		return nil, f.listDeploysErr
	}
	var result []sites.Deploy
	for _, d := range f.deploys {
		if d.SiteID == siteID {
			result = append(result, d)
		}
	}
	if result == nil {
		result = []sites.Deploy{}
	}
	return &sites.DeployListResult{
		Deploys:    result,
		TotalCount: len(result),
		Page:       page,
		PerPage:    perPage,
	}, nil
}

func (f *fakeSiteManager) PromoteDeploy(_ context.Context, siteID, deployID string) (*sites.Deploy, error) {
	if f.promoteErr != nil {
		return nil, f.promoteErr
	}
	for i, d := range f.deploys {
		if d.ID == deployID && d.SiteID == siteID {
			f.deploys[i].Status = sites.StatusLive
			return &f.deploys[i], nil
		}
	}
	return nil, sites.ErrDeployNotFound
}

func (f *fakeSiteManager) FailDeploy(_ context.Context, siteID, deployID, errorMsg string) (*sites.Deploy, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	for i, d := range f.deploys {
		if d.ID == deployID && d.SiteID == siteID {
			f.deploys[i].Status = sites.StatusFailed
			f.deploys[i].ErrorMessage = &errorMsg
			return &f.deploys[i], nil
		}
	}
	return nil, sites.ErrDeployNotFound
}

func (f *fakeSiteManager) RollbackDeploy(_ context.Context, siteID string) (*sites.Deploy, error) {
	if f.rollbackErr != nil {
		return nil, f.rollbackErr
	}
	return &sites.Deploy{
		ID:     "deploy-prev",
		SiteID: siteID,
		Status: sites.StatusLive,
	}, nil
}

// --- helpers ---

func siteRouter(method, path string, handler http.HandlerFunc) *chi.Mux {
	r := chi.NewRouter()
	switch method {
	case http.MethodGet:
		r.Get(path, handler)
	case http.MethodPost:
		r.Post(path, handler)
	case http.MethodPut:
		r.Put(path, handler)
	case http.MethodDelete:
		r.Delete(path, handler)
	}
	return r
}

// --- site handler tests ---

func TestHandleAdminListSites(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{
		sites: []sites.Site{
			{ID: "s1", Name: "Alpha", Slug: "alpha"},
			{ID: "s2", Name: "Beta", Slug: "beta"},
		},
	}
	handler := handleAdminListSites(mgr)
	req := httptest.NewRequest(http.MethodGet, "/admin/sites?page=1&perPage=10", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var result sites.SiteListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 2, result.TotalCount)
}

func TestHandleAdminCreateSiteSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{}
	handler := handleAdminCreateSite(mgr)

	body := `{"name":"My Site","slug":"my-site","spaMode":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	var site sites.Site
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&site))
	testutil.Equal(t, "My Site", site.Name)
	testutil.Equal(t, "my-site", site.Slug)
}

func TestHandleAdminCreateSiteRejectsBlankRequiredFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{name: "empty name", body: `{"name":"","slug":"ok","spaMode":false}`},
		{name: "whitespace name", body: `{"name":"  ","slug":"ok","spaMode":false}`},
		{name: "whitespace slug", body: `{"name":"ok","slug":"  ","spaMode":false}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mgr := &fakeSiteManager{}
			handler := handleAdminCreateSite(mgr)

			req := httptest.NewRequest(http.MethodPost, "/admin/sites", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			testutil.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleAdminCreateSiteSlugTaken(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{createSiteErr: sites.ErrSiteSlugTaken}
	handler := handleAdminCreateSite(mgr)

	body := `{"name":"Dup","slug":"taken","spaMode":false}`
	req := httptest.NewRequest(http.MethodPost, "/admin/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminCreateSiteCustomDomainTaken(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{createSiteErr: sites.ErrSiteCustomDomainTaken}
	handler := handleAdminCreateSite(mgr)

	body := `{"name":"Dup","slug":"taken","spaMode":false,"customDomainId":"00000000-0000-0000-0000-000000000201"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminCreateSiteUnexpectedError(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{createSiteErr: errors.New("insert failed")}
	handler := handleAdminCreateSite(mgr)

	body := `{"name":"My Site","slug":"my-site","spaMode":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, false, strings.Contains(w.Body.String(), "insert failed"))
}

func TestHandleAdminGetSiteInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{}
	r := siteRouter(http.MethodGet, "/admin/sites/{siteId}", handleAdminGetSite(mgr))
	req := httptest.NewRequest(http.MethodGet, "/admin/sites/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminGetSiteSuccess(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	mgr := &fakeSiteManager{
		sites: []sites.Site{{ID: siteID, Name: "Test", Slug: "test"}},
	}
	r := siteRouter(http.MethodGet, "/admin/sites/{siteId}", handleAdminGetSite(mgr))
	req := httptest.NewRequest(http.MethodGet, "/admin/sites/"+siteID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var site sites.Site
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&site))
	testutil.Equal(t, "Test", site.Name)
}

func TestHandleAdminGetSiteNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{}
	r := siteRouter(http.MethodGet, "/admin/sites/{siteId}", handleAdminGetSite(mgr))
	req := httptest.NewRequest(http.MethodGet, "/admin/sites/00000000-0000-0000-0000-000000000000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminUpdateSiteEmptyName(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{
		sites: []sites.Site{{ID: "00000000-0000-0000-0000-000000000001", Name: "Old", Slug: "old"}},
	}
	r := siteRouter(http.MethodPut, "/admin/sites/{siteId}", handleAdminUpdateSite(mgr))

	body := `{"name":"  "}`
	req := httptest.NewRequest(http.MethodPut, "/admin/sites/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminUpdateSiteUnexpectedError(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{updateSiteErr: errors.New("update failed")}
	r := siteRouter(http.MethodPut, "/admin/sites/{siteId}", handleAdminUpdateSite(mgr))

	body := `{"name":"New"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/sites/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, false, strings.Contains(w.Body.String(), "update failed"))
}

func TestHandleAdminUpdateSiteCustomDomainTaken(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{updateSiteErr: sites.ErrSiteCustomDomainTaken}
	r := siteRouter(http.MethodPut, "/admin/sites/{siteId}", handleAdminUpdateSite(mgr))

	body := `{"customDomainId":"00000000-0000-0000-0000-000000000201"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/sites/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminUpdateSiteSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{
		sites: []sites.Site{{ID: "00000000-0000-0000-0000-000000000001", Name: "Old", Slug: "old", SPAMode: true}},
	}
	r := siteRouter(http.MethodPut, "/admin/sites/{siteId}", handleAdminUpdateSite(mgr))

	body := `{"name":"New"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/sites/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var site sites.Site
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&site))
	testutil.Equal(t, "New", site.Name)
}

func TestHandleAdminDeleteSiteSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{
		sites: []sites.Site{{ID: "00000000-0000-0000-0000-000000000001", Name: "Delete Me", Slug: "del"}},
	}
	r := siteRouter(http.MethodDelete, "/admin/sites/{siteId}", handleAdminDeleteSite(mgr))
	req := httptest.NewRequest(http.MethodDelete, "/admin/sites/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandleAdminDeleteSiteNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeSiteManager{}
	r := siteRouter(http.MethodDelete, "/admin/sites/{siteId}", handleAdminDeleteSite(mgr))
	req := httptest.NewRequest(http.MethodDelete, "/admin/sites/00000000-0000-0000-0000-000000000000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

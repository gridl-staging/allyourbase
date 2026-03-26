package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestHandleAdminCreateDeploySuccess(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	mgr := &fakeSiteManager{
		sites: []sites.Site{{ID: siteID, Name: "S", Slug: "s"}},
	}
	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys", handleAdminCreateDeploy(mgr))
	req := httptest.NewRequest(http.MethodPost, "/admin/sites/"+siteID+"/deploys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	var deploy sites.Deploy
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&deploy))
	testutil.Equal(t, sites.StatusUploading, deploy.Status)
}

func TestHandleAdminGetDeployNotFound(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	mgr := &fakeSiteManager{}
	r := siteRouter(http.MethodGet, "/admin/sites/{siteId}/deploys/{deployId}", handleAdminGetDeploy(mgr))
	req := httptest.NewRequest(http.MethodGet, "/admin/sites/"+siteID+"/deploys/00000000-0000-0000-0000-000000000099", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminPromoteDeploySuccess(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{{ID: deployID, SiteID: siteID, Status: sites.StatusUploading}},
	}
	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/promote", handleAdminPromoteDeploy(mgr))
	req := httptest.NewRequest(http.MethodPost, "/admin/sites/"+siteID+"/deploys/"+deployID+"/promote", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var deploy sites.Deploy
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&deploy))
	testutil.Equal(t, sites.StatusLive, deploy.Status)
}

func TestHandleAdminPromoteDeployInvalidTransition(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{promoteErr: sites.ErrInvalidTransition}
	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/promote", handleAdminPromoteDeploy(mgr))
	req := httptest.NewRequest(http.MethodPost, "/admin/sites/"+siteID+"/deploys/"+deployID+"/promote", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminFailDeploySuccess(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{{ID: deployID, SiteID: siteID, Status: sites.StatusUploading}},
	}
	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/fail", handleAdminFailDeploy(mgr))
	body := `{"errorMessage":"upload timed out"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/sites/"+siteID+"/deploys/"+deployID+"/fail", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var deploy sites.Deploy
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&deploy))
	testutil.Equal(t, sites.StatusFailed, deploy.Status)
}

func TestHandleAdminFailDeployInvalidTransition(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{failErr: sites.ErrInvalidTransition}
	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/fail", handleAdminFailDeploy(mgr))
	body := `{"errorMessage":"upload timed out"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/sites/"+siteID+"/deploys/"+deployID+"/fail", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminRollbackDeploySuccess(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	mgr := &fakeSiteManager{}
	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/rollback", handleAdminRollbackDeploy(mgr))
	req := httptest.NewRequest(http.MethodPost, "/admin/sites/"+siteID+"/deploys/rollback", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var deploy sites.Deploy
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&deploy))
	testutil.Equal(t, sites.StatusLive, deploy.Status)
}

func TestHandleAdminRollbackNoSuperseded(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	mgr := &fakeSiteManager{rollbackErr: sites.ErrNoLiveDeploy}
	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/rollback", handleAdminRollbackDeploy(mgr))
	req := httptest.NewRequest(http.MethodPost, "/admin/sites/"+siteID+"/deploys/rollback", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleAdminListDeploysSuccess(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{
			{ID: "d1", SiteID: siteID, Status: sites.StatusLive},
			{ID: "d2", SiteID: siteID, Status: sites.StatusSuperseded},
		},
	}
	r := siteRouter(http.MethodGet, "/admin/sites/{siteId}/deploys", handleAdminListDeploys(mgr))
	req := httptest.NewRequest(http.MethodGet, "/admin/sites/"+siteID+"/deploys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var result sites.DeployListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 2, result.TotalCount)
}

func TestHandleAdminListDeploysMissingSite(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	mgr := &fakeSiteManager{listDeploysErr: sites.ErrSiteNotFound}
	r := siteRouter(http.MethodGet, "/admin/sites/{siteId}/deploys", handleAdminListDeploys(mgr))
	req := httptest.NewRequest(http.MethodGet, "/admin/sites/"+siteID+"/deploys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

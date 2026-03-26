//go:build integration

package sites_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminSitesHTTPIntegrationLifecycle(t *testing.T) {
	ts, adminToken := newSitesIntegrationServer(t)
	adminClient := newAdminHTTPClient(ts, adminToken)

	var createdSite sites.Site
	status := adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites",
		map[string]any{
			"name":    "Integration Site",
			"slug":    "integration-site",
			"spaMode": true,
		},
		&createdSite,
	)
	testutil.StatusCode(t, http.StatusCreated, status)
	testutil.True(t, createdSite.ID != "", "expected created site id")
	testutil.Equal(t, "integration-site", createdSite.Slug)
	testutil.Nil(t, createdSite.LiveDeployID)

	var firstDeploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+createdSite.ID+"/deploys",
		nil,
		&firstDeploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)
	testutil.Equal(t, sites.StatusUploading, firstDeploy.Status)

	var promotedFirst sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+createdSite.ID+"/deploys/"+firstDeploy.ID+"/promote",
		nil,
		&promotedFirst,
	)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Equal(t, sites.StatusLive, promotedFirst.Status)

	assertSiteLiveDeployID(t, adminClient, createdSite.ID, firstDeploy.ID)

	var secondDeploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+createdSite.ID+"/deploys",
		nil,
		&secondDeploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)
	testutil.Equal(t, sites.StatusUploading, secondDeploy.Status)

	var promotedSecond sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+createdSite.ID+"/deploys/"+secondDeploy.ID+"/promote",
		nil,
		&promotedSecond,
	)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Equal(t, sites.StatusLive, promotedSecond.Status)

	assertSiteLiveDeployID(t, adminClient, createdSite.ID, secondDeploy.ID)

	var firstDeployAfterSecondPromote sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodGet,
		"/api/admin/sites/"+createdSite.ID+"/deploys/"+firstDeploy.ID,
		nil,
		&firstDeployAfterSecondPromote,
	)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Equal(t, sites.StatusSuperseded, firstDeployAfterSecondPromote.Status)

	var rollbackDeploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+createdSite.ID+"/deploys/rollback",
		nil,
		&rollbackDeploy,
	)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Equal(t, firstDeploy.ID, rollbackDeploy.ID)
	testutil.Equal(t, sites.StatusLive, rollbackDeploy.Status)

	assertSiteLiveDeployID(t, adminClient, createdSite.ID, firstDeploy.ID)

	var secondDeployAfterRollback sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodGet,
		"/api/admin/sites/"+createdSite.ID+"/deploys/"+secondDeploy.ID,
		nil,
		&secondDeployAfterRollback,
	)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Equal(t, sites.StatusSuperseded, secondDeployAfterRollback.Status)
}

func TestAdminSitesHTTPIntegrationDeployFileUpload(t *testing.T) {
	ts, adminToken, storageSvc := newSitesIntegrationServerWithStorage(t)
	adminClient := newAdminHTTPClient(ts, adminToken)

	var createdSite sites.Site
	status := adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites",
		map[string]any{
			"name":    "Upload Site",
			"slug":    "upload-site",
			"spaMode": true,
		},
		&createdSite,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	var deploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+createdSite.ID+"/deploys",
		nil,
		&deploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	firstContent := []byte("<html>Hello</html>")
	status = adminClient.uploadDeployFile(
		t,
		createdSite.ID,
		deploy.ID,
		"index.html",
		firstContent,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	secondContent := []byte("console.log('hello');")
	status = adminClient.uploadDeployFile(
		t,
		createdSite.ID,
		deploy.ID,
		"assets/app.js",
		secondContent,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	var deployAfterUpload sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodGet,
		"/api/admin/sites/"+createdSite.ID+"/deploys/"+deploy.ID,
		nil,
		&deployAfterUpload,
	)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Equal(t, 2, deployAfterUpload.FileCount)
	testutil.Equal(t, int64(len(firstContent)+len(secondContent)), deployAfterUpload.TotalBytes)

	firstObjectPath := "sites/" + createdSite.ID + "/" + deploy.ID + "/index.html"
	secondObjectPath := "sites/" + createdSite.ID + "/" + deploy.ID + "/assets/app.js"

	firstObj, err := storageSvc.GetObject(context.Background(), "_ayb_sites", firstObjectPath)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(firstContent)), firstObj.Size)

	secondObj, err := storageSvc.GetObject(context.Background(), "_ayb_sites", secondObjectPath)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(secondContent)), secondObj.Size)

	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+createdSite.ID+"/deploys/"+deploy.ID+"/promote",
		nil,
		nil,
	)
	testutil.StatusCode(t, http.StatusOK, status)

	status = adminClient.uploadDeployFile(
		t,
		createdSite.ID,
		deploy.ID,
		"assets/blocked.js",
		[]byte("console.log('blocked');"),
	)
	testutil.StatusCode(t, http.StatusConflict, status)
}

func TestSiteDeployServeRuntimeIntegrationDerivedHostAndRollback(t *testing.T) {
	ts, _, adminToken, _, _ := newSitesIntegrationServerWithRuntimeStorage(t)
	adminClient := newAdminHTTPClient(ts, adminToken)

	var site sites.Site
	status := adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites",
		map[string]any{
			"name":    "Runtime Site",
			"slug":    "runtime-site",
			"spaMode": true,
		},
		&site,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	var firstDeploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys",
		nil,
		&firstDeploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	status = adminClient.uploadDeployFile(t, site.ID, firstDeploy.ID, "index.html", []byte("<html>first</html>"))
	testutil.StatusCode(t, http.StatusCreated, status)
	status = adminClient.uploadDeployFile(t, site.ID, firstDeploy.ID, "assets/app.js", []byte("console.log('first');"))
	testutil.StatusCode(t, http.StatusCreated, status)

	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys/"+firstDeploy.ID+"/promote",
		nil,
		nil,
	)
	testutil.StatusCode(t, http.StatusOK, status)

	runtimeHost := "runtime-site.localhost"
	status, body := requestWithHost(t, http.MethodGet, ts.URL+"/", runtimeHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "first")

	status, body = requestWithHost(t, http.MethodGet, ts.URL+"/assets/app.js", runtimeHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "first")

	status, body = requestWithHost(t, http.MethodGet, ts.URL+"/missing/client/route", runtimeHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "first")

	status, body = requestWithHost(t, http.MethodGet, ts.URL+"/api/openapi.yaml", runtimeHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "openapi:")

	status, body = requestWithHost(t, http.MethodGet, ts.URL+"/health", runtimeHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "ok")

	var secondDeploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys",
		nil,
		&secondDeploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	status = adminClient.uploadDeployFile(t, site.ID, secondDeploy.ID, "index.html", []byte("<html>second</html>"))
	testutil.StatusCode(t, http.StatusCreated, status)
	status = adminClient.uploadDeployFile(t, site.ID, secondDeploy.ID, "assets/app.js", []byte("console.log('second');"))
	testutil.StatusCode(t, http.StatusCreated, status)

	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys/"+secondDeploy.ID+"/promote",
		nil,
		nil,
	)
	testutil.StatusCode(t, http.StatusOK, status)

	status, body = requestWithHost(t, http.MethodGet, ts.URL+"/", runtimeHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "second")

	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys/rollback",
		nil,
		nil,
	)
	testutil.StatusCode(t, http.StatusOK, status)

	status, body = requestWithHost(t, http.MethodGet, ts.URL+"/", runtimeHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "first")
}

func TestSiteDeployServeRuntimeIntegrationSPADisabledReturns404(t *testing.T) {
	ts, _, adminToken, _, _ := newSitesIntegrationServerWithRuntimeStorage(t)
	adminClient := newAdminHTTPClient(ts, adminToken)

	var site sites.Site
	status := adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites",
		map[string]any{
			"name":    "Static Site",
			"slug":    "static-site",
			"spaMode": false,
		},
		&site,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	var deploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys",
		nil,
		&deploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	status = adminClient.uploadDeployFile(t, site.ID, deploy.ID, "index.html", []byte("<html>static</html>"))
	testutil.StatusCode(t, http.StatusCreated, status)
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys/"+deploy.ID+"/promote",
		nil,
		nil,
	)
	testutil.StatusCode(t, http.StatusOK, status)

	status, _ = requestWithHost(t, http.MethodGet, ts.URL+"/missing-route", "static-site.localhost")
	testutil.StatusCode(t, http.StatusNotFound, status)
}

func TestSiteDeployServeRuntimeIntegrationSPAModeMissingIndexReturns404(t *testing.T) {
	ts, _, adminToken, _, _ := newSitesIntegrationServerWithRuntimeStorage(t)
	adminClient := newAdminHTTPClient(ts, adminToken)

	var site sites.Site
	status := adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites",
		map[string]any{
			"name":    "SPA Missing Index",
			"slug":    "spa-missing-index",
			"spaMode": true,
		},
		&site,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	var deploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys",
		nil,
		&deploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	status = adminClient.uploadDeployFile(t, site.ID, deploy.ID, "assets/app.js", []byte("console.log('spa');"))
	testutil.StatusCode(t, http.StatusCreated, status)
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys/"+deploy.ID+"/promote",
		nil,
		nil,
	)
	testutil.StatusCode(t, http.StatusOK, status)

	status, _ = requestWithHost(t, http.MethodGet, ts.URL+"/", "spa-missing-index.localhost")
	testutil.StatusCode(t, http.StatusNotFound, status)

	status, _ = requestWithHost(t, http.MethodGet, ts.URL+"/missing-route", "spa-missing-index.localhost")
	testutil.StatusCode(t, http.StatusNotFound, status)
}

func TestSiteDeployServeRuntimeIntegrationCustomDomainHostResolution(t *testing.T) {
	ts, srv, adminToken, _, pg := newSitesIntegrationServerWithRuntimeStorage(t)
	adminClient := newAdminHTTPClient(ts, adminToken)
	ctx := context.Background()

	const customDomainID = "00000000-0000-0000-0000-000000000150"
	const customHost = "custom.runtime.test"
	_, err := pg.Pool.Exec(
		ctx,
		`INSERT INTO _ayb_custom_domains (id, hostname, environment, status, verification_token)
		 VALUES ($1, $2, 'production', 'active', 'runtime-token')`,
		customDomainID,
		customHost,
	)
	testutil.NoError(t, err)
	testutil.NoError(t, srv.LoadRouteTable(ctx, nil))

	var site sites.Site
	status := adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites",
		map[string]any{
			"name":           "Custom Domain Site",
			"slug":           "custom-domain-site",
			"spaMode":        true,
			"customDomainId": customDomainID,
		},
		&site,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	var deploy sites.Deploy
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys",
		nil,
		&deploy,
	)
	testutil.StatusCode(t, http.StatusCreated, status)

	status = adminClient.uploadDeployFile(t, site.ID, deploy.ID, "index.html", []byte("<html>custom</html>"))
	testutil.StatusCode(t, http.StatusCreated, status)
	status = adminClient.doJSON(
		t,
		http.MethodPost,
		"/api/admin/sites/"+site.ID+"/deploys/"+deploy.ID+"/promote",
		nil,
		nil,
	)
	testutil.StatusCode(t, http.StatusOK, status)

	status, body := requestWithHost(t, http.MethodGet, ts.URL+"/", customHost)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.Contains(t, string(body), "custom")
}

type adminHTTPClient struct {
	baseURL string
	token   string
}

func newAdminHTTPClient(ts *httptest.Server, token string) adminHTTPClient {
	return adminHTTPClient{
		baseURL: ts.URL,
		token:   token,
	}
}

func assertSiteLiveDeployID(t *testing.T, client adminHTTPClient, siteID, expectedDeployID string) {
	t.Helper()

	var site sites.Site
	status := client.doJSON(t, http.MethodGet, "/api/admin/sites/"+siteID, nil, &site)
	testutil.StatusCode(t, http.StatusOK, status)
	testutil.NotNil(t, site.LiveDeployID)
	testutil.Equal(t, expectedDeployID, *site.LiveDeployID)
}

func newSitesIntegrationServer(t *testing.T) (*httptest.Server, string) {
	ts, token, _ := newSitesIntegrationServerWithStorage(t)
	return ts, token
}

func newSitesIntegrationServerWithStorage(t *testing.T) (*httptest.Server, string, *storage.Service) {
	ts, _, token, storageSvc, _ := newSitesIntegrationServerWithRuntimeStorage(t)
	return ts, token, storageSvc
}

func newSitesIntegrationServerWithRuntimeStorage(t *testing.T) (*httptest.Server, *server.Server, string, *storage.Service, *testutil.PGContainer) {
	t.Helper()
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	t.Cleanup(cleanup)

	runner := migrations.NewRunner(pg.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	cfg := config.Default()
	cfg.Admin.Password = "sites-admin-pass"

	backend, err := storage.NewLocalBackend(t.TempDir())
	if err != nil {
		t.Fatalf("create local storage backend: %v", err)
	}
	storageSvc := storage.NewService(pg.Pool, backend, "sites-integration-sign-key", testutil.DiscardLogger(), 0)

	srv := server.New(cfg, testutil.DiscardLogger(), nil, pg.Pool, nil, storageSvc)
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return ts, srv, loginAdminToken(t, ts, cfg.Admin.Password), storageSvc, pg
}

func requestWithHost(t *testing.T, method, rawURL, host string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = host

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request with host: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, body
}

func loginAdminToken(t *testing.T, ts *httptest.Server, password string) string {
	t.Helper()
	status, body := requestJSON(
		t,
		http.MethodPost,
		ts.URL+"/api/admin/auth",
		map[string]string{"password": password},
		"",
	)
	testutil.StatusCode(t, http.StatusOK, status)

	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode admin auth response: %v", err)
	}
	testutil.True(t, response.Token != "", "expected non-empty admin token")
	return response.Token
}

func (c adminHTTPClient) doJSON(
	t *testing.T,
	method string,
	path string,
	requestBody any,
	responseBody any,
) int {
	t.Helper()
	status, body := requestJSON(t, method, c.baseURL+path, requestBody, c.token)
	if responseBody != nil {
		if err := json.Unmarshal(body, responseBody); err != nil {
			t.Fatalf("decode %s %s response: %v (body: %s)", method, path, err, string(body))
		}
	}
	return status
}

func (c adminHTTPClient) uploadDeployFile(t *testing.T, siteID, deployID, name string, body []byte) int {
	t.Helper()

	payload := new(bytes.Buffer)
	writer := multipart.NewWriter(payload)
	if err := writer.WriteField("name", name); err != nil {
		t.Fatalf("write name field: %v", err)
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		c.baseURL+"/api/admin/sites/"+siteID+"/deploys/"+deployID+"/files",
		payload,
	)
	if err != nil {
		t.Fatalf("new upload request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func requestJSON(t *testing.T, method, url string, requestBody any, adminToken string) (int, []byte) {
	t.Helper()

	bodyBytes := []byte{}
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyBytes = payload
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	defer resp.Body.Close()

	responsePayload := new(bytes.Buffer)
	if _, err := responsePayload.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, responsePayload.Bytes()
}

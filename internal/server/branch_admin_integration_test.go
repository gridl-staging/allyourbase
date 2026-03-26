//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/allyourbase/ayb/internal/branching"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func requireBranchCloneTools(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not available, skipping branch admin integration test")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql not available, skipping branch admin integration test")
	}
}

// branchAdminLogin calls the admin login endpoint and returns the bearer token.
func branchAdminLogin(t *testing.T, ts *httptest.Server, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	resp, err := http.Post(ts.URL+"/api/admin/auth", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var result map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	token := result["token"]
	if token == "" {
		t.Fatal("admin login returned empty token")
	}
	return token
}

// adminRequest creates an HTTP request with the admin auth token.
func adminRequest(t *testing.T, method, url, token string, body []byte) *http.Request {
	t.Helper()
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// newBranchIntegrationServer creates a test server with real branch service wired.
func newBranchIntegrationServer(t *testing.T, ctx context.Context) *httptest.Server {
	t.Helper()

	createIntegrationTestSchema(t, ctx)
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "test-admin-pass"

	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	// Wire the real branch service — this is the key thing we're testing.
	branchRepo := branching.NewPgRepo(sharedPG.Pool)
	branchMgr := branching.NewManager(sharedPG.Pool, branchRepo, logger, branching.ManagerConfig{
		DefaultSourceURL: sharedPG.ConnString,
	})
	srv.SetBranchService(branchMgr)

	return httptest.NewServer(srv.Router())
}

// TestBranchServiceWiring_NonNil proves that SetBranchService was called
// and the branch endpoints no longer return 503.
func TestBranchServiceWiring_NonNil(t *testing.T) {
	ctx := context.Background()
	ts := newBranchIntegrationServer(t, ctx)
	defer ts.Close()

	token := branchAdminLogin(t, ts, "test-admin-pass")

	// GET /api/admin/branches — should return 200 with a real (empty) list,
	// not a 503 "service not configured" error.
	req := adminRequest(t, http.MethodGet, ts.URL+"/api/admin/branches", token, nil)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	branches, ok := body["branches"].([]any)
	testutil.True(t, ok, "expected branches array")
	testutil.Equal(t, 0, len(branches))
}

// TestBranchAdminAPI_CreateAndList proves a full round-trip:
// create a branch via POST, then list via GET and see it.
func TestBranchAdminAPI_CreateAndList(t *testing.T) {
	requireBranchCloneTools(t)

	ctx := context.Background()
	ts := newBranchIntegrationServer(t, ctx)
	defer ts.Close()

	token := branchAdminLogin(t, ts, "test-admin-pass")

	// POST /api/admin/branches — create a branch.
	createBody, _ := json.Marshal(map[string]string{"name": "integ-test-branch"})
	req := adminRequest(t, http.MethodPost, ts.URL+"/api/admin/branches", token, createBody)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)

	var created map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	testutil.Equal(t, "integ-test-branch", created["name"])

	// GET /api/admin/branches — should now list our branch.
	req2 := adminRequest(t, http.MethodGet, ts.URL+"/api/admin/branches", token, nil)
	resp2, err := http.DefaultClient.Do(req2)
	testutil.NoError(t, err)
	defer resp2.Body.Close()

	testutil.StatusCode(t, http.StatusOK, resp2.StatusCode)

	var listBody map[string]any
	testutil.NoError(t, json.NewDecoder(resp2.Body).Decode(&listBody))
	branches, ok := listBody["branches"].([]any)
	testutil.True(t, ok, "expected branches array")
	testutil.True(t, len(branches) >= 1, "expected at least 1 branch after create")

	// Cleanup: delete the branch so it doesn't leak into other tests.
	req3 := adminRequest(t, http.MethodDelete, ts.URL+"/api/admin/branches/integ-test-branch", token, nil)
	resp3, err := http.DefaultClient.Do(req3)
	testutil.NoError(t, err)
	defer resp3.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp3.StatusCode)
}

// TestBranchAdminAPI_CreateConflict proves 409 on duplicate branch name.
func TestBranchAdminAPI_CreateConflict(t *testing.T) {
	requireBranchCloneTools(t)

	ctx := context.Background()
	ts := newBranchIntegrationServer(t, ctx)
	defer ts.Close()

	token := branchAdminLogin(t, ts, "test-admin-pass")

	name := "conflict-test-branch"
	createBody, _ := json.Marshal(map[string]string{"name": name})

	// First create succeeds.
	req := adminRequest(t, http.MethodPost, ts.URL+"/api/admin/branches", token, createBody)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	resp.Body.Close()
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)

	// Second create with same name should conflict.
	req2 := adminRequest(t, http.MethodPost, ts.URL+"/api/admin/branches", token, createBody)
	resp2, err := http.DefaultClient.Do(req2)
	testutil.NoError(t, err)
	defer resp2.Body.Close()

	testutil.StatusCode(t, http.StatusConflict, resp2.StatusCode)

	var errBody map[string]any
	testutil.NoError(t, json.NewDecoder(resp2.Body).Decode(&errBody))
	errMsg, _ := errBody["error"].(string)
	testutil.Contains(t, errMsg, "already exists")

	// Cleanup.
	req3 := adminRequest(t, http.MethodDelete, ts.URL+"/api/admin/branches/"+name, token, nil)
	resp3, err := http.DefaultClient.Do(req3)
	testutil.NoError(t, err)
	resp3.Body.Close()
}

// TestBranchAdminAPI_DeleteNotFound proves 404 on deleting a nonexistent branch.
func TestBranchAdminAPI_DeleteNotFound(t *testing.T) {
	ctx := context.Background()
	ts := newBranchIntegrationServer(t, ctx)
	defer ts.Close()

	token := branchAdminLogin(t, ts, "test-admin-pass")

	req := adminRequest(t, http.MethodDelete, ts.URL+"/api/admin/branches/does-not-exist", token, nil)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)
}

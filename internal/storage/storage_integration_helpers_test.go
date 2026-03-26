//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

type requestHeaders struct {
	token    string
	tenantID string
}

func (h requestHeaders) apply(req *http.Request) {
	if req == nil {
		return
	}
	if token := strings.TrimSpace(h.token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenantID := strings.TrimSpace(h.tenantID); tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
}

func uploadFile(t *testing.T, baseURL, bucket, filename, bodyText string, headers requestHeaders) *http.Response {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", filename)
	testutil.NoError(t, err)
	_, err = fw.Write([]byte(bodyText))
	testutil.NoError(t, err)
	testutil.NoError(t, w.Close())

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/storage/"+bucket, body)
	testutil.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	headers.apply(req)

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	return resp
}

func uploadStatus(t *testing.T, baseURL, bucket, filename, bodyText string, headers requestHeaders) int {
	t.Helper()
	resp := uploadFile(t, baseURL, bucket, filename, bodyText, headers)
	defer resp.Body.Close()
	return resp.StatusCode
}

func uploadWithToken(t *testing.T, baseURL, token, bucket, filename, bodyText string) int {
	t.Helper()
	return uploadStatus(t, baseURL, bucket, filename, bodyText, requestHeaders{token: token})
}

func uploadWithTenant(t *testing.T, baseURL, tenantID, bucket, filename, bodyText string) *http.Response {
	t.Helper()
	return uploadFile(t, baseURL, bucket, filename, bodyText, requestHeaders{tenantID: tenantID})
}

func clearTenantQuotaData(t *testing.T) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(context.Background(), `
		TRUNCATE TABLE _ayb_tenant_quotas, _ayb_tenant_usage_daily, _ayb_tenants, _ayb_tenant_memberships CASCADE
	`)
	testutil.NoError(t, err)
}

// createQuotaTestTenant keeps storage quota integration setups repeatable when
// multiple tests share the same backing database.
func createQuotaTestTenant(t *testing.T, ctx context.Context, tenantSvc *tenant.Service, slugPrefix string) *tenant.Tenant {
	t.Helper()
	slug := fmt.Sprintf("%s-%d", slugPrefix, time.Now().UnixNano())
	tenantEnt, err := tenantSvc.CreateTenant(ctx, slug, slug, "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	return tenantEnt
}

func setupServerWithTenantStorageQuotas(t *testing.T, hard, soft *int64) (*httptest.Server, *tenant.Service, string) {
	t.Helper()
	ctx := context.Background()
	pool := sharedPG.Pool
	logger := testutil.DiscardLogger()

	runner := migrations.NewRunner(pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	dir := t.TempDir()
	backend, err := storage.NewLocalBackend(dir)
	testutil.NoError(t, err)
	storageSvc := storage.NewService(pool, backend, "test-sign-key-at-least-32-chars!!", logger, 0)

	cfg := config.Default()
	cfg.Storage.Enabled = true
	cfg.Admin.Password = "admin-pass"

	ch := schema.NewCacheHolder(pool, logger)
	srv := server.New(cfg, logger, ch, pool, nil, storageSvc)

	tenantSvc := tenant.NewService(pool, logger)
	usageAcc := tenant.NewUsageAccumulator(pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})

	tenantEnt := createQuotaTestTenant(t, ctx, tenantSvc, "quota-tenant")

	_, err = tenantSvc.SetQuotas(ctx, tenantEnt.ID, tenant.TenantQuotas{
		DBSizeBytesHard: hard,
		DBSizeBytesSoft: soft,
	})
	testutil.NoError(t, err)

	return httptest.NewServer(srv.Router()), tenantSvc, tenantEnt.ID
}

func createResumableSession(t *testing.T, baseURL, token, bucket, name string, length int64) (location string, id string) {
	t.Helper()
	return createResumableSessionWithHeaders(
		t,
		baseURL,
		bucket,
		name,
		length,
		requestHeaders{token: token},
	)
}

func createResumableSessionWithHeaders(t *testing.T, baseURL, bucket, name string, length int64, headers requestHeaders) (location string, id string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/storage/upload/resumable?bucket=%s&name=%s", baseURL, bucket, name), nil)
	testutil.NoError(t, err)
	headers.apply(req)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.FormatInt(length, 10))

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	defer resp.Body.Close()

	var payload map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	location = resp.Header.Get("Location")
	testutil.True(t, location != "", "expected Location header")

	parsedID, ok := payload["id"].(string)
	testutil.True(t, ok && parsedID != "", "expected resumable ID in response")

	return location, parsedID
}

func patchResumableChunk(t *testing.T, baseURL, token, id string, offset int64, chunk []byte) *http.Response {
	t.Helper()
	return patchResumableChunkWithHeaders(
		t,
		baseURL,
		id,
		offset,
		chunk,
		requestHeaders{token: token},
	)
}

func patchResumableChunkWithHeaders(t *testing.T, baseURL, id string, offset int64, chunk []byte, headers requestHeaders) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/api/storage/upload/resumable/%s", baseURL, id), bytes.NewReader(chunk))
	testutil.NoError(t, err)
	headers.apply(req)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	req.Header.Set("Content-Type", "application/offset+octet-stream")

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	return resp
}

func headResumableSession(t *testing.T, baseURL, token, id string) *http.Response {
	t.Helper()
	return headResumableSessionWithHeaders(t, baseURL, id, requestHeaders{token: token})
}

func headResumableSessionWithHeaders(t *testing.T, baseURL, id string, headers requestHeaders) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodHead, fmt.Sprintf("%s/api/storage/upload/resumable/%s", baseURL, id), nil)
	testutil.NoError(t, err)
	headers.apply(req)
	req.Header.Set("Tus-Resumable", "1.0.0")

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	return resp
}

func parseOffsetHeader(t *testing.T, hdr string) int64 {
	t.Helper()
	parsed, err := strconv.ParseInt(hdr, 10, 64)
	testutil.NoError(t, err)
	return parsed
}

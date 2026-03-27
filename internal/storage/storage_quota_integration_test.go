//go:build integration

package storage_test

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
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

// setupServerWithQuota creates a test server with a small per-user storage quota.
// It wires the tenant service and creates a default tenant so that storage write
// routes pass the enforceTenantContext middleware chain. Returns the test server,
// storage service, auth service, and the default tenant ID.
func setupServerWithQuota(t *testing.T, quotaBytes int64) (*httptest.Server, *storage.Service, *auth.Service, string) {
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
	storageSvc := storage.NewService(pool, backend, "test-sign-key-at-least-32-chars!!", logger, quotaBytes)

	cfg := config.Default()
	cfg.Storage.Enabled = true
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "jwt-secret-test-at-least-32-chars!!"
	authSvc := auth.NewService(pool, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)

	ch := schema.NewCacheHolder(pool, logger)
	srv := server.New(cfg, logger, ch, pool, authSvc, storageSvc)

	tenantSvc := tenant.NewService(pool, logger)
	usageAcc := tenant.NewUsageAccumulator(pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	orgMembershipStore := tenant.NewPostgresOrgMembershipStore(pool, logger)
	teamMembershipStore := tenant.NewPostgresTeamMembershipStore(pool, logger)
	teamStore := tenant.NewPostgresTeamStore(pool, logger)
	srv.SetPermissionResolver(tenant.NewPermissionResolver(tenantSvc, orgMembershipStore, teamMembershipStore, teamStore))

	tenantEnt := createQuotaTestTenant(t, ctx, tenantSvc, "quota-test")

	return httptest.NewServer(srv.Router()), storageSvc, authSvc, tenantEnt.ID
}

func setupServerWithTenantAuthAndStorageAdmin(t *testing.T) (*httptest.Server, *storage.Service, *auth.Service, string) {
	t.Helper()
	return setupServerWithQuota(t, 0)
}

func setupServerWithTenantAuthAndStorageAdminAndMaxFile(t *testing.T, maxSize string) (*httptest.Server, *storage.Service, *auth.Service, string) {
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
	cfg.Storage.MaxFileSize = maxSize
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "jwt-secret-test-at-least-32-chars!!"
	authSvc := auth.NewService(pool, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)

	ch := schema.NewCacheHolder(pool, logger)
	srv := server.New(cfg, logger, ch, pool, authSvc, storageSvc)

	tenantSvc := tenant.NewService(pool, logger)
	usageAcc := tenant.NewUsageAccumulator(pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	orgMembershipStore := tenant.NewPostgresOrgMembershipStore(pool, logger)
	teamMembershipStore := tenant.NewPostgresTeamMembershipStore(pool, logger)
	teamStore := tenant.NewPostgresTeamStore(pool, logger)
	srv.SetPermissionResolver(tenant.NewPermissionResolver(tenantSvc, orgMembershipStore, teamMembershipStore, teamStore))

	tenantEnt := createQuotaTestTenant(t, ctx, tenantSvc, "resumable-max-file")

	return httptest.NewServer(srv.Router()), storageSvc, authSvc, tenantEnt.ID
}

// ensureStorageTestUser creates the backing auth row expected by tenant-gated
// storage routes while resetting per-user quota overrides so reruns remain
// deterministic on the shared integration database.
func ensureStorageTestUser(t *testing.T, userID, email string) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(
		context.Background(),
		`INSERT INTO _ayb_users (id, email, password_hash)
		 VALUES ($1, $2, 'integration-password-hash')
		 ON CONFLICT (id) DO UPDATE
		 SET email = EXCLUDED.email,
		     password_hash = EXCLUDED.password_hash,
		     storage_quota_mb = NULL`,
		userID,
		email,
	)
	testutil.NoError(t, err)
}

// addStorageTestMembership adds the user as a member of the given tenant so the
// tenant permission middleware accepts storage write requests.
func addStorageTestMembership(t *testing.T, tenantID, userID string) {
	t.Helper()
	tenantSvc := tenant.NewService(sharedPG.Pool, testutil.DiscardLogger())
	_, err := tenantSvc.AddMembership(context.Background(), tenantID, userID, tenant.MemberRoleMember)
	testutil.NoError(t, err)
}

func clearQuotaData(t *testing.T) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(context.Background(),
		"TRUNCATE _ayb_storage_uploads, _ayb_storage_objects, _ayb_storage_buckets, _ayb_storage_usage")
	testutil.NoError(t, err)
}

func TestStorageUploadQuotaExceeded(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithQuota(t, 100)
	defer ts.Close()
	clearQuotaData(t)

	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	token := userToken(t, authSvc, userID, "quota-user@example.com")
	ensureStorageTestUser(t, userID, "quota-user@example.com")
	addStorageTestMembership(t, tenantID, userID)
	bucket := fmt.Sprintf("quota-test-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	bigData := strings.Repeat("x", 200)
	status := uploadStatus(t, ts.URL, bucket, "big.txt", bigData, requestHeaders{token: token, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusRequestEntityTooLarge, status)
}

func TestTenantStorageUploadQuotaExceeded(t *testing.T) {
	hard := int64(10)
	ts, _, tenantID := setupServerWithTenantStorageQuotas(t, &hard, nil)
	defer ts.Close()
	clearStorageData(t)

	bucket := fmt.Sprintf("tenant-quota-%d", time.Now().UnixNano())
	resp := uploadWithTenant(t, ts.URL, tenantID, bucket, "big.txt", strings.Repeat("x", 20))
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func TestTenantStorageUploadQuotaWarning(t *testing.T) {
	soft := int64(10)
	hard := int64(100)
	ts, _, tenantID := setupServerWithTenantStorageQuotas(t, &hard, &soft)
	defer ts.Close()
	clearStorageData(t)

	bucket := fmt.Sprintf("tenant-warning-%d", time.Now().UnixNano())
	resp1 := uploadWithTenant(t, ts.URL, tenantID, bucket, "file1.txt", strings.Repeat("x", 9))
	defer resp1.Body.Close()
	testutil.StatusCode(t, http.StatusCreated, resp1.StatusCode)
	testutil.Equal(t, "", resp1.Header.Get("X-Tenant-Quota-Warning"))

	resp2 := uploadWithTenant(t, ts.URL, tenantID, bucket, "file2.txt", strings.Repeat("y", 2))
	defer resp2.Body.Close()
	testutil.StatusCode(t, http.StatusCreated, resp2.StatusCode)
	testutil.Equal(t, "storage", resp2.Header.Get("X-Tenant-Quota-Warning"))
}

func TestStorageUploadQuotaAllowed(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithQuota(t, 1024)
	defer ts.Close()
	clearQuotaData(t)

	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	token := userToken(t, authSvc, userID, "quota-ok@example.com")
	ensureStorageTestUser(t, userID, "quota-ok@example.com")
	addStorageTestMembership(t, tenantID, userID)
	bucket := fmt.Sprintf("quota-ok-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	infoBefore, err := storageSvc.GetUsage(context.Background(), userID)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(0), infoBefore.BytesUsed)

	fileData := strings.Repeat("x", 50)
	status := uploadStatus(t, ts.URL, bucket, "small.txt", fileData, requestHeaders{token: token, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusCreated, status)

	info, err := storageSvc.GetUsage(context.Background(), userID)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(fileData)), info.BytesUsed)
}

func TestStorageResumableCreateQuotaExceeded(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithQuota(t, 100)
	defer ts.Close()
	clearQuotaData(t)

	userID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	token := userToken(t, authSvc, userID, "tus-quota@example.com")
	ensureStorageTestUser(t, userID, "tus-quota@example.com")
	addStorageTestMembership(t, tenantID, userID)
	bucket := fmt.Sprintf("tus-quota-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/storage/upload/resumable?bucket=%s&name=big.bin", ts.URL, bucket), nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Tenant-ID", tenantID)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", "200")

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusRequestEntityTooLarge, resp.StatusCode)

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	msg, _ := body["message"].(string)
	testutil.Contains(t, msg, "quota")
}

func TestStorageDeleteReclaimsQuota(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithQuota(t, 10*1024)
	defer ts.Close()
	clearQuotaData(t)

	userID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	token := userToken(t, authSvc, userID, "reclaim@example.com")
	ensureStorageTestUser(t, userID, "reclaim@example.com")
	addStorageTestMembership(t, tenantID, userID)
	bucket := fmt.Sprintf("reclaim-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	fileData := strings.Repeat("d", 500)
	status := uploadStatus(t, ts.URL, bucket, "deleteme.txt", fileData, requestHeaders{token: token, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusCreated, status)

	infoBefore, err := storageSvc.GetUsage(context.Background(), userID)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(fileData)), infoBefore.BytesUsed)

	delReq, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/storage/%s/deleteme.txt", ts.URL, bucket), nil)
	testutil.NoError(t, err)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delReq.Header.Set("X-Tenant-ID", tenantID)
	delResp, err := http.DefaultClient.Do(delReq)
	testutil.NoError(t, err)
	defer delResp.Body.Close()
	testutil.StatusCode(t, http.StatusNoContent, delResp.StatusCode)

	infoAfter, err := storageSvc.GetUsage(context.Background(), userID)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(0), infoAfter.BytesUsed)
}

func TestStorageQuotaPerUserOverride(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithQuota(t, 100)
	defer ts.Close()
	clearQuotaData(t)

	userID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	token := userToken(t, authSvc, userID, "override@example.com")
	ensureStorageTestUser(t, userID, "override@example.com")
	addStorageTestMembership(t, tenantID, userID)
	bucket := fmt.Sprintf("override-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	oneMB := 1
	err = storageSvc.SetUserQuota(context.Background(), userID, &oneMB)
	testutil.NoError(t, err)

	data200 := strings.Repeat("e", 200)
	status := uploadStatus(t, ts.URL, bucket, "override.txt", data200, requestHeaders{token: token, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusCreated, status)
}

func TestStorageAdminQuotaGetSet(t *testing.T) {
	ts, _, _, _ := setupServerWithQuota(t, 1024)
	defer ts.Close()
	clearQuotaData(t)

	userID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	ensureStorageTestUser(t, userID, "admin-quota@example.com")
	adminJWT := adminToken(t, ts.URL)

	getReq, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/admin/users/%s/storage-quota", ts.URL, userID), nil)
	testutil.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer "+adminJWT)
	getResp, err := http.DefaultClient.Do(getReq)
	testutil.NoError(t, err)
	defer getResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, getResp.StatusCode)

	var info storage.QuotaInfo
	testutil.NoError(t, json.NewDecoder(getResp.Body).Decode(&info))
	testutil.Equal(t, int64(0), info.BytesUsed)
	testutil.Equal(t, int64(1024), info.QuotaBytes)

	putBody := strings.NewReader(`{"quota_mb": 5}`)
	putReq, err := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/admin/users/%s/storage-quota", ts.URL, userID), putBody)
	testutil.NoError(t, err)
	putReq.Header.Set("Authorization", "Bearer "+adminJWT)
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(putReq)
	testutil.NoError(t, err)
	defer putResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, putResp.StatusCode)

	getReq2, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/admin/users/%s/storage-quota", ts.URL, userID), nil)
	testutil.NoError(t, err)
	getReq2.Header.Set("Authorization", "Bearer "+adminJWT)
	getResp2, err := http.DefaultClient.Do(getReq2)
	testutil.NoError(t, err)
	defer getResp2.Body.Close()
	testutil.StatusCode(t, http.StatusOK, getResp2.StatusCode)

	var info2 storage.QuotaInfo
	testutil.NoError(t, json.NewDecoder(getResp2.Body).Decode(&info2))
	fiveMB := 5
	testutil.NotNil(t, info2.QuotaMB)
	testutil.Equal(t, fiveMB, *info2.QuotaMB)
	testutil.Equal(t, int64(5*1024*1024), info2.QuotaBytes)
}

func TestStorageResumableFinalizeIncrementsUsage(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithQuota(t, 10*1024)
	defer ts.Close()
	clearQuotaData(t)

	userID := "11111111-1111-1111-1111-111111111111"
	token := userToken(t, authSvc, userID, "tus-usage@example.com")
	ensureStorageTestUser(t, userID, "tus-usage@example.com")
	addStorageTestMembership(t, tenantID, userID)
	bucket := fmt.Sprintf("tus-usage-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	data := []byte(strings.Repeat("t", 100))
	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "tus-file.bin", int64(len(data)), requestHeaders{token: token, tenantID: tenantID})

	patchResp := patchResumableChunkWithHeaders(t, ts.URL, id, 0, data, requestHeaders{token: token, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, patchResp.StatusCode)
	patchResp.Body.Close()

	info, err := storageSvc.GetUsage(context.Background(), userID)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(data)), info.BytesUsed)
}

func TestStorageQuotaConcurrentUploads(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithQuota(t, 500)
	defer ts.Close()
	clearQuotaData(t)

	userID := "22222222-2222-2222-2222-222222222222"
	token := userToken(t, authSvc, userID, "race@example.com")
	ensureStorageTestUser(t, userID, "race@example.com")
	addStorageTestMembership(t, tenantID, userID)
	bucket := fmt.Sprintf("race-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	const numUploads = 10
	type concurrentUploadResult struct {
		status int
		err    error
	}
	results := make(chan concurrentUploadResult, numUploads)
	data := strings.Repeat("r", 100)

	for i := 0; i < numUploads; i++ {
		go func(idx int) {
			name := fmt.Sprintf("race-%d.txt", idx)
			status, uploadErr := uploadStatusWithError(t, ts.URL, bucket, name, data, requestHeaders{token: token, tenantID: tenantID})
			results <- concurrentUploadResult{status: status, err: uploadErr}
		}(i)
	}

	var successes, quotaExceeded, other int
	for i := 0; i < numUploads; i++ {
		result := <-results
		testutil.NoError(t, result.err)
		switch result.status {
		case http.StatusCreated:
			successes++
		case http.StatusRequestEntityTooLarge:
			quotaExceeded++
		default:
			other++
		}
	}

	testutil.True(t, successes >= 1, fmt.Sprintf("expected at least 1 success, got %d", successes))
	testutil.True(t, quotaExceeded >= 1, fmt.Sprintf("expected concurrent quota rejections, got successes=%d quotaExceeded=%d", successes, quotaExceeded))
	testutil.Equal(t, 0, other)

	info, err := storageSvc.GetUsage(context.Background(), userID)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(successes*len(data)), info.BytesUsed)

	followUpStatus := uploadStatus(t, ts.URL, bucket, "post-race.txt", data, requestHeaders{token: token, tenantID: tenantID})
	expectedStatus := http.StatusCreated
	if info.BytesUsed+int64(len(data)) > 500 {
		expectedStatus = http.StatusRequestEntityTooLarge
	}
	testutil.StatusCode(t, expectedStatus, followUpStatus)
}

//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

func applyStorageTemplate(t *testing.T, baseURL, adminJWT, template, payload string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/admin/rls/templates/storage-objects/"+template, strings.NewReader(payload))
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

var (
	sharedPG      *testutil.PGContainer
	sharedCleanup func()
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	sharedCleanup = cleanup
	code := m.Run()
	sharedCleanup()
	os.Exit(code)
}

func setupServer(t *testing.T) *httptest.Server {
	t.Helper()

	ctx := context.Background()
	pool := sharedPG.Pool
	logger := testutil.DiscardLogger()

	// Run migrations.
	runner := migrations.NewRunner(pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create local storage backend.
	dir := t.TempDir()
	backend, err := storage.NewLocalBackend(dir)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}

	storageSvc := storage.NewService(pool, backend, "test-sign-key-at-least-32-chars!!", logger, 0)

	cfg := config.Default()
	cfg.Storage.Enabled = true
	ch := schema.NewCacheHolder(pool, logger)

	srv := server.New(cfg, logger, ch, pool, nil, storageSvc)
	return httptest.NewServer(srv.Router())
}

func setupServerWithAuthAndStorageAdmin(t *testing.T) (*httptest.Server, *storage.Service, *auth.Service) {
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
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "jwt-secret-test-at-least-32-chars!!"
	authSvc := auth.NewService(pool, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)

	ch := schema.NewCacheHolder(pool, logger)
	srv := server.New(cfg, logger, ch, pool, authSvc, storageSvc)
	return httptest.NewServer(srv.Router()), storageSvc, authSvc
}

func adminToken(t *testing.T, baseURL string) string {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/admin/auth", "application/json", strings.NewReader(`{"password":"admin-pass"}`))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	var payload map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	token := payload["token"]
	testutil.True(t, token != "", "expected admin token")
	return token
}

func userToken(t *testing.T, authSvc *auth.Service, userID, email string) string {
	t.Helper()
	token, err := authSvc.IssueTestToken(userID, email)
	testutil.NoError(t, err)
	testutil.True(t, token != "", "expected user token")
	return token
}

func clearStorageData(t *testing.T) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(context.Background(), "TRUNCATE _ayb_storage_uploads, _ayb_storage_objects, _ayb_storage_buckets")
	testutil.NoError(t, err)
}

func TestStorageUploadAndServe(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload a file.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "hello.txt")
	fw.Write([]byte("Hello, Storage!"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/storage/testbucket", w.FormDataContentType(), body)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)

	var obj map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&obj))
	resp.Body.Close()

	testutil.Equal(t, "testbucket", obj["bucket"])
	testutil.Equal(t, "hello.txt", obj["name"])
	testutil.Equal(t, float64(15), obj["size"].(float64))

	// Serve the file.
	resp, err = http.Get(ts.URL + "/api/storage/testbucket/hello.txt")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.Equal(t, "Hello, Storage!", string(got))
}

func TestStorageDelete(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "delete-me.txt")
	fw.Write([]byte("bye"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/storage/testbucket", w.FormDataContentType(), body)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Delete.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/testbucket/delete-me.txt", nil)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Serve should 404.
	resp, _ = http.Get(ts.URL + "/api/storage/testbucket/delete-me.txt")
	testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestStorageList(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload 3 files.
	for i := 0; i < 3; i++ {
		body := &bytes.Buffer{}
		w := multipart.NewWriter(body)
		fw, err := w.CreateFormFile("file", fmt.Sprintf("file%d.txt", i))
		testutil.NoError(t, err)
		_, err = fw.Write([]byte(fmt.Sprintf("content %d", i)))
		testutil.NoError(t, err)
		w.Close()
		resp, err := http.Post(ts.URL+"/api/storage/listbucket", w.FormDataContentType(), body)
		testutil.NoError(t, err)
		testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	resp, err := http.Get(ts.URL + "/api/storage/listbucket")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var list map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	resp.Body.Close()

	testutil.Equal(t, float64(3), list["totalItems"].(float64))
	items := list["items"].([]any)
	testutil.Equal(t, 3, len(items))
}

func TestStorageSignedURL(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "signed.txt")
	fw.Write([]byte("signed content"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/storage/signbucket", w.FormDataContentType(), body)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Generate signed URL.
	signBody := bytes.NewReader([]byte(`{"expiresIn": 3600}`))
	resp, err = http.Post(ts.URL+"/api/storage/signbucket/signed.txt/sign", "application/json", signBody)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var signResp map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&signResp))
	resp.Body.Close()

	signedURL := signResp["url"]
	testutil.True(t, signedURL != "", "should have a URL")

	// Fetch via signed URL.
	resp, err = http.Get(ts.URL + signedURL)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.Equal(t, "signed content", string(got))
}

func TestStorageBucketAPICreateUpdateDelete(t *testing.T) {
	ts, storageSvc, _ := setupServerWithAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	ctx := context.Background()

	name := fmt.Sprintf("api-bucket-%d", time.Now().UnixNano())
	payload := fmt.Sprintf(`{"name":"%s","public":false}`, name)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets", strings.NewReader(payload))
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)

	var bucket storage.Bucket
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&bucket))
	resp.Body.Close()
	testutil.Equal(t, name, bucket.Name)
	testutil.False(t, bucket.Public)

	req, err = http.NewRequest(http.MethodGet, ts.URL+"/api/storage/buckets", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var listed struct {
		Items []storage.Bucket `json:"items"`
	}
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
	resp.Body.Close()
	testutil.True(t, len(listed.Items) >= 1, "expected at least one bucket")

	payload = `{"public":true}`
	req, err = http.NewRequest(http.MethodPut, ts.URL+"/api/storage/buckets/"+name, strings.NewReader(payload))
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var updated storage.Bucket
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	resp.Body.Close()
	testutil.True(t, updated.Public)

	_, err = storageSvc.Upload(ctx, name, "bucket-object.txt", "text/plain", nil, strings.NewReader("abc"))
	testutil.NoError(t, err)

	req, err = http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/buckets/"+name, nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/buckets/"+name+"?force=true", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	_, err = storageSvc.GetBucket(ctx, name)
	testutil.ErrorContains(t, err, "bucket not found")
}

func TestStorageBucketACLAndCacheBehavior(t *testing.T) {
	ts, storageSvc, authSvc := setupServerWithAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	userJWT := userToken(t, authSvc, "user-1", "user-1@example.com")
	ctx := context.Background()

	publicName := fmt.Sprintf("public-%d", time.Now().UnixNano())
	privateName := fmt.Sprintf("private-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(ctx, publicName, true)
	testutil.NoError(t, err)
	_, err = storageSvc.CreateBucket(ctx, privateName, false)
	testutil.NoError(t, err)

	_, err = storageSvc.Upload(ctx, publicName, "public.txt", "text/plain", nil, strings.NewReader("public-data"))
	testutil.NoError(t, err)
	_, err = storageSvc.Upload(ctx, privateName, "private.txt", "text/plain", nil, strings.NewReader("private-data"))
	testutil.NoError(t, err)

	resp, err := http.Get(ts.URL + "/api/storage/" + publicName + "/public.txt")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/storage/" + privateName + "/private.txt")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+privateName+"/private.txt", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+privateName+"/private.txt", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))
	resp.Body.Close()

	sign := storageSvc.SignURL(privateName, "private.txt", time.Hour)
	resp, err = http.Get(ts.URL + "/api/storage/" + privateName + "/private.txt?" + sign)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))
	resp.Body.Close()
}

func TestStorageBucketServiceLifecycle(t *testing.T) {
	_, storageSvc, _ := setupServerWithAuthAndStorageAdmin(t)
	clearStorageData(t)
	ctx := context.Background()

	publicName := fmt.Sprintf("lifecycle-%d", time.Now().UnixNano())
	privateName := fmt.Sprintf("lifecycle-private-%d", time.Now().UnixNano())

	pub, err := storageSvc.CreateBucket(ctx, publicName, true)
	testutil.NoError(t, err)
	testutil.Equal(t, publicName, pub.Name)
	testutil.True(t, pub.Public)

	_, err = storageSvc.CreateBucket(ctx, publicName, false)
	testutil.ErrorContains(t, err, "object already exists")

	pubBucket, err := storageSvc.GetBucket(ctx, publicName)
	testutil.NoError(t, err)
	testutil.Equal(t, publicName, pubBucket.Name)

	_, err = storageSvc.GetBucket(ctx, "missing-bucket")
	testutil.ErrorContains(t, err, "bucket not found")

	_, err = storageSvc.CreateBucket(ctx, privateName, false)
	testutil.NoError(t, err)

	buckets, err := storageSvc.ListBuckets(ctx)
	testutil.NoError(t, err)
	testutil.True(t, len(buckets) >= 2, "expected at least two buckets")

	private, err := storageSvc.UpdateBucket(ctx, privateName, true)
	testutil.NoError(t, err)
	testutil.True(t, private.Public)

	_, err = storageSvc.Upload(ctx, privateName, "file.txt", "text/plain", nil, strings.NewReader("hello"))
	testutil.NoError(t, err)

	err = storageSvc.DeleteBucket(ctx, privateName, false)
	testutil.ErrorContains(t, err, "bucket has objects")

	err = storageSvc.DeleteBucket(ctx, privateName, true)
	testutil.NoError(t, err)

	_, err = storageSvc.GetBucket(ctx, privateName)
	testutil.ErrorContains(t, err, "bucket not found")
}

func TestStorageRLSUserIsolationAdminBypassAndPolicyUpdate(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	userAID := "11111111-1111-1111-1111-111111111111"
	userBID := "22222222-2222-2222-2222-222222222222"
	ensureStorageTestUser(t, userAID, "a@example.com")
	ensureStorageTestUser(t, userBID, "b@example.com")
	addStorageTestMembership(t, tenantID, userAID)
	addStorageTestMembership(t, tenantID, userBID)
	userA := userToken(t, authSvc, userAID, "a@example.com")
	userB := userToken(t, authSvc, userBID, "b@example.com")

	bucket := fmt.Sprintf("rls-private-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	status := applyStorageTemplate(t, ts.URL, adminJWT, "user-own-files", `{"prefix":"storage_owner"}`)
	testutil.Equal(t, http.StatusCreated, status)

	testutil.Equal(t, http.StatusCreated, uploadStatus(t, ts.URL, bucket, "a.txt", "owner-a", requestHeaders{token: userA, tenantID: tenantID}))
	testutil.Equal(t, http.StatusCreated, uploadStatus(t, ts.URL, bucket, "b.txt", "owner-b", requestHeaders{token: userB, tenantID: tenantID}))

	reqA, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket, nil)
	testutil.NoError(t, err)
	reqA.Header.Set("Authorization", "Bearer "+userA)
	respA, err := http.DefaultClient.Do(reqA)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respA.StatusCode)
	var listA struct {
		Items []storage.Object `json:"items"`
	}
	testutil.NoError(t, json.NewDecoder(respA.Body).Decode(&listA))
	respA.Body.Close()
	testutil.Equal(t, 1, len(listA.Items))
	testutil.Equal(t, "a.txt", listA.Items[0].Name)

	reqUserBReadA, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/a.txt", nil)
	testutil.NoError(t, err)
	reqUserBReadA.Header.Set("Authorization", "Bearer "+userB)
	respUserBReadA, err := http.DefaultClient.Do(reqUserBReadA)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNotFound, respUserBReadA.StatusCode)
	respUserBReadA.Body.Close()

	reqAdminReadA, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/a.txt", nil)
	testutil.NoError(t, err)
	reqAdminReadA.Header.Set("Authorization", "Bearer "+adminJWT)
	respAdminReadA, err := http.DefaultClient.Do(reqAdminReadA)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respAdminReadA.StatusCode)
	respAdminReadA.Body.Close()

	status = applyStorageTemplate(t, ts.URL, adminJWT, "public-read-auth-write", `{"prefix":"storage_public_auth"}`)
	testutil.Equal(t, http.StatusCreated, status)

	reqUserBReadA2, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/a.txt", nil)
	testutil.NoError(t, err)
	reqUserBReadA2.Header.Set("Authorization", "Bearer "+userB)
	respUserBReadA2, err := http.DefaultClient.Do(reqUserBReadA2)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respUserBReadA2.StatusCode)
	respUserBReadA2.Body.Close()
}

func TestStorageResumableUploadCreateResumeComplete(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	ensureStorageTestUser(t, userID, "resumable-user@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user@example.com")
	_, _ = adminJWT, userJWT
	_ = storageSvc

	bucket := fmt.Sprintf("resumable-%d", time.Now().UnixNano())
	ctx := context.Background()
	_, err := storageSvc.CreateBucket(ctx, bucket, false)
	testutil.NoError(t, err)

	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "hello.txt", 12, requestHeaders{token: userJWT, tenantID: tenantID})

	resp := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("hello"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	testutil.Equal(t, "5", resp.Header.Get("Upload-Offset"))
	testutil.Equal(t, "12", resp.Header.Get("Upload-Length"))
	resp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 5, []byte(" world!"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	testutil.Equal(t, "12", resp.Header.Get("Upload-Offset"))
	testutil.Equal(t, "12", resp.Header.Get("Upload-Length"))
	resp.Body.Close()

	getReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/hello.txt", nil)
	testutil.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer "+userJWT)
	getResp, err := http.DefaultClient.Do(getReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, getResp.StatusCode)
	body, err := io.ReadAll(getResp.Body)
	testutil.NoError(t, err)
	getResp.Body.Close()
	testutil.Equal(t, "hello world!", string(body))
}

func TestStorageResumableUploadOffsetMismatch(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	ensureStorageTestUser(t, userID, "resumable-user2@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user2@example.com")
	bucket := fmt.Sprintf("resumable-offset-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "bad-offset.txt", 6, requestHeaders{token: userJWT, tenantID: tenantID})

	resp := patchResumableChunkWithHeaders(t, ts.URL, id, 2, []byte("abc"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	headResp := headResumableSessionWithHeaders(t, ts.URL, id, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusOK, headResp.StatusCode)
	testutil.Equal(t, int64(0), parseOffsetHeader(t, headResp.Header.Get("Upload-Offset")))
	headResp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("abc"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 3, []byte("def"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()
}

func TestStorageResumableUploadInterruptedResume(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	userID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	ensureStorageTestUser(t, userID, "resumable-user3@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user3@example.com")
	bucket := fmt.Sprintf("resumable-reconnect-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "resume.txt", 11, requestHeaders{token: userJWT, tenantID: tenantID})

	resp := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("part"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	head := headResumableSessionWithHeaders(t, ts.URL, id, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusOK, head.StatusCode)
	testutil.Equal(t, int64(4), parseOffsetHeader(t, head.Header.Get("Upload-Offset")))
	head.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 1, []byte("wrong"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 4, []byte("resume!"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	getReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/resume.txt", nil)
	testutil.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer "+userJWT)
	getResp, err := http.DefaultClient.Do(getReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, getResp.StatusCode)
	got, err := io.ReadAll(getResp.Body)
	testutil.NoError(t, err)
	getResp.Body.Close()
	testutil.Equal(t, "partresume!", string(got))
}

func TestStorageResumableUploadOversizedChunkRejected(t *testing.T) {
	ts, storageSvc, _, tenantID := setupServerWithTenantAuthAndStorageAdminAndMaxFile(t, "1KB")
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	bucket := fmt.Sprintf("resumable-small-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	reqOpts, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/storage/upload/resumable", nil)
	testutil.NoError(t, err)
	reqOpts.Header.Set("Authorization", "Bearer "+adminJWT)
	reqOpts.Header.Set("X-Tenant-ID", tenantID)
	reqOpts.Header.Set("Tus-Resumable", "1.0.0")
	optionsResp, err := http.DefaultClient.Do(reqOpts)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, optionsResp.StatusCode)
	testutil.Equal(t, "1.0.0", optionsResp.Header.Get("Tus-Resumable"))
	testutil.Equal(t, "creation", optionsResp.Header.Get("Tus-Extension"))
	testutil.Equal(t, "1024", optionsResp.Header.Get("Tus-Max-Size"))
	optionsResp.Body.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/upload/resumable?bucket="+bucket+"&name=big.txt", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("X-Tenant-ID", tenantID)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.Itoa(2048))
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	resp.Body.Close()
}

func TestStorageResumableUploadExpiration(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	userID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	ensureStorageTestUser(t, userID, "resumable-user4@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user4@example.com")
	bucket := fmt.Sprintf("resumable-expire-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "expire.txt", 4, requestHeaders{token: userJWT, tenantID: tenantID})
	_, err = sharedPG.Pool.Exec(context.Background(),
		`UPDATE _ayb_storage_uploads SET expires_at = NOW() - interval '1 hour' WHERE id = $1`, id)
	testutil.NoError(t, err)

	head := headResumableSessionWithHeaders(t, ts.URL, id, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusGone, head.StatusCode)
	head.Body.Close()

	deleted, err := storageSvc.CleanupExpiredResumableUploads(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, deleted)
}

func TestStorageResumableUploadConcurrentIDs(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	userID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	ensureStorageTestUser(t, userID, "resumable-user5@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user5@example.com")
	bucket := fmt.Sprintf("resumable-concurrent-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	ids := map[string]struct{}{}
	for i := 0; i < 8; i++ {
		_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, fmt.Sprintf("c%d.txt", i), 1, requestHeaders{token: userJWT, tenantID: tenantID})
		_, exists := ids[id]
		testutil.False(t, exists)
		ids[id] = struct{}{}
	}
	testutil.Equal(t, 8, len(ids))
}

func TestStorageResumableUploadOwnerlessSessionAdminOnly(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	userID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	ensureStorageTestUser(t, userID, "resumable-user6@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user6@example.com")
	bucket := fmt.Sprintf("resumable-admin-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	// Admin-created sessions have no user_id, so they must remain admin-only.
	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "admin-owned.txt", 5, requestHeaders{token: adminJWT, tenantID: tenantID})

	headAsUser := headResumableSessionWithHeaders(t, ts.URL, id, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusForbidden, headAsUser.StatusCode)
	headAsUser.Body.Close()

	patchAsUser := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("hello"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusForbidden, patchAsUser.StatusCode)
	patchAsUser.Body.Close()

	// Admin can still continue the upload.
	patchAsAdmin := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("hello"), requestHeaders{token: adminJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, patchAsAdmin.StatusCode)
	patchAsAdmin.Body.Close()
}

package storage

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type countBackend struct {
	putCount int
	putErr   error
}

func (b *countBackend) Put(_ context.Context, bucket, name string, r io.Reader) (int64, error) {
	b.putCount++
	n, _ := io.Copy(io.Discard, r)
	return n, b.putErr
}

func (b *countBackend) Get(_ context.Context, bucket, name string) (io.ReadCloser, error) {
	return nil, nil
}

func (b *countBackend) Delete(_ context.Context, bucket, name string) error {
	return nil
}

func (b *countBackend) Exists(_ context.Context, bucket, name string) (bool, error) {
	return false, nil
}

type tenantQuotaReaderMock struct {
	quotas *tenant.TenantQuotas
	err    error
	called bool
}

type storageQuotaMetricsCapture struct {
	utilizationCalls int32
	violationCalls   int32
	lastTenantID     string
	lastResource     string
	lastCurrent      int64
	lastLimit        int64
}

func (m *storageQuotaMetricsCapture) RecordQuotaUtilization(_ context.Context, tenantID, resource string, current, limit int64) {
	atomic.AddInt32(&m.utilizationCalls, 1)
	m.lastTenantID = tenantID
	m.lastResource = resource
	m.lastCurrent = current
	m.lastLimit = limit
}

func (m *storageQuotaMetricsCapture) IncrQuotaViolation(_ context.Context, tenantID, resource string) {
	atomic.AddInt32(&m.violationCalls, 1)
	m.lastTenantID = tenantID
	m.lastResource = resource
}

type storageQuotaAuditCapture struct {
	tenantID string
	resource string
	current  int64
	limit    int64
	actorID  *string
	ipAddr   *string
}

func (m *storageQuotaAuditCapture) EmitQuotaViolation(_ context.Context, tenantID, resource string, current, limit int64, actorID *string, ipAddress *string) error {
	m.tenantID = tenantID
	m.resource = resource
	m.current = current
	m.limit = limit
	m.actorID = actorID
	m.ipAddr = ipAddress
	return nil
}

func (m *tenantQuotaReaderMock) GetQuotas(_ context.Context, tenantID string) (*tenant.TenantQuotas, error) {
	m.called = true
	return m.quotas, m.err
}

func newMultipartUploadRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	filePart, err := w.CreateFormFile("file", "upload.txt")
	testutil.NoError(t, err)
	_, err = filePart.Write(body)
	testutil.NoError(t, err)
	testutil.NoError(t, w.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/storage/images", buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func performUpload(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	router.Route("/api/storage", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandleUpload_HardTenantQuotaDenial(t *testing.T) {
	t.Parallel()
	backend := &countBackend{}
	svc := NewService(nil, backend, "test-sign-key", testutil.DiscardLogger(), 0)
	h := NewHandler(svc, testutil.DiscardLogger(), 1024, "")

	acc := tenant.NewUsageAccumulator(nil, nil)
	metricsCapture := &storageQuotaMetricsCapture{}
	auditCapture := &storageQuotaAuditCapture{}
	h.SetTenantQuota(&tenantQuotaReaderMock{
		quotas: &tenant.TenantQuotas{
			DBSizeBytesHard: ptrInt64(10),
		},
	}, tenant.DefaultQuotaChecker{}, acc)
	h.SetTenantQuotaTelemetry(metricsCapture, auditCapture)

	req := newMultipartUploadRequest(t, bytes.Repeat([]byte("x"), 20))
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "tenant-1"))
	claims := &auth.Claims{}
	claims.Subject = "cbf722d5-d03e-43ac-becf-f4dca1764f36"
	req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	req.Header.Set("X-Forwarded-For", "198.51.100.30")
	rec := performUpload(t, h, req)

	testutil.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	testutil.Equal(t, 0, backend.putCount)
	testutil.Contains(t, rec.Body.String(), "tenant storage quota exceeded")
	testutil.True(t, atomic.LoadInt32(&metricsCapture.utilizationCalls) > 0, "expected storage quota utilization metrics")
	testutil.Equal(t, int32(1), atomic.LoadInt32(&metricsCapture.violationCalls))
	testutil.Equal(t, "tenant-1", metricsCapture.lastTenantID)
	testutil.Equal(t, string(tenant.ResourceTypeDBSizeBytes), metricsCapture.lastResource)
	testutil.Equal(t, int64(20), metricsCapture.lastCurrent)
	testutil.Equal(t, int64(10), metricsCapture.lastLimit)
	testutil.Equal(t, "tenant-1", auditCapture.tenantID)
	testutil.Equal(t, string(tenant.ResourceTypeDBSizeBytes), auditCapture.resource)
	testutil.Equal(t, int64(20), auditCapture.current)
	testutil.Equal(t, int64(10), auditCapture.limit)
	if auditCapture.actorID == nil {
		t.Fatal("expected actor ID in storage quota audit event")
	}
	testutil.Equal(t, "cbf722d5-d03e-43ac-becf-f4dca1764f36", *auditCapture.actorID)
	if auditCapture.ipAddr == nil {
		t.Fatal("expected IP address in storage quota audit event")
	}
	testutil.Equal(t, "198.51.100.30", *auditCapture.ipAddr)
}

func TestHandleUpload_SoftTenantQuotaWarning(t *testing.T) {
	t.Parallel()
	backend := &countBackend{}
	svc := NewService(nil, backend, "test-sign-key", testutil.DiscardLogger(), 0)
	h := NewHandler(svc, testutil.DiscardLogger(), 1024, "")

	acc := tenant.NewUsageAccumulator(nil, nil)
	acc.Record("tenant-1", tenant.ResourceTypeDBSizeBytes, 45)
	h.SetTenantQuota(&tenantQuotaReaderMock{
		quotas: &tenant.TenantQuotas{
			DBSizeBytesSoft: ptrInt64(50),
		},
	}, tenant.DefaultQuotaChecker{}, acc)

	req := newMultipartUploadRequest(t, []byte("ten-bytes!!"))
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "tenant-1"))
	rec := performUpload(t, h, req)

	testutil.Equal(t, "storage", rec.Header().Get(headerTenantQuotaWarning))
	testutil.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code)
	testutil.Equal(t, 1, backend.putCount)
}

func TestHandleUpload_TenantWithoutQuotasPassesThrough(t *testing.T) {
	t.Parallel()
	backend := &countBackend{}
	svc := NewService(nil, backend, "test-sign-key", testutil.DiscardLogger(), 0)
	h := NewHandler(svc, testutil.DiscardLogger(), 1024, "")

	h.SetTenantQuota(&tenantQuotaReaderMock{
		quotas: nil,
	}, tenant.DefaultQuotaChecker{}, tenant.NewUsageAccumulator(nil, nil))

	req := newMultipartUploadRequest(t, bytes.Repeat([]byte("x"), 20))
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "tenant-1"))
	rec := performUpload(t, h, req)

	testutil.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code)
	testutil.Equal(t, 1, backend.putCount)
	testutil.Equal(t, "", rec.Header().Get(headerTenantQuotaWarning))
}

func ptrInt64(v int64) *int64 {
	return &v
}

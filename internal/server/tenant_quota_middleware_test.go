package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

type testQuotaReader struct {
	quotas *tenant.TenantQuotas
	err    error

	called int32
}

func (t *testQuotaReader) GetQuotas(_ context.Context, _ string) (*tenant.TenantQuotas, error) {
	atomic.StoreInt32(&t.called, 1)
	return t.quotas, t.err
}

type quotaAuditCapture struct {
	tenantID string
	action   string
	result   string
	metadata json.RawMessage
	actorID  *string
	ipAddr   *string
}

func (c *quotaAuditCapture) InsertAuditEvent(_ context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string) error {
	c.tenantID = tenantID
	c.action = action
	c.result = result
	c.metadata = append(c.metadata[:0], metadata...)
	c.actorID = actorID
	c.ipAddr = ipAddress
	return nil
}

type quotaMetricsCapture struct {
	utilizationCalls int32
	violationCalls   int32
	lastTenantID     string
	lastResource     string
	lastCurrent      int64
	lastLimit        int64
}

func (c *quotaMetricsCapture) RecordQuotaUtilization(_ context.Context, tenantID, resource string, current, limit int64) {
	atomic.AddInt32(&c.utilizationCalls, 1)
	c.lastTenantID = tenantID
	c.lastResource = resource
	c.lastCurrent = current
	c.lastLimit = limit
}

func (c *quotaMetricsCapture) IncrQuotaViolation(_ context.Context, tenantID, resource string) {
	atomic.AddInt32(&c.violationCalls, 1)
	c.lastTenantID = tenantID
	c.lastResource = resource
}

func TestTenantRequestRateMiddleware_DenyOnHardLimit(t *testing.T) {
	t.Parallel()

	hardLimit := 1
	limiter := tenant.NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() {
		limiter.Stop()
	})

	acc := tenant.NewUsageAccumulator(nil, nil)
	reader := &testQuotaReader{quotas: &tenant.TenantQuotas{RequestRateRPSHard: &hardLimit}}
	called := 0

	h := tenantRequestRateMiddleware(limiter, reader, acc)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called++
		// Each allowed request should pass through.
	}))

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-1"))
		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, i+1, called)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-1"))
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, strconv.Itoa(hardLimit*60), w.Header().Get(headerRateLimitLimit))
	testutil.Equal(t, "0", w.Header().Get(headerRateLimitRemaining))
	testutil.True(t, w.Header().Get(headerRetryAfter) != "")
	_, parseErr := strconv.Atoi(w.Header().Get(headerRateLimitReset))
	testutil.NoError(t, parseErr)

	var body map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	code, ok := body["code"].(float64)
	testutil.True(t, ok)
	testutil.Equal(t, float64(http.StatusTooManyRequests), code)
	testutil.Equal(t, int64(60), acc.GetCurrentWindow("tenant-1", tenant.ResourceTypeRequestRate))
	testutil.Equal(t, 1, int(atomic.LoadInt32(&reader.called)))
}

func TestTenantRequestRateMiddleware_SoftThresholdWarning(t *testing.T) {
	t.Parallel()

	hardLimit := 10
	softLimit := 1
	limiter := tenant.NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() {
		limiter.Stop()
	})

	acc := tenant.NewUsageAccumulator(nil, nil)
	head := tenantRequestRateMiddleware(limiter, &testQuotaReader{
		quotas: &tenant.TenantQuotas{
			RequestRateRPSHard: &hardLimit,
			RequestRateRPSSoft: &softLimit,
		},
	}, acc)

	middleware := head(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-2"))
		middleware.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		if i < 59 {
			testutil.Equal(t, "", w.Header().Get(headerTenantQuotaWarning))
		} else {
			testutil.Equal(t, "request_rate", w.Header().Get(headerTenantQuotaWarning))
		}
	}
	testutil.Equal(t, 60, calledRequests(t, acc, "tenant-2"))
}

func TestTenantRequestRateMiddleware_NoTenantInContextPassThrough(t *testing.T) {
	t.Parallel()

	hardLimit := 1
	limiter := tenant.NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() {
		limiter.Stop()
	})

	acc := tenant.NewUsageAccumulator(nil, nil)
	reader := &testQuotaReader{quotas: &tenant.TenantQuotas{RequestRateRPSHard: &hardLimit}}
	h := tenantRequestRateMiddleware(limiter, reader, acc)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, int32(0), atomic.LoadInt32(&reader.called))
}

func TestTenantWSAdmission_HardDenyAndRelease(t *testing.T) {
	t.Parallel()

	hardLimit := 1
	counter := tenant.NewTenantConnCounter()
	acc := tenant.NewUsageAccumulator(nil, nil)
	reader := &testQuotaReader{quotas: &tenant.TenantQuotas{RealtimeConnectionsHard: &hardLimit}}

	start := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	wsHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		select {
		case start <- struct{}{}:
		default:
		}
		<-release
	})
	adapter := tenantWSAdmission(counter, reader, acc, wsHandler)

	firstReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-realtime"))
	firstRec := httptest.NewRecorder()
	go func() {
		adapter.ServeHTTP(firstRec, firstReq)
		close(done)
	}()

	<-start
	testutil.Equal(t, int64(1), acc.GetCurrentPeakWindow("tenant-realtime", tenant.ResourceTypeRealtimeConns))

	secondReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-realtime"))
	secondRec := httptest.NewRecorder()
	adapter.ServeHTTP(secondRec, secondReq)
	testutil.Equal(t, http.StatusTooManyRequests, secondRec.Code)

	close(release)
	<-done
	testutil.Equal(t, int64(1), acc.GetCurrentPeakWindow("tenant-realtime", tenant.ResourceTypeRealtimeConns))

	thirdReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-realtime"))
	thirdRec := httptest.NewRecorder()
	adapter.ServeHTTP(thirdRec, thirdReq)
	testutil.Equal(t, http.StatusOK, thirdRec.Code)
}

func TestTenantWSAdmission_SoftAllowancePath(t *testing.T) {
	t.Parallel()

	hardLimit := 2
	softLimit := 1
	counter := tenant.NewTenantConnCounter()
	acc := tenant.NewUsageAccumulator(nil, nil)
	reader := &testQuotaReader{
		quotas: &tenant.TenantQuotas{
			RealtimeConnectionsHard: &hardLimit,
			RealtimeConnectionsSoft: &softLimit,
		},
	}

	start1 := make(chan struct{})
	start2 := make(chan struct{})
	release1 := make(chan struct{})
	release2 := make(chan struct{})
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	calls := int32(0)
	wsHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		callIdx := atomic.AddInt32(&calls, 1)
		switch callIdx {
		case 1:
			close(start1)
			<-release1
		case 2:
			close(start2)
			<-release2
		}
	})
	adapter := tenantWSAdmission(counter, reader, acc, wsHandler)

	req1 := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-soft"))
	req2 := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-soft"))
	req3 := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-soft"))

	rec1 := httptest.NewRecorder()
	rec2 := httptest.NewRecorder()
	go func() { adapter.ServeHTTP(rec1, req1); close(done1) }()
	go func() { adapter.ServeHTTP(rec2, req2); close(done2) }()

	<-start1
	<-start2
	testutil.Equal(t, int64(2), acc.GetCurrentPeakWindow("tenant-soft", tenant.ResourceTypeRealtimeConns))

	rec3 := httptest.NewRecorder()
	adapter.ServeHTTP(rec3, req3)
	testutil.Equal(t, http.StatusTooManyRequests, rec3.Code)

	close(release1)
	close(release2)
	<-done1
	<-done2
}

func calledRequests(t *testing.T, acc *tenant.UsageAccumulator, tenantID string) int {
	t.Helper()
	return int(acc.GetCurrentWindow(tenantID, tenant.ResourceTypeRequestRate))
}

func TestServerTenantRequestRateMiddlewareDynamic_UsesSetterWiringAfterConstruction(t *testing.T) {
	t.Parallel()

	s := &Server{}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := s.tenantRequestRateMiddlewareDynamic(next)

	hardLimit := 1
	s.tenantRateLimiter = tenant.NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() {
		s.tenantRateLimiter.Stop()
	})
	s.tenantQuotaReader = &testQuotaReader{quotas: &tenant.TenantQuotas{RequestRateRPSHard: &hardLimit}}
	s.usageAccumulator = tenant.NewUsageAccumulator(nil, nil)

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-dynamic-rate"))
		middleware.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-dynamic-rate"))
	middleware.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestServerTenantWSAdmissionDynamic_UsesSetterWiringAfterConstruction(t *testing.T) {
	t.Parallel()

	s := &Server{}
	start := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	wsHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case start <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusOK)
	})
	handler := s.tenantWSAdmissionDynamic(wsHandler)

	hardLimit := 1
	s.tenantConnCounter = tenant.NewTenantConnCounter()
	s.tenantQuotaReader = &testQuotaReader{quotas: &tenant.TenantQuotas{RealtimeConnectionsHard: &hardLimit}}

	firstReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-dynamic-ws"))
	firstRec := httptest.NewRecorder()
	go func() {
		handler.ServeHTTP(firstRec, firstReq)
		close(done)
	}()

	<-start
	secondReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-dynamic-ws"))
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	testutil.Equal(t, http.StatusTooManyRequests, secondRec.Code)

	close(release)
	<-done
}

func TestEnforceTenantRequestRate_EmitsQuotaViolationAuditWithConsistentUnits(t *testing.T) {
	t.Parallel()

	hardLimit := 1
	limiter := tenant.NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() {
		limiter.Stop()
	})
	reader := &testQuotaReader{quotas: &tenant.TenantQuotas{RequestRateRPSHard: &hardLimit}}
	auditCapture := &quotaAuditCapture{}
	emitter := tenant.NewAuditEmitterWithInserter(auditCapture, nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-audit-rate"))
		enforceTenantRequestRate(w, req, next, limiter, reader, nil, emitter, nil)
		testutil.Equal(t, http.StatusOK, w.Code)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-audit-rate"))
	claims := &auth.Claims{}
	claims.Subject = "3f3c6f64-0e8d-4f37-b48a-bf55e6dc7105"
	req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	enforceTenantRequestRate(w, req, next, limiter, reader, nil, emitter, nil)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)

	testutil.Equal(t, "tenant-audit-rate", auditCapture.tenantID)
	testutil.Equal(t, tenant.AuditActionQuotaViolation, auditCapture.action)
	testutil.Equal(t, tenant.AuditResultDenied, auditCapture.result)
	if auditCapture.actorID == nil {
		t.Fatal("expected actor id to be captured")
	}
	testutil.Equal(t, "3f3c6f64-0e8d-4f37-b48a-bf55e6dc7105", *auditCapture.actorID)
	if auditCapture.ipAddr == nil {
		t.Fatal("expected ip address to be captured")
	}
	testutil.Equal(t, "198.51.100.20", *auditCapture.ipAddr)

	var meta map[string]any
	testutil.NoError(t, json.Unmarshal(auditCapture.metadata, &meta))
	testutil.Equal(t, "request_rate", meta["resource"])
	current, ok := meta["current"].(float64)
	testutil.True(t, ok, "expected numeric current value in audit metadata")
	testutil.Equal(t, float64(60), current)
	limit, ok := meta["limit"].(float64)
	testutil.True(t, ok, "expected numeric limit value in audit metadata")
	testutil.Equal(t, float64(60), limit)
}

func TestEnforceTenantRequestRate_EmitsTenantQuotaMetrics(t *testing.T) {
	t.Parallel()

	hardLimit := 1
	limiter := tenant.NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() {
		limiter.Stop()
	})

	reader := &testQuotaReader{quotas: &tenant.TenantQuotas{RequestRateRPSHard: &hardLimit}}
	metricsCapture := &quotaMetricsCapture{}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-metrics-rate"))
		enforceTenantRequestRate(w, req, next, limiter, reader, nil, nil, metricsCapture)
		testutil.Equal(t, http.StatusOK, w.Code)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-metrics-rate"))
	enforceTenantRequestRate(w, req, next, limiter, reader, nil, nil, metricsCapture)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)

	testutil.True(t, atomic.LoadInt32(&metricsCapture.utilizationCalls) > 0, "expected quota utilization metric emissions")
	testutil.Equal(t, int32(1), atomic.LoadInt32(&metricsCapture.violationCalls))
	testutil.Equal(t, "tenant-metrics-rate", metricsCapture.lastTenantID)
	testutil.Equal(t, string(tenant.ResourceTypeRequestRate), metricsCapture.lastResource)
	testutil.Equal(t, int64(60), metricsCapture.lastCurrent)
	testutil.Equal(t, int64(60), metricsCapture.lastLimit)
}

func TestTenantWSAdmission_EmitsTenantQuotaMetricsOnViolation(t *testing.T) {
	t.Parallel()

	hardLimit := 1
	counter := tenant.NewTenantConnCounter()
	reader := &testQuotaReader{quotas: &tenant.TenantQuotas{RealtimeConnectionsHard: &hardLimit}}
	metricsCapture := &quotaMetricsCapture{}

	start := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	wsHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		select {
		case start <- struct{}{}:
		default:
		}
		<-release
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-metrics-ws"))
	firstRec := httptest.NewRecorder()
	go func() {
		enforceTenantWSAdmission(firstRec, firstReq, wsHandler, counter, reader, nil, nil, metricsCapture)
		close(done)
	}()

	<-start
	secondReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-metrics-ws"))
	secondRec := httptest.NewRecorder()
	enforceTenantWSAdmission(secondRec, secondReq, wsHandler, counter, reader, nil, nil, metricsCapture)
	testutil.Equal(t, http.StatusTooManyRequests, secondRec.Code)

	close(release)
	<-done

	testutil.True(t, atomic.LoadInt32(&metricsCapture.utilizationCalls) > 0, "expected realtime utilization metrics")
	testutil.Equal(t, int32(1), atomic.LoadInt32(&metricsCapture.violationCalls))
	testutil.Equal(t, "tenant-metrics-ws", metricsCapture.lastTenantID)
	testutil.Equal(t, string(tenant.ResourceTypeRealtimeConns), metricsCapture.lastResource)
	testutil.Equal(t, int64(1), metricsCapture.lastLimit)
}

func TestTenantWSAdmission_EmitsQuotaViolationAudit(t *testing.T) {
	t.Parallel()

	hardLimit := 1
	counter := tenant.NewTenantConnCounter()
	reader := &testQuotaReader{quotas: &tenant.TenantQuotas{RealtimeConnectionsHard: &hardLimit}}
	auditCapture := &quotaAuditCapture{}
	emitter := tenant.NewAuditEmitterWithInserter(auditCapture, nil)

	start := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	wsHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		select {
		case start <- struct{}{}:
		default:
		}
		<-release
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-ws-audit"))
	firstRec := httptest.NewRecorder()
	go func() {
		enforceTenantWSAdmission(firstRec, firstReq, wsHandler, counter, reader, nil, emitter, nil)
		close(done)
	}()

	<-start
	secondReq := httptest.NewRequest(http.MethodGet, "/api/realtime/ws", nil).WithContext(tenant.ContextWithTenantID(context.Background(), "tenant-ws-audit"))
	wsClaims := &auth.Claims{}
	wsClaims.Subject = "321f7a2e-fca3-4df2-bf5e-fb6e9c4f0b61"
	secondReq = secondReq.WithContext(auth.ContextWithClaims(secondReq.Context(), wsClaims))
	secondReq.Header.Set("X-Forwarded-For", "198.51.100.23")
	secondRec := httptest.NewRecorder()
	enforceTenantWSAdmission(secondRec, secondReq, wsHandler, counter, reader, nil, emitter, nil)
	testutil.Equal(t, http.StatusTooManyRequests, secondRec.Code)

	close(release)
	<-done

	testutil.Equal(t, "tenant-ws-audit", auditCapture.tenantID)
	testutil.Equal(t, tenant.AuditActionQuotaViolation, auditCapture.action)
	testutil.Equal(t, tenant.AuditResultDenied, auditCapture.result)

	var meta map[string]any
	testutil.NoError(t, json.Unmarshal(auditCapture.metadata, &meta))
	testutil.Equal(t, "realtime_connections", meta["resource"])
	current, ok := meta["current"].(float64)
	testutil.True(t, ok, "expected numeric current in audit metadata")
	testutil.Equal(t, float64(1), current)
	limit, ok := meta["limit"].(float64)
	testutil.True(t, ok, "expected numeric limit in audit metadata")
	testutil.Equal(t, float64(1), limit)
}

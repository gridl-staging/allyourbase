package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func scrapeMetrics(t *testing.T, m *HTTPMetrics) string {
	t.Helper()
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from metrics handler, got %d", w.Code)
	}
	return w.Body.String()
}

func TestInfraMetrics_DBPoolGauges(t *testing.T) {
	statFn := func() (total, idle, inUse, max int32) {
		return 10, 3, 7, 20
	}

	infra := NewInfraMetrics(statFn)
	m, err := NewHTTPMetrics(infra.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	body := scrapeMetrics(t, m)

	for _, want := range []string{
		"ayb_db_pool_total 10",
		"ayb_db_pool_idle 3",
		"ayb_db_pool_in_use 7",
		"ayb_db_pool_max 20",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in metrics output:\n%s", want, body)
		}
	}
}

func TestInfraMetrics_DBPoolGauges_NilStatFn(t *testing.T) {
	infra := NewInfraMetrics(nil)
	m, err := NewHTTPMetrics(infra.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	// Should not panic or error with nil stat function; gauges simply report 0.
	body := scrapeMetrics(t, m)
	if !strings.Contains(body, "ayb_db_pool_total 0") {
		t.Errorf("expected ayb_db_pool_total 0 with nil stat fn:\n%s", body)
	}
}

func TestInfraMetrics_AuthCounters(t *testing.T) {
	infra := NewInfraMetrics(nil)
	m, err := NewHTTPMetrics(infra.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	ctx := context.Background()
	infra.RecordAuthSignup(ctx)
	infra.RecordAuthSignup(ctx)
	infra.RecordAuthLogin(ctx)
	infra.RecordAuthLogin(ctx)
	infra.RecordAuthLogin(ctx)

	body := scrapeMetrics(t, m)

	if !strings.Contains(body, "ayb_auth_signups_total 2") {
		t.Errorf("expected ayb_auth_signups_total 2:\n%s", body)
	}
	if !strings.Contains(body, "ayb_auth_logins_total 3") {
		t.Errorf("expected ayb_auth_logins_total 3:\n%s", body)
	}
}

func TestInfraMetrics_EdgeFuncCounter(t *testing.T) {
	infra := NewInfraMetrics(nil)
	m, err := NewHTTPMetrics(infra.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	ctx := context.Background()
	infra.RecordEdgeFuncInvocation(ctx, "my-func", "ok")
	infra.RecordEdgeFuncInvocation(ctx, "my-func", "ok")
	infra.RecordEdgeFuncInvocation(ctx, "my-func", "error")
	infra.RecordEdgeFuncInvocation(ctx, "other-func", "ok")

	body := scrapeMetrics(t, m)

	testutilEdgeFuncCounterValue(t, body, "my-func", "ok", 2)
	testutilEdgeFuncCounterValue(t, body, "my-func", "error", 1)
	testutilEdgeFuncCounterValue(t, body, "other-func", "ok", 1)
}

func testutilEdgeFuncCounterValue(t *testing.T, body, function, status string, want int) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "ayb_edge_function_invocations_total{") {
			continue
		}
		if !strings.Contains(line, `function="`+function+`"`) || !strings.Contains(line, `status="`+status+`"`) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			t.Fatalf("unexpected metric line format: %q", line)
		}
		got, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("parsing metric value %q from %q: %v", parts[1], line, err)
		}
		if got != want {
			t.Fatalf("expected edge metric value %d for function=%q status=%q, got %d", want, function, status, got)
		}
		return
	}
	t.Fatalf("missing edge metric for function=%q status=%q in metrics output:\n%s", function, status, body)
}

func TestInfraMetrics_StorageBytesGauge(t *testing.T) {
	infra := NewInfraMetrics(nil)
	m, err := NewHTTPMetrics(infra.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	// Default should be 0.
	body := scrapeMetrics(t, m)
	if !strings.Contains(body, "ayb_storage_bytes_total 0") {
		t.Errorf("expected ayb_storage_bytes_total 0 initially:\n%s", body)
	}

	// Update and check.
	infra.SetStorageBytes(1024 * 1024 * 500) // 500MB
	body = scrapeMetrics(t, m)
	if !strings.Contains(body, "ayb_storage_bytes_total 5.24288e+08") && !strings.Contains(body, "ayb_storage_bytes_total 524288000") {
		t.Errorf("expected ayb_storage_bytes_total 524288000:\n%s", body)
	}
}

func TestRealtimeMetricsAggregator_ExposesExpectedMetrics(t *testing.T) {
	agg := NewRealtimeMetricsAggregator(
		func() int { return 2 },
		func() int { return 3 },
		func() int { return 4 },
		func() int { return 5 },
		func() uint64 { return 7 },
		func() uint64 { return 11 },
	)
	m, err := NewHTTPMetrics(agg.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	body := scrapeMetrics(t, m)
	for _, want := range []string{
		`ayb_realtime_connections_active{transport="sse"} 2`,
		`ayb_realtime_connections_active{transport="ws"} 3`,
		`ayb_realtime_channels_active{type="broadcast"} 4`,
		`ayb_realtime_channels_active{type="presence"} 5`,
		"ayb_realtime_broadcast_messages_total 7",
		"ayb_realtime_presence_syncs_total 11",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in metrics output:\n%s", want, body)
		}
	}

	for _, oldName := range []string{
		"ayb_realtime_ws_connections_active",
		"ayb_realtime_presence_channels_active",
	} {
		if strings.Contains(body, oldName) {
			t.Fatalf("did not expect legacy metric name %q in output:\n%s", oldName, body)
		}
	}
}

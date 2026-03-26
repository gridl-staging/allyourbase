package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/billing"
)

func TestHandleAdminUsageTrendsReturnsServiceUnavailableWithoutAggregate(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleAdminUsageTrends(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/trends?metric=api_requests", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "usage aggregation service not configured" {
		t.Fatalf("message = %q, want %q", got, "usage aggregation service not configured")
	}
}

func TestHandleAdminUsageTrendsValidationErrors(t *testing.T) {
	t.Parallel()

	h := handleAdminUsageTrends(&fakeUsageAggregateService{})
	cases := []struct {
		name        string
		path        string
		wantMessage string
	}{
		{name: "missing metric", path: "/admin/usage/trends", wantMessage: "metric is required"},
		{name: "invalid metric", path: "/admin/usage/trends?metric=bad&granularity=day", wantMessage: "invalid metric"},
		{name: "invalid granularity", path: "/admin/usage/trends?metric=bandwidth_bytes&granularity=hour", wantMessage: "invalid granularity"},
		{name: "invalid period", path: "/admin/usage/trends?metric=api_requests&period=year", wantMessage: "invalid date range"},
		{name: "invalid date range", path: "/admin/usage/trends?metric=api_requests&from=2026-03-10&to=2026-03-01", wantMessage: "invalid date range"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			if got := usageErrorMessage(t, w); got != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}

func TestHandleAdminUsageTrendsReturnsBackendError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleAdminUsageTrends(&fakeUsageAggregateService{trendErr: errors.New("boom")}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/trends?metric=api_requests", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "failed to load usage trends" {
		t.Fatalf("message = %q, want %q", got, "failed to load usage trends")
	}
}

func TestHandleAdminUsageTrendsReturnsTrendPayload(t *testing.T) {
	t.Parallel()

	agg := &fakeUsageAggregateService{trendRows: []billing.TrendPoint{}}
	w := httptest.NewRecorder()
	handleAdminUsageTrends(agg).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/trends?metric=api_requests&granularity=day", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	got := decodeUsageTestResponse[usageTrendResponse](t, w)
	if got.Metric != "api_requests" || got.Granularity != "day" || len(got.Items) != 0 {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if agg.trendOpts.Metric != "api_requests" || agg.trendOpts.Granularity != "day" {
		t.Fatalf("unexpected trend opts: %+v", agg.trendOpts)
	}
}

package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/billing"
)

func TestHandleAdminUsageBreakdownReturnsServiceUnavailableWithoutAggregate(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleAdminUsageBreakdown(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/breakdown?metric=api_requests", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "usage aggregation service not configured" {
		t.Fatalf("message = %q, want %q", got, "usage aggregation service not configured")
	}
}

func TestHandleAdminUsageBreakdownValidationErrors(t *testing.T) {
	t.Parallel()

	h := handleAdminUsageBreakdown(&fakeUsageAggregateService{})
	cases := []struct {
		name        string
		path        string
		wantMessage string
	}{
		{name: "missing metric", path: "/admin/usage/breakdown?group_by=tenant", wantMessage: "metric is required"},
		{name: "invalid group by", path: "/admin/usage/breakdown?metric=api_requests&group_by=bad", wantMessage: "invalid group_by"},
		{name: "invalid metric group combination", path: "/admin/usage/breakdown?metric=storage_bytes&group_by=endpoint", wantMessage: "invalid metric/group_by combination"},
		{name: "invalid limit", path: "/admin/usage/breakdown?metric=api_requests&limit=101", wantMessage: "invalid limit"},
		{name: "invalid period", path: "/admin/usage/breakdown?metric=api_requests&period=year", wantMessage: "invalid date range"},
		{name: "invalid date range", path: "/admin/usage/breakdown?metric=api_requests&from=2026-03-10&to=2026-03-01", wantMessage: "invalid date range"},
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

func TestHandleAdminUsageBreakdownReturnsBackendError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleAdminUsageBreakdown(&fakeUsageAggregateService{breakdownErr: errors.New("boom")}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/breakdown?metric=api_requests", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "failed to load usage breakdown" {
		t.Fatalf("message = %q, want %q", got, "failed to load usage breakdown")
	}
}

func TestHandleAdminUsageBreakdownReturnsBreakdownPayload(t *testing.T) {
	t.Parallel()

	agg := &fakeUsageAggregateService{breakdownRows: []billing.BreakdownEntry{{Key: "/api/health", Value: 17}}}
	w := httptest.NewRecorder()
	handleAdminUsageBreakdown(agg).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/breakdown?metric=api_requests&group_by=endpoint&limit=10", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	got := decodeUsageTestResponse[usageBreakdownResponse](t, w)
	if got.Metric != "api_requests" || got.GroupBy != "endpoint" || len(got.Items) != 1 {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if agg.breakdownOpts.Metric != "api_requests" || agg.breakdownOpts.GroupBy != "endpoint" || agg.breakdownOpts.Limit != 10 {
		t.Fatalf("unexpected breakdown opts: %+v", agg.breakdownOpts)
	}
}

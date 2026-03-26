package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/billing"
)

func TestHandleAdminUsageListReturnsServiceUnavailableWithoutAggregate(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleAdminUsageList(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "usage aggregation service not configured" {
		t.Fatalf("message = %q, want %q", got, "usage aggregation service not configured")
	}
}

func TestHandleAdminUsageListValidationErrors(t *testing.T) {
	t.Parallel()

	h := handleAdminUsageList(&fakeUsageAggregateService{})
	cases := []struct {
		name        string
		path        string
		wantMessage string
	}{
		{name: "invalid sort", path: "/admin/usage?sort=bad:asc", wantMessage: "invalid sort"},
		{name: "invalid limit", path: "/admin/usage?limit=-1", wantMessage: "invalid limit"},
		{name: "invalid offset", path: "/admin/usage?offset=1000001", wantMessage: "invalid offset"},
		{name: "invalid period", path: "/admin/usage?period=year", wantMessage: "invalid period or date range"},
		{name: "invalid date range", path: "/admin/usage?from=2026-03-10&to=2026-03-01", wantMessage: "invalid period or date range"},
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

func TestHandleAdminUsageListReturnsBackendError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleAdminUsageList(&fakeUsageAggregateService{listErr: errors.New("query failed")}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "failed to load usage summaries" {
		t.Fatalf("message = %q, want %q", got, "failed to load usage summaries")
	}
}

func TestHandleAdminUsageListReturnsPaginatedResponse(t *testing.T) {
	t.Parallel()

	agg := &fakeUsageAggregateService{
		listRows:  []billing.TenantUsageSummaryRow{{TenantID: "00000000-0000-0000-0000-000000000111", TenantName: "Acme", RequestCount: 123, StorageBytesUsed: 456}},
		listTotal: 1,
	}

	w := httptest.NewRecorder()
	handleAdminUsageList(agg).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage?period=week&sort=tenant_name:asc&limit=25&offset=5", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	got := decodeUsageTestResponse[usageListResponse](t, w)
	if got.Total != 1 || got.Limit != 25 || got.Offset != 5 || len(got.Items) != 1 {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if agg.listOpts.Period != "week" || agg.listOpts.Sort.Column != "tenant_name" || agg.listOpts.Sort.Direction != "ASC" {
		t.Fatalf("unexpected list opts: %+v", agg.listOpts)
	}
	if agg.listOpts.Limit != 25 || agg.listOpts.Offset != 5 {
		t.Fatalf("unexpected paging opts: limit=%d offset=%d", agg.listOpts.Limit, agg.listOpts.Offset)
	}
}

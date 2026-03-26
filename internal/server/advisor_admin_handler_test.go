package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminAdvisorEndpointsRequireAdminAuth(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")

	for _, path := range []string{
		"/api/admin/advisors/security",
		"/api/admin/advisors/performance",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestAdminAdvisorSecurityEndpointReturnsReportShape(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/advisors/security", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var report struct {
		EvaluatedAt string `json:"evaluatedAt"`
		Stale       bool   `json:"stale"`
		Findings    []any  `json:"findings"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
	testutil.True(t, report.EvaluatedAt != "", "evaluatedAt should be set")
	testutil.Equal(t, false, report.Stale)
	testutil.Equal(t, 0, len(report.Findings))
}

func TestAdminAdvisorPerformanceEndpointReturnsReportShape(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/advisors/performance?range=24h", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var report struct {
		GeneratedAt string `json:"generatedAt"`
		Stale       bool   `json:"stale"`
		Range       string `json:"range"`
		Queries     []any  `json:"queries"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
	testutil.True(t, report.GeneratedAt != "", "generatedAt should be set")
	testutil.Equal(t, false, report.Stale)
	testutil.Equal(t, "24h", report.Range)
	testutil.Equal(t, 0, len(report.Queries))
}

func TestAdminAdvisorPerformanceRangeNormalization(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	cases := []struct {
		query    string
		expected string
	}{
		{"range=15m", "15m"},
		{"range=1h", "1h"},
		{"range=6h", "6h"},
		{"range=24h", "24h"},
		{"range=7d", "7d"},
		{"range=invalid", "1h"},
		{"range=", "1h"},
		{"", "1h"},
	}

	for _, tc := range cases {
		path := "/api/admin/advisors/performance"
		if tc.query != "" {
			path += "?" + tc.query
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		srv.Router().ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var report struct {
			Range string `json:"range"`
		}
		testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
		testutil.Equal(t, tc.expected, report.Range)
	}
}

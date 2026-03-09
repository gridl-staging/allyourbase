package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestHandleAdminSQLNoPool(t *testing.T) {
	t.Parallel()
	handler := handleAdminSQL(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sql", strings.NewReader(`{"query":"SELECT 1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.Contains(t, w.Body.String(), "database not available")
}

func TestHandleAdminSQLEmptyQuery(t *testing.T) {
	t.Parallel()
	handler := handleAdminSQL(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sql", strings.NewReader(`{"query":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "query is required")
}

func TestHandleAdminSQLInvalidJSON(t *testing.T) {
	t.Parallel()
	handler := handleAdminSQL(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sql", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleAdminSQLWhitespaceOnlyQuery(t *testing.T) {
	t.Parallel()
	handler := handleAdminSQL(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sql", strings.NewReader(`{"query":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "query is required")
}

// sqlResponse JSON round-trip tests removed — they tested Go's json.Marshal
// against struct literals with no handler code exercised.

func TestToJSONSafe(t *testing.T) {
	// nil
	t.Parallel()

	testutil.Nil(t, toJSONSafe(nil))

	// time.Time → RFC3339
	ts := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	result := toJSONSafe(ts)
	testutil.Equal(t, "2026-02-09T12:00:00Z", result)

	// []byte JSON → parsed
	jsonBytes := []byte(`{"key":"value"}`)
	parsed := toJSONSafe(jsonBytes)
	m, ok := parsed.(map[string]any)
	testutil.True(t, ok, "JSON bytes should parse to map")
	testutil.Equal(t, "value", m["key"])

	// []byte non-JSON → string
	plainBytes := []byte("hello world")
	testutil.Equal(t, "hello world", toJSONSafe(plainBytes))

	// [16]byte UUID → formatted UUID string
	uuid := [16]byte{0xd3, 0x3c, 0xb9, 0x43, 0x5d, 0x6b, 0x48, 0x2c, 0xb8, 0xb8, 0xc3, 0x07, 0xbb, 0x28, 0xff, 0x81}
	testutil.Equal(t, "d33cb943-5d6b-482c-b8b8-c307bb28ff81", toJSONSafe(uuid))

	// string passthrough
	testutil.Equal(t, "test", toJSONSafe("test"))

	// int passthrough
	testutil.Equal(t, 42, toJSONSafe(42))

	// bool passthrough
	testutil.Equal(t, true, toJSONSafe(true))
}

func TestIsDDL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		query    string
		expected bool
	}{
		{"CREATE TABLE foo (id int)", true},
		{"create table foo (id int)", true},
		{"ALTER TABLE foo ADD col int", true},
		{"DROP TABLE foo", true},
		{"TRUNCATE foo", true},
		{"GRANT ALL ON foo TO bar", true},
		{"REVOKE ALL ON foo FROM bar", true},
		{"COMMENT ON TABLE foo IS 'desc'", true},
		{"  CREATE TABLE foo (id int)", true},
		{"\n\tDROP TABLE foo", true},
		{"SELECT 1", false},
		{"INSERT INTO foo VALUES (1)", false},
		{"UPDATE foo SET x = 1", false},
		{"DELETE FROM foo", false},
		{"", false},
		{"   ", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.expected, isDDL(tt.query))
		})
	}
}

// TestQueryTimeout removed — tested the constant's value only, not that the
// timeout is actually applied during query execution. The context.WithTimeout
// behavior requires a real DB and is covered by integration tests.

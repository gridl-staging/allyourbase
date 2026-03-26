package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

type adminDrainInfo struct {
	ID    string             `json:"id"`
	Name  string             `json:"name"`
	Stats logging.DrainStats `json:"stats"`
}

func listDrains(t *testing.T, srv *server.Server, token string) []adminDrainInfo {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/logging/drains", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var got []adminDrainInfo
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return got
}

func TestAdminDrainsEndpointCreatesListsAndDeletes(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	got := listDrains(t, srv, token)
	testutil.Equal(t, 0, len(got))

	createBody := `{"id":"api-http","type":"http","url":"https://logs.example.com/ingest"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/logging/drains", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusCreated, w.Code)

	var created adminDrainInfo
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	testutil.Equal(t, "api-http", created.ID)
	testutil.Equal(t, "api-http", created.Name)

	got = listDrains(t, srv, token)
	testutil.Equal(t, 1, len(got))
	testutil.Equal(t, "api-http", got[0].ID)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/logging/drains/api-http", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNoContent, w.Code)

	got = listDrains(t, srv, token)
	testutil.Equal(t, 0, len(got))
}

func TestAdminDrainsCreateRejectsInvalidType(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/logging/drains", strings.NewReader(`{"type":"invalid","url":"https://logs.example.com/ingest"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "unsupported")
}

func TestAdminDrainsCreateRequiresURL(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/logging/drains", strings.NewReader(`{"type":"http"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "url is required")
}

func TestAdminDrainsCreateRejectsNegativeBatchAndFlushViaJSONFields(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{
			name:        "batch size",
			body:        `{"id":"api-http","type":"http","url":"https://logs.example.com/ingest","batch_size":-1}`,
			wantMessage: "batch_size must be non-negative",
		},
		{
			name:        "flush interval",
			body:        `{"id":"api-http","type":"http","url":"https://logs.example.com/ingest","flush_interval_seconds":-1}`,
			wantMessage: "flush_interval_seconds must be non-negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/admin/logging/drains", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			srv.Router().ServeHTTP(w, req)

			testutil.Equal(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, w.Body.String(), tc.wantMessage)
		})
	}
}

func TestAdminDrainsCreateDisabledDoesNotStart(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Logging.Drains = []config.LogDrainConfig{{
		ID:      "pre-disabled",
		Type:    "http",
		URL:     "https://logs.example.com/ingest",
		Enabled: boolPtr(false),
	}}
	srv := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, nil)

	token := adminLogin(t, srv)
	got := listDrains(t, srv, token)
	testutil.Equal(t, 0, len(got))
}

func TestAdminDrainsCreateRuntimeEnablesFanout(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Logging.RequestLogEnabled = false
	srv := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, nil)
	token := adminLogin(t, srv)

	createBody := `{"id":"runtime-http","type":"http","url":"http://127.0.0.1:1/ingest"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/logging/drains", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusCreated, w.Code)

	baseline := listDrains(t, srv, token)
	testutil.Equal(t, 1, len(baseline))
	baseSent := baseline[0].Stats.Sent
	baseFailed := baseline[0].Stats.Failed

	for range 4 {
		healthW := httptest.NewRecorder()
		healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
		srv.Router().ServeHTTP(healthW, healthReq)
		testutil.Equal(t, http.StatusOK, healthW.Code)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := listDrains(t, srv, token)
		if len(got) == 1 && (got[0].Stats.Failed > baseFailed || got[0].Stats.Sent > baseSent) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := listDrains(t, srv, token)
	testutil.Equal(t, 1, len(got))
	testutil.True(t, got[0].Stats.Failed > baseFailed || got[0].Stats.Sent > baseSent, "runtime-created drain should receive fan-out entries from post-create requests")
}

func boolPtr(v bool) *bool {
	return &v
}

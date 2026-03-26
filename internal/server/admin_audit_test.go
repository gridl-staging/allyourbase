//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminAuditEndpointFilteringAndPagination(t *testing.T) {
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `TRUNCATE TABLE _ayb_audit_log`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx, `
		INSERT INTO _ayb_audit_log (id, timestamp, user_id, table_name, operation, record_id, old_values, new_values, ip_address) VALUES
		('10000000-0000-0000-0000-000000000001', '2026-02-01T10:00:00Z', '11111111-1111-1111-1111-111111111111', 'orders', 'DELETE', '{"id":"1"}', '{"id":"1","status":"open"}', NULL, '198.51.100.10'),
		('10000000-0000-0000-0000-000000000002', '2026-02-02T10:00:00Z', '11111111-1111-1111-1111-111111111111', 'orders', 'INSERT', '{"id":"2"}', NULL, '{"id":"2","status":"open"}', '198.51.100.11'),
		('10000000-0000-0000-0000-000000000003', '2026-02-03T10:00:00Z', '22222222-2222-2222-2222-222222222222', 'users', 'DELETE', '{"id":"3"}', '{"id":"3","name":"Alice"}', NULL, '198.51.100.12')`)
	testutil.NoError(t, err)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit?table=orders&user_id=11111111-1111-1111-1111-111111111111&operation=DELETE&from=2026-02-01&to=2026-02-28&limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var filtered struct {
		Items []struct {
			ID        string `json:"id"`
			TableName string `json:"table_name"`
			Operation string `json:"operation"`
			UserID    string `json:"user_id"`
		} `json:"items"`
		Count int `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &filtered))
	testutil.Equal(t, 1, filtered.Count)
	testutil.Equal(t, 1, len(filtered.Items))
	testutil.Equal(t, "10000000-0000-0000-0000-000000000001", filtered.Items[0].ID)
	testutil.Equal(t, "orders", filtered.Items[0].TableName)
	testutil.Equal(t, "DELETE", filtered.Items[0].Operation)
	testutil.Equal(t, "11111111-1111-1111-1111-111111111111", filtered.Items[0].UserID)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/audit?limit=1&offset=1", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w2, req2)
	testutil.Equal(t, http.StatusOK, w2.Code)

	var paged struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Count int `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w2.Body.Bytes(), &paged))
	testutil.Equal(t, 1, paged.Count)
	testutil.Equal(t, 1, len(paged.Items))
	testutil.Equal(t, "10000000-0000-0000-0000-000000000002", paged.Items[0].ID)
}

func TestAdminAuditEndpointRejectsInvalidUserID(t *testing.T) {
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit?user_id=not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminAuditEndpointRejectsInvalidOperation(t *testing.T) {
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit?operation=merge", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid operation filter")
}

//go:build integration

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func stage7SetupServer(t *testing.T) (*Server, context.Context) {
	t.Helper()
	ctx := context.Background()
	pg := newRequestLoggerTestDB(t)
	ensureIntegrationMigrations(t, ctx, pg.Pool)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := New(cfg, logger, ch, pg.Pool, nil, nil)

	tenantSvc := tenant.NewService(pg.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(tenant.NewUsageAccumulator(pg.Pool, logger))
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	rl := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(rl)
	t.Cleanup(rl.Stop)
	return srv, ctx
}

func stage7CreateTenant(t *testing.T, srv *Server, adminToken, slug, isolationMode string) tenant.Tenant {
	t.Helper()
	stage5EnsureUser(t, srv, stageIntegrationOwnerUserID)
	body := fmt.Sprintf(`{"name":"test-%s","slug":"%s","ownerUserId":"%s","isolationMode":"%s","planTier":"free"}`,
		slug, slug, stageIntegrationOwnerUserID, isolationMode)
	w := stage5TenantAdminRequest(t, srv, http.MethodPost, "/api/admin/tenants", adminToken, "", body)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created tenant.Tenant
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	created = stage5ActivateTenant(t, srv, created.ID)
	return created
}

func TestSchemaIsolation_CreateTenantProvisionSchema(t *testing.T) {
	srv, ctx := stage7SetupServer(t)
	adminToken := stage5AdminLogin(t, srv)
	slug := fmt.Sprintf("schema-create-%d", time.Now().UnixNano())

	created := stage7CreateTenant(t, srv, adminToken, slug, "schema")

	var exists bool
	err := srv.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		created.Slug,
	).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "schema should be provisioned for schema-mode tenant")
	testutil.Equal(t, "schema", created.IsolationMode)
}

func TestSchemaIsolation_DataIsolation(t *testing.T) {
	srv, ctx := stage7SetupServer(t)
	adminToken := stage5AdminLogin(t, srv)

	schemaTenant := stage7CreateTenant(t, srv, adminToken, fmt.Sprintf("schema-iso-%d", time.Now().UnixNano()), "schema")
	_ = stage7CreateTenant(t, srv, adminToken, fmt.Sprintf("shared-iso-%d", time.Now().UnixNano()), "shared")

	schemaIdent := pgx.Identifier{schemaTenant.Slug}.Sanitize()
	_, err := srv.pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s.tenant_items (id SERIAL PRIMARY KEY, note TEXT NOT NULL)`, schemaIdent))
	testutil.NoError(t, err)
	_, err = srv.pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s.tenant_items (note) VALUES ('secret')`, schemaIdent))
	testutil.NoError(t, err)

	conn, err := srv.pool.Acquire(ctx)
	testutil.NoError(t, err)
	defer conn.Release()

	_, err = conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, schemaIdent))
	testutil.NoError(t, err)

	var schemaCount int
	err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM tenant_items`).Scan(&schemaCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, schemaCount)

	_, err = conn.Exec(ctx, `SET search_path TO public`)
	testutil.NoError(t, err)

	err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM tenant_items`).Scan(&schemaCount)
	testutil.ErrorContains(t, err, `relation "tenant_items" does not exist`)
}

func TestSchemaIsolation_CrossTenantDataIsolation(t *testing.T) {
	srv, ctx := stage7SetupServer(t)
	adminToken := stage5AdminLogin(t, srv)

	tenantA := stage7CreateTenant(t, srv, adminToken, fmt.Sprintf("cross-a-%d", time.Now().UnixNano()), "schema")
	tenantB := stage7CreateTenant(t, srv, adminToken, fmt.Sprintf("cross-b-%d", time.Now().UnixNano()), "schema")

	schemaA := pgx.Identifier{tenantA.Slug}.Sanitize()
	schemaB := pgx.Identifier{tenantB.Slug}.Sanitize()

	// Create table and insert data in tenant A's schema
	_, err := srv.pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s.secrets (id SERIAL PRIMARY KEY, data TEXT NOT NULL)`, schemaA))
	testutil.NoError(t, err)
	_, err = srv.pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s.secrets (data) VALUES ('tenant-a-secret')`, schemaA))
	testutil.NoError(t, err)

	// Verify tenant B's search_path cannot see tenant A's table
	conn, err := srv.pool.Acquire(ctx)
	testutil.NoError(t, err)
	defer conn.Release()

	_, err = conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, schemaB))
	testutil.NoError(t, err)

	var count int
	err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM secrets`).Scan(&count)
	testutil.ErrorContains(t, err, `relation "secrets" does not exist`)

	// Verify tenant A's search_path CAN see the table
	_, err = conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, schemaA))
	testutil.NoError(t, err)

	err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM secrets`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

func TestSchemaIsolation_SearchPathResetAfterRequest(t *testing.T) {
	srv, ctx := stage7SetupServer(t)
	adminToken := stage5AdminLogin(t, srv)

	schemaTenant := stage7CreateTenant(t, srv, adminToken, fmt.Sprintf("reset-%d", time.Now().UnixNano()), "schema")
	schemaIdent := pgx.Identifier{schemaTenant.Slug}.Sanitize()

	// Seed tenant-scoped data so the request can prove it has tenant search_path.
	_, err := srv.pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s.reset_test (id SERIAL PRIMARY KEY)`, schemaIdent))
	testutil.NoError(t, err)
	_, err = srv.pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s.reset_test DEFAULT VALUES`, schemaIdent))
	testutil.NoError(t, err)

	// Create a 1-connection pool so the post-request acquire necessarily returns
	// the same backend session the middleware used during the request.
	pinnedPool := newSingleConnPool(t, ctx, srv.pool)
	srv.tenantConnAcquire = newTenantConnAcquire(pinnedPool)

	var requestPID int
	handler := srv.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestConn := tenant.RequestConnFromContext(r.Context())
		testutil.True(t, requestConn != nil, "expected request connection in context")

		// Record backend PID to prove same-session in post-request check.
		err := requestConn.QueryRow(r.Context(), `SELECT pg_backend_pid()`).Scan(&requestPID)
		testutil.NoError(t, err)

		var sp string
		err = requestConn.QueryRow(r.Context(), `SHOW search_path`).Scan(&sp)
		testutil.NoError(t, err)
		testutil.Contains(t, sp, schemaTenant.Slug)
		testutil.Contains(t, sp, "public")

		var count int
		err = requestConn.QueryRow(r.Context(), `SELECT COUNT(*) FROM reset_test`).Scan(&count)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, count)

		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/schema-reset-check", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), schemaTenant.ID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusNoContent, rec.Code)
	testutil.True(t, requestPID != 0, "handler must have recorded pg_backend_pid()")

	// Acquire from the same 1-connection pool — must be the same backend session.
	conn, err := pinnedPool.Acquire(ctx)
	testutil.NoError(t, err)
	defer conn.Release()

	var postPID int
	err = conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&postPID)
	testutil.NoError(t, err)
	testutil.Equal(t, requestPID, postPID)

	var searchPath string
	err = conn.QueryRow(ctx, `SHOW search_path`).Scan(&searchPath)
	testutil.NoError(t, err)
	testutil.Contains(t, searchPath, "public")
	testutil.False(t, strings.Contains(searchPath, schemaTenant.Slug), "search_path should not retain tenant schema after request")

	var count int
	err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM reset_test`).Scan(&count)
	testutil.ErrorContains(t, err, `relation "reset_test" does not exist`)
}

// newSingleConnPool creates a pool with MaxConns=1 from the same database as
// the source pool. This forces deterministic connection reuse for session-pinned
// assertions.
func newSingleConnPool(t *testing.T, ctx context.Context, source *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := source.Config().Copy()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	testutil.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestSchemaIsolation_DeleteCleansUpSchema(t *testing.T) {
	srv, ctx := stage7SetupServer(t)
	adminToken := stage5AdminLogin(t, srv)
	slug := fmt.Sprintf("schema-delete-%d", time.Now().UnixNano())
	created := stage7CreateTenant(t, srv, adminToken, slug, "schema")

	var exists bool
	err := srv.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		created.Slug,
	).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "schema should exist before delete")

	w := stage5TenantAdminRequest(t, srv, http.MethodDelete, "/api/admin/tenants/"+created.ID, adminToken, "", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	err = srv.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		created.Slug,
	).Scan(&exists)
	testutil.NoError(t, err)
	testutil.False(t, exists, "schema should be dropped on tenant delete")
}

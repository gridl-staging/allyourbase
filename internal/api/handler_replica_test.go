package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeRequestConn struct {
	beginCalls int
	beginTx    pgx.Tx
}

type fakeRequestRow struct{}

func (fakeRequestRow) Scan(...any) error { return nil }

func (c *fakeRequestConn) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (c *fakeRequestConn) QueryRow(context.Context, string, ...any) pgx.Row        { return fakeRequestRow{} }
func (c *fakeRequestConn) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("SELECT"), nil
}
func (c *fakeRequestConn) Begin(context.Context) (pgx.Tx, error) {
	c.beginCalls++
	if c.beginTx != nil {
		return c.beginTx, nil
	}
	return &fakeRequestTx{}, nil
}

type fakeRequestTx struct {
	execSQLs []string
}

func (tx *fakeRequestTx) Begin(context.Context) (pgx.Tx, error) { return tx, nil }
func (tx *fakeRequestTx) Commit(context.Context) error          { return nil }
func (tx *fakeRequestTx) Rollback(context.Context) error        { return nil }
func (tx *fakeRequestTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (tx *fakeRequestTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (tx *fakeRequestTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (tx *fakeRequestTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (tx *fakeRequestTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.execSQLs = append(tx.execSQLs, sql)
	return pgconn.NewCommandTag("SET"), nil
}
func (tx *fakeRequestTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (tx *fakeRequestTx) QueryRow(context.Context, string, ...any) pgx.Row        { return fakeRequestRow{} }
func (tx *fakeRequestTx) Conn() *pgx.Conn                                         { return nil }

func TestWithRLSWithoutPoolRouterReturnsPrimaryPool(t *testing.T) {
	t.Parallel()

	var primary pgxpool.Pool
	h := &Handler{
		pool:   &primary,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest("GET", "/collections/users", nil)
	q, done, err := h.withRLS(req)
	testutil.NoError(t, err)

	gotPool, ok := q.(*pgxpool.Pool)
	testutil.True(t, ok, "expected Querier to be *pgxpool.Pool")
	testutil.Equal(t, &primary, gotPool)
	testutil.NoError(t, done(nil))
}

func TestWithRLSReadOnlyUsesReplicaPool(t *testing.T) {
	t.Parallel()

	var primary, readReplica pgxpool.Pool
	router := replica.NewPoolRouter(&primary, []replica.ReplicaPool{
		{
			Pool: &readReplica,
			Config: replica.ReplicaConfig{
				URL:         "postgresql://replica-1/db",
				Weight:      1,
				MaxLagBytes: 1,
			},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	h := &Handler{
		pool:       &primary,
		poolRouter: router,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx := replica.WithRoutingState(context.Background(), &replica.RoutingState{})
	req := httptest.NewRequest("GET", "/collections/users", nil).WithContext(ctx)

	q, done, err := h.withRLS(req)
	testutil.NoError(t, err)

	gotPool, ok := q.(*pgxpool.Pool)
	testutil.True(t, ok, "expected Querier to be *pgxpool.Pool")
	testutil.Equal(t, &readReplica, gotPool)
	testutil.NoError(t, done(nil))
}

func TestWithRLSForcePrimaryReturnsPrimaryPool(t *testing.T) {
	t.Parallel()

	var primary, readReplica pgxpool.Pool
	router := replica.NewPoolRouter(&primary, []replica.ReplicaPool{
		{
			Pool: &readReplica,
			Config: replica.ReplicaConfig{
				URL:         "postgresql://replica-1/db",
				Weight:      1,
				MaxLagBytes: 1,
			},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	h := &Handler{
		pool:       &primary,
		poolRouter: router,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx := replica.WithRoutingState(context.Background(), &replica.RoutingState{ForcePrimary: true})
	req := httptest.NewRequest("GET", "/collections/users", nil).WithContext(ctx)

	q, done, err := h.withRLS(req)
	testutil.NoError(t, err)

	gotPool, ok := q.(*pgxpool.Pool)
	testutil.True(t, ok, "expected Querier to be *pgxpool.Pool")
	testutil.Equal(t, &primary, gotPool)
	testutil.NoError(t, done(nil))
}

func TestWithRLSPostUsesPrimaryPoolWithRoutingMiddlewareContext(t *testing.T) {
	t.Parallel()

	var primary, readReplica pgxpool.Pool
	router := replica.NewPoolRouter(&primary, []replica.ReplicaPool{
		{
			Pool: &readReplica,
			Config: replica.ReplicaConfig{
				URL:         "postgresql://replica-1/db",
				Weight:      1,
				MaxLagBytes: 1,
			},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	h := &Handler{
		pool:       &primary,
		poolRouter: router,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	var routedReq *http.Request
	mw := replica.ReplicaRoutingMiddleware(router)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		routedReq = r
	}))
	req := httptest.NewRequest(http.MethodPost, "/collections/users", nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)
	testutil.NotNil(t, routedReq)

	q, done, err := h.withRLS(routedReq)
	testutil.NoError(t, err)

	gotPool, ok := q.(*pgxpool.Pool)
	testutil.True(t, ok, "expected Querier to be *pgxpool.Pool")
	testutil.Equal(t, &primary, gotPool)
	testutil.NoError(t, done(nil))
}

func TestWithRLSUsesRequestConnFromContextWithoutClaims(t *testing.T) {
	t.Parallel()

	requestConn := &fakeRequestConn{}
	h := &Handler{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/collections/users", nil)
	req = req.WithContext(tenant.ContextWithRequestConn(req.Context(), requestConn))

	q, done, err := h.withRLS(req)
	testutil.NoError(t, err)
	testutil.True(t, q == requestConn, "expected request-scoped connection")
	testutil.NoError(t, done(nil))
}

func TestWithRLSBeginsRequestConnTransactionForClaims(t *testing.T) {
	t.Parallel()

	requestTx := &fakeRequestTx{}
	requestConn := &fakeRequestConn{beginTx: requestTx}
	h := &Handler{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx := tenant.ContextWithRequestConn(context.Background(), requestConn)
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
		Email:            "user@example.com",
		TenantID:         "tenant-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/collections/users", nil).WithContext(ctx)

	q, done, err := h.withRLS(req)
	testutil.NoError(t, err)
	testutil.True(t, q == requestTx, "expected request-scoped transaction")
	testutil.Equal(t, 1, requestConn.beginCalls)
	testutil.SliceLen(t, requestTx.execSQLs, 4)
	testutil.Contains(t, requestTx.execSQLs[0], "SET LOCAL ROLE")
	testutil.Contains(t, requestTx.execSQLs[3], "SET LOCAL ayb.tenant_id")
	testutil.NoError(t, done(nil))
}

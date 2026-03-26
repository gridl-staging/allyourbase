package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeReplicaTopologyStore struct {
	listRecords      []replica.TopologyNodeRecord
	listErr          error
	isEmpty          bool
	isEmptyErr       error
	bootstrapErr     error
	bootstrapCalls   int
	bootstrapRecords []replica.TopologyNodeRecord
}

func (s *fakeReplicaTopologyStore) List(_ context.Context) ([]replica.TopologyNodeRecord, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]replica.TopologyNodeRecord, len(s.listRecords))
	copy(out, s.listRecords)
	return out, nil
}

func (s *fakeReplicaTopologyStore) Get(_ context.Context, name string) (replica.TopologyNodeRecord, error) {
	for _, record := range s.listRecords {
		if record.Name == name {
			return record, nil
		}
	}
	return replica.TopologyNodeRecord{}, errors.New("not found")
}

func (s *fakeReplicaTopologyStore) IsEmpty(_ context.Context) (bool, error) {
	if s.isEmptyErr != nil {
		return false, s.isEmptyErr
	}
	return s.isEmpty, nil
}

func (s *fakeReplicaTopologyStore) Bootstrap(_ context.Context, records []replica.TopologyNodeRecord) error {
	if s.bootstrapErr != nil {
		return s.bootstrapErr
	}
	s.bootstrapCalls++
	s.bootstrapRecords = make([]replica.TopologyNodeRecord, len(records))
	copy(s.bootstrapRecords, records)
	if s.isEmpty {
		s.listRecords = append([]replica.TopologyNodeRecord(nil), records...)
		s.isEmpty = false
	}
	return nil
}

func (s *fakeReplicaTopologyStore) UpdateState(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *fakeReplicaTopologyStore) Add(_ context.Context, _ replica.TopologyNodeRecord) error {
	return nil
}

func (s *fakeReplicaTopologyStore) PromoteNode(_ context.Context, _ string) error {
	return nil
}

func testReplicaLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newReplicaTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgresql://postgres:postgres@127.0.0.1:1/postgres?sslmode=disable&connect_timeout=1")
	testutil.NoError(t, err)
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	testutil.NoError(t, err)
	return pool
}

func TestNewServerNoReplicasKeepsReplicaRoutingDisabled(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)

	s := newServer(cfg, logger, ch, nil, nil, nil, nil)
	if s.poolRouter != nil {
		t.Fatal("expected poolRouter to be nil when no replicas configured")
	}
	if s.healthChecker != nil {
		t.Fatal("expected healthChecker to be nil when no replicas configured")
	}
}

func TestRegisterServerAPIRoutesMountsSchemaAndAdminStatus(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)

	r := chi.NewRouter()
	s.registerServerAPIRoutes(r, func(next http.Handler) http.Handler { return next })

	schemaReq := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	schemaRes := httptest.NewRecorder()
	r.ServeHTTP(schemaRes, schemaReq)
	testutil.Equal(t, http.StatusServiceUnavailable, schemaRes.Code)

	adminReq := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	adminRes := httptest.NewRecorder()
	r.ServeHTTP(adminRes, adminReq)
	testutil.Equal(t, http.StatusOK, adminRes.Code)
}

func TestBuildReplicaRoutingSkipsFailedReplicas(t *testing.T) {
	// Mutates package-level replica dial/ping hooks for deterministic wiring tests.
	// Keep non-parallel to avoid races with other tests touching the same hooks.

	logger := testReplicaLogger()
	primary := &pgxpool.Pool{}
	store := &fakeReplicaTopologyStore{
		listRecords: []replica.TopologyNodeRecord{
			{Name: "primary", Role: "primary", State: "active"},
			{Name: "good", Host: "replica-good.local", Port: 5432, Database: "app", SSLMode: "disable", Role: "replica", State: "active", Weight: 1, MaxLagBytes: 1},
			{Name: "bad-connect", Host: "bad-connect.local", Port: 5432, Database: "app", SSLMode: "disable", Role: "replica", State: "active", Weight: 1, MaxLagBytes: 1},
			{Name: "bad-ping", Host: "bad-ping.local", Port: 5432, Database: "app", SSLMode: "disable", Role: "replica", State: "active", Weight: 1, MaxLagBytes: 1},
		},
	}

	goodPool := newReplicaTestPool(t)
	t.Cleanup(goodPool.Close)
	badPingPool := newReplicaTestPool(t)
	t.Cleanup(badPingPool.Close)

	prevNewReplicaPool := newReplicaPool
	prevPingReplicaPool := pingReplicaPool
	t.Cleanup(func() {
		newReplicaPool = prevNewReplicaPool
		pingReplicaPool = prevPingReplicaPool
	})

	newReplicaPool = func(_ context.Context, dsn string) (*pgxpool.Pool, error) {
		switch {
		case strings.Contains(dsn, "bad-connect"):
			return nil, errors.New("connect failed")
		case strings.Contains(dsn, "bad-ping"):
			return badPingPool, nil
		default:
			return goodPool, nil
		}
	}
	pingReplicaPool = func(_ context.Context, pool *pgxpool.Pool) error {
		if pool == badPingPool {
			return errors.New("ping failed")
		}
		return nil
	}

	result := buildReplicaRouting(context.Background(), store, primary, logger)
	if result.router == nil {
		t.Fatal("expected router to be created from healthy replica")
	}
	if result.checker == nil {
		t.Fatal("expected health checker to be created from healthy replica")
	}
	if got := len(result.router.Replicas()); got != 1 {
		t.Fatalf("len(router.Replicas()) = %d, want 1", got)
	}
}

func TestBuildReplicaRoutingNoActiveReplicasReturnsPassThrough(t *testing.T) {
	t.Parallel()

	store := &fakeReplicaTopologyStore{
		listRecords: []replica.TopologyNodeRecord{
			{Name: "primary", Role: "primary", State: "active"},
			{Name: "replica-removed", Role: "replica", State: "removed"},
		},
	}

	result := buildReplicaRouting(context.Background(), store, &pgxpool.Pool{}, testReplicaLogger())
	testutil.NotNil(t, result.router)
	testutil.NotNil(t, result.checker)
	testutil.Equal(t, 0, len(result.initialPools))
}

func TestNewServerWiresLifecycleServiceWithZeroActiveReplicas(t *testing.T) {
	// Mutates the replica store factory to force a deterministic zero-replica
	// startup path through newServer.

	cfg := config.Default()
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	store := &fakeReplicaTopologyStore{
		listRecords: []replica.TopologyNodeRecord{
			{Name: "primary", Host: "primary.local", Port: 5432, Database: "appdb", SSLMode: "disable", Role: "primary", State: "active"},
		},
	}

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)

	prevStoreFactory := newReplicaStore
	t.Cleanup(func() {
		newReplicaStore = prevStoreFactory
	})
	newReplicaStore = func(_ *pgxpool.Pool) replica.ReplicaStore { return store }

	s := newServer(cfg, logger, ch, primary, nil, nil, nil)
	testutil.NotNil(t, s.poolRouter)
	testutil.NotNil(t, s.healthChecker)
	testutil.NotNil(t, s.lifecycleService)
}

func TestBuildReplicaRoutingDialsWithPrimaryCredentials(t *testing.T) {
	// Mutates package-level replica dial/ping hooks for deterministic wiring tests.
	// Keep non-parallel to avoid races with other tests touching the same hooks.

	store := &fakeReplicaTopologyStore{
		listRecords: []replica.TopologyNodeRecord{
			{Name: "primary", Host: "primary.local", Port: 5432, Database: "appdb", SSLMode: "require", Role: "primary", State: "active", Weight: 1, MaxLagBytes: 1},
			{Name: "replica-auth", Host: "replica-auth.local", Port: 5432, Database: "appdb", SSLMode: "disable", Role: "replica", State: "active", Weight: 1, MaxLagBytes: 1},
		},
	}
	logger := testReplicaLogger()
	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	var dialedURL string
	prevNewReplicaPool := newReplicaPool
	prevPingReplicaPool := pingReplicaPool
	t.Cleanup(func() {
		newReplicaPool = prevNewReplicaPool
		pingReplicaPool = prevPingReplicaPool
	})

	newReplicaPool = func(_ context.Context, dsn string) (*pgxpool.Pool, error) {
		dialedURL = dsn
		return replicaPool, nil
	}
	pingReplicaPool = func(_ context.Context, _ *pgxpool.Pool) error { return nil }

	result := buildReplicaRouting(context.Background(), store, primary, logger)
	testutil.NotNil(t, result.checker)
	testutil.Contains(t, dialedURL, "postgres:postgres@replica-auth.local")

	statuses := result.checker.Statuses()
	testutil.SliceLen(t, statuses, 1)
	testutil.True(t, !strings.Contains(statuses[0].Config.URL, "postgres:postgres@"))
	testutil.Equal(t, "postgres://replica-auth.local:5432/appdb?sslmode=disable", statuses[0].Config.URL)
}

func TestNewServerBootstrapsReplicaStoreWhenEmpty(t *testing.T) {
	// Mutates package-level store and replica dial/ping hooks for deterministic wiring tests.
	// Keep non-parallel to avoid races with other tests touching the same hooks.

	cfg := config.Default()
	cfg.Database.URL = "postgres://primary.local:5432/appdb?sslmode=require"
	cfg.Database.Replicas = []config.ReplicaConfig{{
		URL:         "postgres://replica-a.local:5432/appdb?sslmode=disable",
		Weight:      0,
		MaxLagBytes: 0,
	}}

	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	store := &fakeReplicaTopologyStore{isEmpty: true}

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	prevStoreFactory := newReplicaStore
	prevNewReplicaPool := newReplicaPool
	prevPingReplicaPool := pingReplicaPool
	t.Cleanup(func() {
		newReplicaStore = prevStoreFactory
		newReplicaPool = prevNewReplicaPool
		pingReplicaPool = prevPingReplicaPool
	})

	newReplicaStore = func(_ *pgxpool.Pool) replica.ReplicaStore { return store }
	newReplicaPool = func(_ context.Context, _ string) (*pgxpool.Pool, error) { return replicaPool, nil }
	pingReplicaPool = func(_ context.Context, _ *pgxpool.Pool) error { return nil }

	s := newServer(cfg, logger, ch, primary, nil, nil, nil)
	testutil.NotNil(t, s.poolRouter)
	testutil.NotNil(t, s.healthChecker)
	testutil.Equal(t, 1, store.bootstrapCalls)
	testutil.SliceLen(t, store.bootstrapRecords, 2)
	testutil.Equal(t, "primary", store.bootstrapRecords[0].Name)
	testutil.Equal(t, "replica", store.bootstrapRecords[1].Role)
	testutil.Equal(t, config.DefaultReplicaWeight, store.bootstrapRecords[1].Weight)
	testutil.Equal(t, config.DefaultReplicaMaxLagBytes, store.bootstrapRecords[1].MaxLagBytes)
}

func TestNewServerSkipsBootstrapWhenStoreAlreadyPopulated(t *testing.T) {
	// Mutates package-level store and replica dial/ping hooks for deterministic wiring tests.
	// Keep non-parallel to avoid races with other tests touching the same hooks.

	cfg := config.Default()
	cfg.Database.URL = "postgres://primary.local:5432/appdb?sslmode=require"
	cfg.Database.Replicas = []config.ReplicaConfig{{
		URL:         "postgres://config-replica.local:5432/appdb?sslmode=disable",
		Weight:      5,
		MaxLagBytes: 4096,
	}}

	store := &fakeReplicaTopologyStore{
		isEmpty: false,
		listRecords: []replica.TopologyNodeRecord{
			{Name: "primary", Host: "primary.local", Port: 5432, Database: "appdb", SSLMode: "require", Role: "primary", State: "active", Weight: 1, MaxLagBytes: 1},
			{Name: "stored-replica", Host: "stored-replica.local", Port: 5432, Database: "appdb", SSLMode: "disable", Role: "replica", State: "active", Weight: 3, MaxLagBytes: 1024},
		},
	}

	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	dialedURLs := make([]string, 0, 1)
	prevStoreFactory := newReplicaStore
	prevNewReplicaPool := newReplicaPool
	prevPingReplicaPool := pingReplicaPool
	t.Cleanup(func() {
		newReplicaStore = prevStoreFactory
		newReplicaPool = prevNewReplicaPool
		pingReplicaPool = prevPingReplicaPool
	})

	newReplicaStore = func(_ *pgxpool.Pool) replica.ReplicaStore { return store }
	newReplicaPool = func(_ context.Context, dsn string) (*pgxpool.Pool, error) {
		dialedURLs = append(dialedURLs, dsn)
		return replicaPool, nil
	}
	pingReplicaPool = func(_ context.Context, _ *pgxpool.Pool) error { return nil }

	s := newServer(cfg, logger, ch, primary, nil, nil, nil)
	testutil.NotNil(t, s.poolRouter)
	testutil.Equal(t, 0, store.bootstrapCalls)
	testutil.SliceLen(t, dialedURLs, 1)
	testutil.Contains(t, dialedURLs[0], "stored-replica.local")
	testutil.True(t, !strings.Contains(dialedURLs[0], "config-replica.local"))
}

func TestBuildReplicaRoutingPreservesStoredReplicaQueryHints(t *testing.T) {
	// Mutates package-level replica dial/ping hooks for deterministic wiring tests.
	// Keep non-parallel to avoid races with other tests touching the same hooks.

	store := &fakeReplicaTopologyStore{
		listRecords: []replica.TopologyNodeRecord{
			{Name: "primary", Host: "primary.local", Port: 5432, Database: "appdb", SSLMode: "require", Query: "sslmode=require", Role: "primary", State: "active", Weight: 1, MaxLagBytes: 1},
			{Name: "stored-replica", Host: "stored-replica.local", Port: 5432, Database: "appdb", SSLMode: "disable", Query: "application_name=stored-replica&sslmode=disable", Role: "replica", State: "active", Weight: 3, MaxLagBytes: 1024},
		},
	}

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	var dialedURL string
	prevNewReplicaPool := newReplicaPool
	prevPingReplicaPool := pingReplicaPool
	t.Cleanup(func() {
		newReplicaPool = prevNewReplicaPool
		pingReplicaPool = prevPingReplicaPool
	})

	newReplicaPool = func(_ context.Context, dsn string) (*pgxpool.Pool, error) {
		dialedURL = dsn
		return replicaPool, nil
	}
	pingReplicaPool = func(_ context.Context, _ *pgxpool.Pool) error { return nil }

	result := buildReplicaRouting(context.Background(), store, primary, testReplicaLogger())
	testutil.NotNil(t, result.checker)
	testutil.Contains(t, dialedURL, "application_name=stored-replica")

	statuses := result.checker.Statuses()
	testutil.SliceLen(t, statuses, 1)
	testutil.Contains(t, statuses[0].Config.URL, "application_name=stored-replica")
}

func TestNewServerBootstrapPreservesOmittedReplicaSSLMode(t *testing.T) {
	// Mutates package-level store and replica dial/ping hooks for deterministic wiring tests.
	// Keep non-parallel to avoid races with other tests touching the same hooks.

	cfg := config.Default()
	cfg.Database.URL = "postgres://primary.local:5432/appdb?sslmode=require"
	cfg.Database.Replicas = []config.ReplicaConfig{{
		URL:         "postgres://replica-a.local:5432/appdb?application_name=replica-a",
		Weight:      1,
		MaxLagBytes: 1,
	}}

	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	store := &fakeReplicaTopologyStore{isEmpty: true}

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	var dialedURL string
	prevStoreFactory := newReplicaStore
	prevNewReplicaPool := newReplicaPool
	prevPingReplicaPool := pingReplicaPool
	t.Cleanup(func() {
		newReplicaStore = prevStoreFactory
		newReplicaPool = prevNewReplicaPool
		pingReplicaPool = prevPingReplicaPool
	})

	newReplicaStore = func(_ *pgxpool.Pool) replica.ReplicaStore { return store }
	newReplicaPool = func(_ context.Context, dsn string) (*pgxpool.Pool, error) {
		dialedURL = dsn
		return replicaPool, nil
	}
	pingReplicaPool = func(_ context.Context, _ *pgxpool.Pool) error { return nil }

	s := newServer(cfg, logger, ch, primary, nil, nil, nil)
	testutil.NotNil(t, s.poolRouter)
	testutil.NotNil(t, s.healthChecker)
	testutil.SliceLen(t, store.bootstrapRecords, 2)
	testutil.Equal(t, "", store.bootstrapRecords[1].SSLMode)
	testutil.Equal(t, "application_name=replica-a", store.bootstrapRecords[1].Query)
	testutil.Equal(t, "postgres://postgres:postgres@replica-a.local:5432/appdb?application_name=replica-a", dialedURL)

	statuses := s.healthChecker.Statuses()
	testutil.SliceLen(t, statuses, 1)
	testutil.Equal(t, "postgres://replica-a.local:5432/appdb?application_name=replica-a", statuses[0].Config.URL)
}

func TestStartHealthCheckerNoOpWhenNil(t *testing.T) {
	t.Parallel()
	s := &Server{}
	s.startHealthChecker()
	testutil.True(t, true)
}

func TestStartWithReadyStartsHealthChecker(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Server.ShutdownTimeout = 2

	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	router := replica.NewPoolRouter(primary, []replica.ReplicaPool{
		{
			Pool: replicaPool,
			Config: config.ReplicaConfig{
				URL:         "postgres://replica-1.local:5432/appdb?application_name=replica-1",
				Weight:      1,
				MaxLagBytes: 1,
			},
		},
	}, logger)
	s.poolRouter = router
	s.healthChecker = replica.NewHealthChecker(router, time.Second, logger)

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.StartWithReady(ready)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not become ready")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statuses := s.healthChecker.Statuses()
		if len(statuses) == 1 && !statuses[0].LastCheckedAt.IsZero() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	statuses := s.healthChecker.Statuses()
	if len(statuses) != 1 || statuses[0].LastCheckedAt.IsZero() {
		t.Fatal("expected health checker to run at least one cycle")
	}

	testutil.NoError(t, s.Shutdown(context.Background()))
	testutil.NoError(t, <-errCh)
}

func TestShutdownStopsHealthCheckerAndClosesReplicaPools(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.ShutdownTimeout = 2
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)
	s.http = &http.Server{}

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	router := replica.NewPoolRouter(primary, []replica.ReplicaPool{
		{
			Pool: replicaPool,
			Config: config.ReplicaConfig{
				URL:         "postgres://replica-1.local:5432/appdb?application_name=replica-1",
				Weight:      1,
				MaxLagBytes: 1,
			},
		},
	}, logger)
	s.poolRouter = router
	s.healthChecker = replica.NewHealthChecker(router, 10*time.Millisecond, logger)
	s.healthChecker.Start()

	testutil.NoError(t, s.Shutdown(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := replicaPool.Acquire(ctx)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "closed") {
		t.Fatalf("expected closed pool error after shutdown, got: %v", err)
	}
}

func TestShutdownWithoutReplicasNoPanic(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.ShutdownTimeout = 1
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)
	s.http = &http.Server{}

	testutil.NoError(t, s.Shutdown(context.Background()))
}

func TestNewServerMetricsIncludeReplicaMetrics(t *testing.T) {
	// Mutates package-level replica dial/ping hooks for deterministic wiring tests.
	// Keep non-parallel to avoid races with other tests touching the same hooks.

	cfg := config.Default()
	cfg.Metrics.Enabled = true
	store := &fakeReplicaTopologyStore{
		isEmpty: false,
		listRecords: []replica.TopologyNodeRecord{
			{Name: "primary", Host: "primary.local", Port: 5432, Database: "appdb", SSLMode: "require", Role: "primary", State: "active", Weight: 1, MaxLagBytes: 1},
			{Name: "replica-1", Host: "replica-1.local", Port: 5432, Database: "appdb", SSLMode: "disable", Role: "replica", State: "active", Weight: 1, MaxLagBytes: 1024},
		},
	}

	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	prevNewReplicaPool := newReplicaPool
	prevPingReplicaPool := pingReplicaPool
	prevStoreFactory := newReplicaStore
	t.Cleanup(func() {
		newReplicaPool = prevNewReplicaPool
		pingReplicaPool = prevPingReplicaPool
		newReplicaStore = prevStoreFactory
	})
	newReplicaStore = func(_ *pgxpool.Pool) replica.ReplicaStore { return store }
	newReplicaPool = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return replicaPool, nil
	}
	pingReplicaPool = func(_ context.Context, _ *pgxpool.Pool) error { return nil }

	s := newServer(cfg, logger, ch, primary, nil, nil, nil)
	testutil.NotNil(t, s.poolRouter)
	testutil.NotNil(t, s.healthChecker)

	_ = s.poolRouter.ReadPool()
	_ = s.poolRouter.ReadPool()
	s.poolRouter.SetHealthy(nil)
	_ = s.poolRouter.ReadPool()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	testutil.Contains(t, body, `ayb_db_replica_lag_bytes{replica="postgres://replica-1.local:5432/appdb?sslmode=disable"}`)
	testutil.Contains(t, body, `ayb_db_replica_status{replica="postgres://replica-1.local:5432/appdb?sslmode=disable"} 0`)
	testutil.Contains(t, body, `ayb_db_queries_routed_total{target="replica"} 2`)
	testutil.Contains(t, body, `ayb_db_queries_routed_total{target="primary"} 1`)
}

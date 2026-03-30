package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Package-level test-seam variables for replica pool construction.
var newReplicaPool = pgxpool.New

var pingReplicaPool = func(ctx context.Context, pool *pgxpool.Pool) error {
	return pool.Ping(ctx)
}

var newReplicaStore = func(pool *pgxpool.Pool) replica.ReplicaStore {
	return replica.NewPostgresReplicaStore(pool)
}

// replicaRoutingResult holds the output of buildReplicaRouting so callers
// can wire both the router/checker and pass the pool map to LifecycleService.
type replicaRoutingResult struct {
	router       *replica.PoolRouter
	checker      *replica.HealthChecker
	initialPools map[string]*pgxpool.Pool
}

// buildReplicaRouting creates a PoolRouter and HealthChecker from persisted
// topology records, connecting to active replicas and skipping those that fail
// to connect or ping. It always returns a usable router/checker when
// store+primary are present, even with zero connected replicas (pass-through
// mode), so that AddReplica works on a fresh or degraded system.
func buildReplicaRouting(ctx context.Context, store replica.ReplicaStore, primary *pgxpool.Pool, logger *slog.Logger) replicaRoutingResult {
	empty := replicaRoutingResult{}
	if store == nil || primary == nil {
		return empty
	}
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	records, err := safeReplicaStoreList(ctx, store)
	if err != nil {
		logger.Warn("replica topology load failed; replica routing disabled", "error", err)
		return empty
	}

	replicaPools := make([]replica.ReplicaPool, 0)
	initialPools := make(map[string]*pgxpool.Pool)
	for _, record := range records {
		if record.Role != replica.TopologyRoleReplica || record.State != replica.TopologyStateActive {
			continue
		}
		connectionURL := record.ConnectionURL()
		sanitizedConnectionURL := replica.SanitizeReplicaURL(connectionURL)
		dialURL := replica.DialURLWithPrimaryCredentials(connectionURL, primary)
		pool, dialErr := newReplicaPool(ctx, dialURL)
		if dialErr != nil {
			logger.Warn("replica connection failed; skipping replica", "name", record.Name, "url", sanitizedConnectionURL, "error", dialErr)
			continue
		}

		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		pingErr := pingReplicaPool(pingCtx, pool)
		cancel()
		if pingErr != nil {
			pool.Close()
			logger.Warn("replica ping failed; skipping replica", "name", record.Name, "url", sanitizedConnectionURL, "error", pingErr)
			continue
		}

		replicaConfig := replica.NormalizeReplicaConfig(config.ReplicaConfig{
			URL:         connectionURL,
			Weight:      record.Weight,
			MaxLagBytes: record.MaxLagBytes,
		})
		replicaPools = append(replicaPools, replica.ReplicaPool{
			Name:   record.Name,
			Pool:   pool,
			Config: replicaConfig,
		})
		initialPools[record.Name] = pool
	}

	poolRouter := replica.NewPoolRouter(primary, replicaPools, logger)
	healthChecker := replica.NewHealthChecker(poolRouter, 0, logger)
	if len(replicaPools) > 0 {
		logger.Info("replica routing enabled", "replicas", len(replicaPools))
	} else {
		logger.Info("replica routing enabled in pass-through mode; no active replicas connected")
	}
	return replicaRoutingResult{
		router:       poolRouter,
		checker:      healthChecker,
		initialPools: initialPools,
	}
}

// buildLifecycleService constructs the replica LifecycleService when
// all required dependencies are available. Returns nil otherwise.
func buildLifecycleService(cfg *config.Config, store replica.ReplicaStore, pool *pgxpool.Pool, routing replicaRoutingResult, logger *slog.Logger) replicaLifecycle {
	if store == nil || pool == nil || routing.router == nil || routing.checker == nil {
		return nil
	}
	auditLogger := audit.NewAuditLogger(cfg.Audit, pool)
	return replica.NewLifecycleService(store, routing.router, routing.checker, auditLogger, logger, routing.initialPools)
}

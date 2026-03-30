package replica

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultHealthCheckInterval = 10 * time.Second
	replicaPingTimeout         = 2 * time.Second
	replicaLagTimeout          = 2 * time.Second
)

var errReplicaNotFoundInReplication = errors.New("replica not found in pg_stat_replication")

// defaultHealthJitter adds ±20% jitter to a base duration to prevent
// synchronized health checks from multiple AYB instances (thundering herd
// on pg_stat_replication).
func defaultHealthJitter(base time.Duration) time.Duration {
	jitterRange := base / 5 // 20%
	return base - jitterRange + time.Duration(rand.Int64N(int64(2*jitterRange)+1))
}

var sensitiveReplicaURLQueryKeys = map[string]struct{}{
	"password":    {},
	"passfile":    {},
	"sslpassword": {},
	"user":        {},
}

type ReplicaHealth int

const (
	HealthHealthy ReplicaHealth = iota
	HealthSuspect
	HealthUnhealthy
)

func (h ReplicaHealth) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthSuspect:
		return "suspect"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

type ReplicaStatus struct {
	Name                 string
	Pool                 *pgxpool.Pool
	Config               ReplicaConfig
	State                ReplicaHealth
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LagBytes             int64
	LastError            error
	LastCheckedAt        time.Time
}

type replicationLagRow struct {
	ApplicationName string
	ClientAddr      string
	LagBytes        int64
}

type HealthChecker struct {
	router    *PoolRouter
	statuses  []*ReplicaStatus
	logger    *slog.Logger
	interval  time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once

	mu sync.RWMutex

	pingReplicaFn     func(ctx context.Context, status *ReplicaStatus) error
	lagCheckFn        func(ctx context.Context, status *ReplicaStatus) (int64, error)
	replicationRowsFn func(ctx context.Context) ([]replicationLagRow, error)
	nowFn             func() time.Time
	jitterFn          func(base time.Duration) time.Duration
	afterCycleHook    func()
}

func NewHealthChecker(router *PoolRouter, interval time.Duration, logger *slog.Logger) *HealthChecker {
	if interval <= 0 {
		interval = defaultHealthCheckInterval
	}
	if logger == nil {
		logger = slog.Default()
	}

	checker := &HealthChecker{
		router:   router,
		logger:   logger,
		interval: interval,
		stopCh:   make(chan struct{}),
		nowFn:    time.Now,
		jitterFn: defaultHealthJitter,
	}

	if router != nil {
		replicas := router.Replicas()
		checker.statuses = make([]*ReplicaStatus, 0, len(replicas))
		for _, replica := range replicas {
			if replica == nil || replica.pool == nil {
				continue
			}
			checker.statuses = append(checker.statuses, &ReplicaStatus{
				Name:   replica.name,
				Pool:   replica.pool,
				Config: replica.config,
				State:  HealthHealthy,
			})
		}
	}

	checker.pingReplicaFn = checker.defaultPingReplica
	checker.replicationRowsFn = checker.defaultReplicationRows
	checker.lagCheckFn = checker.checkLag

	return checker
}

func (h *HealthChecker) defaultPingReplica(ctx context.Context, status *ReplicaStatus) error {
	if status == nil || status.Pool == nil {
		return errors.New("replica pool is nil")
	}

	var ping int
	if err := status.Pool.QueryRow(ctx, "SELECT 1").Scan(&ping); err != nil {
		return err
	}
	return nil
}

// defaultReplicationRows queries the primary database's pg_stat_replication view, returning application name, client address, and replication lag for each connected replica.
func (h *HealthChecker) defaultReplicationRows(ctx context.Context) ([]replicationLagRow, error) {
	if h.router == nil || h.router.Primary() == nil {
		return nil, errors.New("primary pool is nil")
	}

	rows, err := h.router.Primary().Query(ctx, `
		SELECT
			COALESCE(application_name, ''),
			COALESCE(client_addr::text, ''),
			COALESCE(pg_wal_lsn_diff(sent_lsn, replay_lsn), 0)::bigint
		FROM pg_stat_replication
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]replicationLagRow, 0)
	for rows.Next() {
		var row replicationLagRow
		if scanErr := rows.Scan(&row.ApplicationName, &row.ClientAddr, &row.LagBytes); scanErr != nil {
			return nil, scanErr
		}
		result = append(result, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return result, nil
}

// checkReplica checks a single replica's connectivity via ping and replication lag, applying the results to update its health state. Ping and lag checks use separate timeouts.
func (h *HealthChecker) checkReplica(ctx context.Context, status *ReplicaStatus) {
	if status == nil {
		return
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, replicaPingTimeout)
	pingErr := h.pingReplicaFn(pingCtx, status)
	cancelPing()
	if pingErr != nil {
		h.applyCheckResult(status, false, 0, pingErr)
		return
	}

	lagCtx, cancelLag := context.WithTimeout(ctx, replicaLagTimeout)
	lagBytes, lagErr := h.lagCheckFn(lagCtx, status)
	cancelLag()
	if lagErr != nil {
		h.applyCheckResult(status, false, 0, lagErr)
		return
	}

	h.applyCheckResult(status, true, lagBytes, nil)
}

func (h *HealthChecker) checkLag(ctx context.Context, status *ReplicaStatus) (int64, error) {
	if status == nil {
		return 0, errors.New("replica status is nil")
	}

	rows, err := h.replicationRowsFn(ctx)
	if err != nil {
		return 0, err
	}

	hints, parseErr := parseReplicaHints(status.Config.URL)
	if parseErr != nil {
		// Bad replica URL — lag check will fail. Log so operators can
		// trace "replica not found" back to a config problem.
		h.logger.Warn("replica URL parse failed; lag check will not match",
			slog.String("url", SanitizeReplicaURL(status.Config.URL)),
			slog.Any("error", parseErr),
		)
	}
	row, ok := selectReplicationLagRow(rows, hints)
	if !ok {
		return 0, errReplicaNotFoundInReplication
	}

	if row.LagBytes > status.Config.MaxLagBytes {
		return row.LagBytes, fmt.Errorf("replica lag %d exceeds max %d", row.LagBytes, status.Config.MaxLagBytes)
	}
	return row.LagBytes, nil
}

func parseReplicaHints(rawURL string) (replicaHints, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return replicaHints{}, fmt.Errorf("parse replica URL: %w", err)
	}

	return replicaHints{
		host:            strings.ToLower(parsed.Hostname()),
		applicationName: parsed.Query().Get("application_name"),
	}, nil
}

type replicaHints struct {
	host            string
	applicationName string
}

// selectReplicationLagRow finds a replication lag row matching the given hints using a scoring system that prioritizes rows matching both host and application name. Returns false if no row matches or if multiple rows tie with the same score.
func selectReplicationLagRow(rows []replicationLagRow, hints replicaHints) (replicationLagRow, bool) {
	var (
		selected  replicationLagRow
		found     bool
		ambiguous bool
		bestScore int
	)

	for _, row := range rows {
		score := rowMatchScore(row, hints)
		if score == 0 {
			continue
		}

		if !found || score > bestScore {
			selected = row
			bestScore = score
			found = true
			ambiguous = false
			continue
		}

		if score == bestScore {
			ambiguous = true
		}
	}

	if !found || ambiguous {
		return replicationLagRow{}, false
	}

	return selected, true
}

func rowMatchScore(row replicationLagRow, hints replicaHints) int {
	score := 0
	if hints.applicationName != "" && row.ApplicationName == hints.applicationName {
		score++
	}
	if hints.host != "" && strings.EqualFold(row.ClientAddr, hints.host) {
		score++
	}
	return score
}

// SanitizeReplicaURL removes credentials and sensitive query parameters before
// a replica connection string crosses a log, metric, or admin-response boundary.
func SanitizeReplicaURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-replica-url>"
	}

	parsed.User = nil
	values := parsed.Query()
	for key := range values {
		if _, sensitive := sensitiveReplicaURLQueryKeys[strings.ToLower(strings.TrimSpace(key))]; sensitive {
			values.Del(key)
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

// applyCheckResult updates a replica's state based on check success or failure, tracking consecutive successes and failures, and transitioning between healthy, suspect, and unhealthy states. State changes are logged as warnings.
func (h *HealthChecker) applyCheckResult(status *ReplicaStatus, success bool, lagBytes int64, resultErr error) {
	if status == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	from := status.State
	if success {
		status.ConsecutiveSuccesses++
		status.ConsecutiveFailures = 0
		status.LagBytes = lagBytes
		status.LastError = nil

		switch status.State {
		case HealthUnhealthy:
			status.State = HealthSuspect
		case HealthSuspect:
			status.State = HealthHealthy
		case HealthHealthy:
			// no-op
		}
	} else {
		status.ConsecutiveFailures++
		status.ConsecutiveSuccesses = 0
		status.LastError = resultErr

		switch status.State {
		case HealthHealthy:
			status.State = HealthSuspect
		case HealthSuspect:
			status.State = HealthUnhealthy
		case HealthUnhealthy:
			// no-op
		}
	}
	status.LastCheckedAt = h.nowFn()

	if from != status.State {
		h.logger.Warn("replica health state changed",
			slog.String("url", SanitizeReplicaURL(status.Config.URL)),
			slog.String("from", from.String()),
			slog.String("to", status.State.String()),
			slog.Any("error", resultErr),
		)
	}
}

// runCheckCycle executes a complete health check iteration: pings and checks lag for each replica, updates the router's healthy pool list based on current replica states, and runs any registered afterCycleHook.
func (h *HealthChecker) runCheckCycle(ctx context.Context) {
	statuses := h.statusesSnapshot()
	for _, status := range statuses {
		h.checkReplica(ctx, status)
	}

	healthyPools := make([]*pgxpool.Pool, 0, len(statuses))
	for _, status := range statuses {
		if status != nil && status.State == HealthHealthy && status.Pool != nil {
			healthyPools = append(healthyPools, status.Pool)
		}
	}

	if h.router != nil {
		h.router.SetHealthy(healthyPools)
	}

	if h.afterCycleHook != nil {
		h.afterCycleHook()
	}
}

func (h *HealthChecker) statusesSnapshot() []*ReplicaStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]*ReplicaStatus(nil), h.statuses...)
}

// RunCheck runs one immediate health-check cycle.
func (h *HealthChecker) RunCheck(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	h.runCheckCycle(ctx)
}

func (h *HealthChecker) Start() {
	h.startOnce.Do(func() {
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					h.logger.Error("replica health checker panic recovered", "panic", r)
				}
			}()

			h.runCheckCycle(context.Background())

			// Use a timer with jitter instead of a fixed ticker to prevent
			// synchronized health checks across multiple AYB instances.
			timer := time.NewTimer(h.jitterFn(h.interval))
			defer timer.Stop()

			for {
				select {
				case <-h.stopCh:
					return
				case <-timer.C:
					h.runCheckCycle(context.Background())
					timer.Reset(h.jitterFn(h.interval))
				}
			}
		}()
	})
}

func (h *HealthChecker) Stop() {
	h.stopOnce.Do(func() {
		close(h.stopCh)
	})
	h.wg.Wait()
}

func (h *HealthChecker) Statuses() []ReplicaStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	snapshot := make([]ReplicaStatus, 0, len(h.statuses))
	for _, status := range h.statuses {
		if status == nil {
			continue
		}
		snapshot = append(snapshot, *status)
	}

	return snapshot
}

// AddStatus registers a new replica pool for health monitoring, initializing it as healthy.
func (h *HealthChecker) AddStatus(pool *pgxpool.Pool, name string, cfg ReplicaConfig) {
	if pool == nil {
		return
	}

	normalizedConfig := NormalizeReplicaConfig(cfg)

	h.mu.Lock()
	defer h.mu.Unlock()
	h.statuses = append(h.statuses, &ReplicaStatus{
		Name:   name,
		Pool:   pool,
		Config: normalizedConfig,
		State:  HealthHealthy,
	})
}

// RemoveStatus removes the health-tracking entry for the given replica pool.
func (h *HealthChecker) RemoveStatus(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	filtered := make([]*ReplicaStatus, 0, len(h.statuses))
	for _, status := range h.statuses {
		if status == nil || status.Pool == nil {
			continue
		}
		if status.Pool == pool {
			continue
		}
		filtered = append(filtered, status)
	}
	h.statuses = filtered
}

// Package tenant UsageAccumulator tracks cumulative and peak resource usage metrics per tenant and periodically persists them to the database.
package tenant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UsageAccumulator struct {
	pool   *pgxpool.Pool
	logger *slog.Logger

	mu sync.RWMutex
	// additiveCounters tracks cumulative counts: tenant -> resource -> count
	additiveCounters map[string]map[ResourceType]int64
	// peakCounters tracks max values: tenant -> resource -> peak
	peakCounters map[string]map[ResourceType]int64
}

func NewUsageAccumulator(pool *pgxpool.Pool, logger *slog.Logger) *UsageAccumulator {
	return &UsageAccumulator{
		pool:             pool,
		logger:           logger,
		additiveCounters: make(map[string]map[ResourceType]int64),
		peakCounters:     make(map[string]map[ResourceType]int64),
	}
}

func (ua *UsageAccumulator) Record(tenantID string, resource ResourceType, delta int64) {
	ua.mu.Lock()
	defer ua.mu.Unlock()

	if _, ok := ua.additiveCounters[tenantID]; !ok {
		ua.additiveCounters[tenantID] = make(map[ResourceType]int64)
	}
	ua.additiveCounters[tenantID][resource] += delta
}

func (ua *UsageAccumulator) RecordPeak(tenantID string, resource ResourceType, current int64) {
	ua.mu.Lock()
	defer ua.mu.Unlock()

	if _, ok := ua.peakCounters[tenantID]; !ok {
		ua.peakCounters[tenantID] = make(map[ResourceType]int64)
	}

	currentPeak := ua.peakCounters[tenantID][resource]
	if current > currentPeak {
		ua.peakCounters[tenantID][resource] = current
	}
}

func (ua *UsageAccumulator) GetCurrentWindow(tenantID string, resource ResourceType) int64 {
	ua.mu.RLock()
	defer ua.mu.RUnlock()

	if counters, ok := ua.additiveCounters[tenantID]; ok {
		return counters[resource]
	}
	return 0
}

func (ua *UsageAccumulator) GetCurrentPeakWindow(tenantID string, resource ResourceType) int64 {
	ua.mu.RLock()
	defer ua.mu.RUnlock()

	if counters, ok := ua.peakCounters[tenantID]; ok {
		return counters[resource]
	}
	return 0
}

func cloneResourceCounters(src map[string]map[ResourceType]int64) map[string]map[ResourceType]int64 {
	clone := make(map[string]map[ResourceType]int64, len(src))
	for tenantID, resources := range src {
		clone[tenantID] = make(map[ResourceType]int64, len(resources))
		for res, value := range resources {
			clone[tenantID][res] = value
		}
	}
	return clone
}

func collectTenantIDs(counters ...map[string]map[ResourceType]int64) map[string]struct{} {
	tenantIDs := make(map[string]struct{})
	for _, counterSet := range counters {
		for tenantID := range counterSet {
			tenantIDs[tenantID] = struct{}{}
		}
	}
	return tenantIDs
}

// restoreSnapshots merges previously captured usage snapshots back into the accumulators, restoring additive counters by summing values and peak counters by taking the maximum.
func (ua *UsageAccumulator) restoreSnapshots(additive, peak map[string]map[ResourceType]int64) {
	ua.mu.Lock()
	defer ua.mu.Unlock()

	for tenantID, resources := range additive {
		if _, ok := ua.additiveCounters[tenantID]; !ok {
			ua.additiveCounters[tenantID] = make(map[ResourceType]int64)
		}
		for res, value := range resources {
			ua.additiveCounters[tenantID][res] += value
		}
	}

	for tenantID, resources := range peak {
		if _, ok := ua.peakCounters[tenantID]; !ok {
			ua.peakCounters[tenantID] = make(map[ResourceType]int64)
		}
		for res, value := range resources {
			if value > ua.peakCounters[tenantID][res] {
				ua.peakCounters[tenantID][res] = value
			}
		}
	}
}

// Flush writes all accumulated usage metrics to the database, clearing in-memory counters. If a write fails, it restores the snapshots and returns an error.
func (ua *UsageAccumulator) Flush(ctx context.Context) error {
	ua.mu.Lock()
	additiveCopy := cloneResourceCounters(ua.additiveCounters)
	peakCopy := cloneResourceCounters(ua.peakCounters)
	ua.additiveCounters = make(map[string]map[ResourceType]int64)
	ua.peakCounters = make(map[string]map[ResourceType]int64)
	ua.mu.Unlock()

	if len(additiveCopy) == 0 && len(peakCopy) == 0 {
		return nil
	}

	if ua.pool == nil {
		return nil
	}

	tenantIDs := collectTenantIDs(additiveCopy, peakCopy)

	now := time.Now().UTC().Truncate(24 * time.Hour)

	for tenantID := range tenantIDs {
		var requestCount, dbBytesUsed, jobRuns int64
		var bandwidthBytes, functionInvocations int64
		if resources, ok := additiveCopy[tenantID]; ok {
			requestCount = resources[ResourceTypeRequestRate]
			dbBytesUsed = resources[ResourceTypeDBSizeBytes]
			jobRuns = resources[ResourceTypeJobConcurrency]
			bandwidthBytes = resources[ResourceTypeBandwidthBytes]
			functionInvocations = resources[ResourceTypeFunctionInvocations]
		}

		var peakConnections int
		if peakResources, ok := peakCopy[tenantID]; ok {
			if pc := peakResources[ResourceTypeRealtimeConns]; pc > 0 {
				peakConnections = int(pc)
			}
		}

		if requestCount > 0 || dbBytesUsed > 0 || bandwidthBytes > 0 || functionInvocations > 0 || peakConnections > 0 || jobRuns > 0 {
			_, err := ua.pool.Exec(ctx,
				`INSERT INTO _ayb_tenant_usage_daily
					(tenant_id, date, request_count, db_bytes_used, bandwidth_bytes, function_invocations, realtime_peak_connections, job_runs)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				 ON CONFLICT (tenant_id, date) DO UPDATE SET
					request_count = _ayb_tenant_usage_daily.request_count + EXCLUDED.request_count,
					db_bytes_used = _ayb_tenant_usage_daily.db_bytes_used + EXCLUDED.db_bytes_used,
					bandwidth_bytes = _ayb_tenant_usage_daily.bandwidth_bytes + EXCLUDED.bandwidth_bytes,
					function_invocations = _ayb_tenant_usage_daily.function_invocations + EXCLUDED.function_invocations,
					realtime_peak_connections = GREATEST(_ayb_tenant_usage_daily.realtime_peak_connections, EXCLUDED.realtime_peak_connections),
					job_runs = _ayb_tenant_usage_daily.job_runs + EXCLUDED.job_runs`,
				tenantID, now, requestCount, dbBytesUsed, bandwidthBytes, functionInvocations, peakConnections, jobRuns,
			)
			if err != nil {
				ua.restoreSnapshots(additiveCopy, peakCopy)
				return fmt.Errorf("flushing usage for tenant %s: %w", tenantID, err)
			}
		}

		delete(additiveCopy, tenantID)
		delete(peakCopy, tenantID)
	}

	if ua.logger != nil {
		ua.logger.Info("usage flushed", "tenants", len(tenantIDs))
	}
	return nil
}

// StartPeriodicFlush launches a background goroutine that periodically flushes accumulated usage to the database at the specified interval. When the context is cancelled, it performs a final flush before returning.
func (ua *UsageAccumulator) StartPeriodicFlush(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if flushErr := ua.Flush(context.Background()); flushErr != nil {
				if ua.logger != nil {
					ua.logger.Error("final flush on shutdown", "error", flushErr)
				}
			}
			return
		case <-ticker.C:
			if err := ua.Flush(ctx); err != nil {
				if ua.logger != nil {
					ua.logger.Error("periodic flush failed", "error", err)
				}
			}
		}
	}
}

// resourceColumn maps a ResourceType to its column in _ayb_tenant_usage_daily.
var resourceColumn = map[ResourceType]string{
	ResourceTypeRequestRate:         "request_count",
	ResourceTypeDBSizeBytes:         "db_bytes_used",
	ResourceTypeBandwidthBytes:      "bandwidth_bytes",
	ResourceTypeFunctionInvocations: "function_invocations",
	ResourceTypeRealtimeConns:       "realtime_peak_connections",
	ResourceTypeJobConcurrency:      "job_runs",
}

// isPeakResource returns true for resource types that track peaks (max) rather
// than additive sums. Peak resources use GetCurrentPeakWindow and merge with
// max() instead of addition.
func isPeakResource(r ResourceType) bool {
	return r == ResourceTypeRealtimeConns
}

// GetCurrentUsage returns the current total usage for a given tenant and resource type, combining both in-memory accumulated values and persisted database values. For peak resources, it returns the maximum; for additive resources, it returns the sum.
func (ua *UsageAccumulator) GetCurrentUsage(ctx context.Context, tenantID string, resource ResourceType) (int64, error) {
	var windowUsage int64
	if isPeakResource(resource) {
		windowUsage = ua.GetCurrentPeakWindow(tenantID, resource)
	} else {
		windowUsage = ua.GetCurrentWindow(tenantID, resource)
	}

	col, ok := resourceColumn[resource]
	if !ok {
		return windowUsage, nil
	}
	if ua.pool == nil {
		return windowUsage, nil
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)

	var persisted int64
	// Column name is from a compile-time constant map, not user input.
	err := ua.pool.QueryRow(ctx,
		`SELECT `+col+` FROM _ayb_tenant_usage_daily WHERE tenant_id = $1 AND date = $2`,
		tenantID, now,
	).Scan(&persisted)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return windowUsage, nil
		}
		return 0, fmt.Errorf("getting current usage: %w", err)
	}

	if isPeakResource(resource) {
		if persisted > windowUsage {
			return persisted, nil
		}
		return windowUsage, nil
	}

	return persisted + windowUsage, nil
}

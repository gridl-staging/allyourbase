// Package replica PoolRouter routes database queries to a primary pool and distributes read-only queries across replica pools using weighted round-robin load balancing with health status tracking.
package replica

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReplicaConfig = config.ReplicaConfig

type ReplicaPool struct {
	Name   string
	Pool   *pgxpool.Pool
	Config ReplicaConfig
}

type replicaEntry struct {
	name   string
	pool   *pgxpool.Pool
	config ReplicaConfig
}

type PoolRouter struct {
	primary      *pgxpool.Pool
	replicas     []*replicaEntry
	healthy      []*replicaEntry
	selection    []*replicaEntry
	mu           sync.RWMutex
	counter      atomic.Uint64
	primaryReads atomic.Uint64
	replicaReads atomic.Uint64
	logger       *slog.Logger
	passThrough  bool
	closeOnce    sync.Once
}

// NewPoolRouter creates and initializes a PoolRouter that distributes read-only queries across replicas based on health and configured weights. If no replicas are available, it operates in passThrough mode and routes all queries to the primary pool.
func NewPoolRouter(primary *pgxpool.Pool, replicas []ReplicaPool, logger *slog.Logger) *PoolRouter {
	router := &PoolRouter{
		primary: primary,
		logger:  logger,
	}
	if len(replicas) == 0 {
		router.passThrough = true
		return router
	}

	router.replicas = make([]*replicaEntry, 0, len(replicas))
	for _, replica := range replicas {
		if replica.Pool == nil {
			continue
		}
		normalizedConfig := NormalizeReplicaConfig(replica.Config)
		router.replicas = append(router.replicas, &replicaEntry{
			name:   replica.Name,
			pool:   replica.Pool,
			config: normalizedConfig,
		})
	}
	if len(router.replicas) == 0 {
		router.passThrough = true
		return router
	}

	router.healthy = append([]*replicaEntry(nil), router.replicas...)
	router.selection = buildWeightedSelection(router.healthy)

	return router
}

func NormalizeReplicaConfig(cfg ReplicaConfig) ReplicaConfig {
	cfg.Weight = effectiveWeight(cfg.Weight)
	if cfg.MaxLagBytes <= 0 {
		cfg.MaxLagBytes = config.DefaultReplicaMaxLagBytes
	}
	return cfg
}

func effectiveWeight(weight int) int {
	if weight < config.DefaultReplicaWeight {
		return config.DefaultReplicaWeight
	}
	return weight
}

// buildWeightedSelection constructs a flattened selection array for round-robin load balancing, repeating each entry according to its weight. It returns nil if total weight across entries is zero.
func buildWeightedSelection(entries []*replicaEntry) []*replicaEntry {
	totalWeight := 0
	for _, entry := range entries {
		if entry == nil || entry.pool == nil {
			continue
		}
		weight := effectiveWeight(entry.config.Weight)
		totalWeight += weight
	}
	if totalWeight == 0 {
		return nil
	}

	selection := make([]*replicaEntry, 0, totalWeight)
	for _, entry := range entries {
		if entry == nil || entry.pool == nil {
			continue
		}
		weight := effectiveWeight(entry.config.Weight)
		for i := 0; i < weight; i++ {
			selection = append(selection, entry)
		}
	}
	return selection
}

func (p *PoolRouter) Primary() *pgxpool.Pool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.primary
}

// ReadPool selects a database pool for read-only queries using weighted round-robin distribution across healthy replicas. It falls back to the primary pool if no replicas are available or in passThrough mode, and increments corresponding read counters.
func (p *PoolRouter) ReadPool() *pgxpool.Pool {
	p.mu.RLock()
	passThrough := p.passThrough
	selection := p.selection
	primary := p.primary
	p.mu.RUnlock()

	if passThrough {
		p.primaryReads.Add(1)
		return primary
	}

	if len(selection) == 0 {
		p.primaryReads.Add(1)
		return primary
	}

	idx := (p.counter.Add(1) - 1) % uint64(len(selection))
	selected := selection[idx]
	if selected == nil || selected.pool == nil {
		p.primaryReads.Add(1)
		return primary
	}
	p.replicaReads.Add(1)
	return selected.pool
}

func (p *PoolRouter) Acquire(ctx context.Context, readOnly bool) (*pgxpool.Conn, error) {
	pool := p.Primary()
	if readOnly {
		pool = p.ReadPool()
	}
	if pool == nil {
		return nil, errors.New("replica: selected pool is nil")
	}
	return pool.Acquire(ctx)
}

func (p *PoolRouter) Close() {
	p.closeOnce.Do(func() {
		p.mu.RLock()
		replicas := append([]*replicaEntry(nil), p.replicas...)
		p.mu.RUnlock()
		for _, replica := range replicas {
			if replica == nil || replica.pool == nil {
				continue
			}
			replica.pool.Close()
		}
	})
}

func (p *PoolRouter) HasReplicas() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.replicas) > 0
}

// SetHealthy updates the set of healthy replica pools and rebuilds the weighted selection for load balancing. It is a no-op in passThrough mode and must be called when replica health changes to exclude unhealthy replicas from query routing.
func (p *PoolRouter) SetHealthy(pools []*pgxpool.Pool) {
	healthySet := make(map[*pgxpool.Pool]struct{}, len(pools))
	for _, pool := range pools {
		if pool == nil {
			continue
		}
		healthySet[pool] = struct{}{}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.passThrough {
		return
	}

	healthy := make([]*replicaEntry, 0, len(p.replicas))
	for _, replica := range p.replicas {
		if replica == nil || replica.pool == nil {
			continue
		}
		if _, ok := healthySet[replica.pool]; ok {
			healthy = append(healthy, replica)
		}
	}

	p.healthy = healthy
	p.selection = buildWeightedSelection(healthy)
}

func (p *PoolRouter) Replicas() []*replicaEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]*replicaEntry, len(p.replicas))
	for i, entry := range p.replicas {
		if entry == nil {
			continue
		}
		entryCopy := *entry
		out[i] = &entryCopy
	}
	return out
}

func (p *PoolRouter) RoutingStats() (primary, replica uint64) {
	return p.primaryReads.Load(), p.replicaReads.Load()
}

// TODO: Document PoolRouter.AddReplicaEntry.
func (p *PoolRouter) AddReplicaEntry(pool *pgxpool.Pool, name string, cfg ReplicaConfig) {
	if pool == nil {
		return
	}

	normalizedConfig := NormalizeReplicaConfig(cfg)
	newEntry := &replicaEntry{
		name:   name,
		pool:   pool,
		config: normalizedConfig,
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.replicas = append(p.replicas, newEntry)
	p.healthy = append(p.healthy, newEntry)
	p.selection = buildWeightedSelection(p.healthy)
	p.passThrough = false
}

func (p *PoolRouter) RemoveReplicaEntry(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.replicas = filterReplicaEntriesByPool(p.replicas, pool)
	p.healthy = filterReplicaEntriesByPool(p.healthy, pool)
	p.selection = buildWeightedSelection(p.healthy)
	p.passThrough = len(p.replicas) == 0
}

func (p *PoolRouter) SwapPrimary(newPrimary *pgxpool.Pool) (old *pgxpool.Pool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	old = p.primary
	p.primary = newPrimary
	return old
}

func filterReplicaEntriesByPool(entries []*replicaEntry, pool *pgxpool.Pool) []*replicaEntry {
	filtered := make([]*replicaEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.pool == nil {
			continue
		}
		if entry.pool == pool {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

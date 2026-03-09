package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps a pgx connection pool with health checking and lifecycle management.
type Pool struct {
	pool            *pgxpool.Pool
	healthCheckStop chan struct{}
	wg              sync.WaitGroup
	closeOnce       sync.Once
	logger          *slog.Logger
}

// Config holds database connection parameters.
type Config struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	HealthCheckSecs int
}

// New creates a new Pool, validates the connection, and starts health checking.
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Pool, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// Validate connection at startup.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	// Check Postgres version.
	var version string
	if err := pool.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		pool.Close()
		return nil, fmt.Errorf("querying server version: %w", err)
	}
	logger.Info("connected to PostgreSQL", "version", version)

	p := &Pool{
		pool:            pool,
		healthCheckStop: make(chan struct{}),
		logger:          logger,
	}

	// Start periodic health checks.
	if cfg.HealthCheckSecs > 0 {
		p.startHealthCheck(time.Duration(cfg.HealthCheckSecs) * time.Second)
	}

	return p, nil
}

// DB returns the underlying pgxpool.Pool for executing queries.
func (p *Pool) DB() *pgxpool.Pool {
	return p.pool
}

// Close gracefully shuts down the pool and stops health checking.
// Safe to call multiple times.
func (p *Pool) Close() {
	p.closeOnce.Do(func() {
		close(p.healthCheckStop)
		p.wg.Wait()
		p.pool.Close()
		p.logger.Info("database connection pool closed")
	})
}

func (p *Pool) startHealthCheck(interval time.Duration) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.healthCheckStop:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := p.pool.Ping(ctx); err != nil {
					p.logger.Warn("database health check failed", "error", err)
				}
				cancel()
			}
		}
	}()
}

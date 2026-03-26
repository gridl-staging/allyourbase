// Package status Defines probes for checking health of database and other services. Each probe reports health status, latency, and error details.
package status

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Probe is one service health probe used by the checker.
type Probe interface {
	Name() ServiceName
	Check(ctx context.Context) ProbeResult
}

type pinger interface {
	Ping(ctx context.Context) error
}

// DatabaseProbe checks database reachability via pgx pool ping.
type DatabaseProbe struct {
	pinger pinger
}

// NewDatabaseProbe constructs a database probe backed by a pgx pool.
func NewDatabaseProbe(pool *pgxpool.Pool) *DatabaseProbe {
	if pool == nil {
		return &DatabaseProbe{}
	}
	return &DatabaseProbe{pinger: pool}
}

func (p *DatabaseProbe) Name() ServiceName {
	return Database
}

// Check pings the database via the configured pgx pool and returns a ProbeResult with health status, latency, and any error. If the probe or its pinger is nil, an unhealthy result is returned without attempting to ping.
func (p *DatabaseProbe) Check(ctx context.Context) ProbeResult {
	start := time.Now().UTC()
	if p == nil || p.pinger == nil {
		return ProbeResult{
			Service:   Database,
			Healthy:   false,
			Error:     "database pool not configured",
			CheckedAt: start,
		}
	}

	err := p.pinger.Ping(ctx)
	latency := time.Since(start)
	if err != nil {
		return ProbeResult{
			Service:   Database,
			Healthy:   false,
			Latency:   latency,
			Error:     err.Error(),
			CheckedAt: start,
		}
	}

	return ProbeResult{
		Service:   Database,
		Healthy:   true,
		Latency:   latency,
		CheckedAt: start,
	}
}

type staticHealthyProbe struct {
	service ServiceName
}

func (p staticHealthyProbe) Name() ServiceName {
	return p.service
}

func (p staticHealthyProbe) Check(ctx context.Context) ProbeResult {
	_ = ctx
	now := time.Now().UTC()
	return ProbeResult{
		Service:   p.service,
		Healthy:   true,
		Latency:   0,
		CheckedAt: now,
	}
}

func NewStorageProbe() Probe {
	return staticHealthyProbe{service: Storage}
}

func NewAuthProbe() Probe {
	return staticHealthyProbe{service: Auth}
}

func NewRealtimeProbe() Probe {
	return staticHealthyProbe{service: Realtime}
}

func NewFunctionsProbe() Probe {
	return staticHealthyProbe{service: Functions}
}

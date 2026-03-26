// Package server manages HTTP request routing for custom domains, maintaining an in-memory route table that is periodically synced from the domain store and used as middleware for routing decisions. It also handles expiration of tombstoned domain entries.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/jobs"
)

type RouteEntry struct {
	DomainID     string
	Hostname     string
	Environment  string
	Status       DomainStatus
	RedirectMode *string
	TombstonedAt *time.Time
}

type RouteTable map[string]RouteEntry

// NewRouteTable constructs a RouteTable from a slice of RouteEntry objects, normalizing hostnames to lowercase and applying deduplication rules based on status.
func NewRouteTable(entries []RouteEntry) RouteTable {
	rt := make(RouteTable, len(entries))
	for _, e := range entries {
		normalizedHostname := normalizeRouteHostname(e.Hostname)
		if normalizedHostname == "" {
			continue
		}
		e.Hostname = normalizedHostname

		existing, exists := rt[normalizedHostname]
		if !exists || shouldReplaceRouteEntry(existing, e) {
			rt[normalizedHostname] = e
		}
	}
	return rt
}

func (rt RouteTable) Lookup(hostname string) (RouteEntry, bool) {
	entry, ok := rt[normalizeRouteHostname(hostname)]
	return entry, ok
}

func normalizeRouteHostname(hostname string) string {
	return strings.ToLower(strings.TrimSpace(hostname))
}

func shouldReplaceRouteEntry(existing, candidate RouteEntry) bool {
	if existing.Status == StatusTombstoned && candidate.Status != StatusTombstoned {
		return false
	}
	if existing.Status != StatusTombstoned && candidate.Status == StatusTombstoned {
		return true
	}
	return true
}

type DomainRouteLister interface {
	ListDomainsForRouting(ctx context.Context) ([]DomainBinding, error)
}

type DomainTombstoneReaper interface {
	ReapExpiredTombstones(ctx context.Context) (int64, error)
}

type customDomainRouteKey struct{}

func CustomDomainRouteFromContext(ctx context.Context) (RouteEntry, bool) {
	re, ok := ctx.Value(customDomainRouteKey{}).(RouteEntry)
	return re, ok
}

// loadRouteTable fetches domain route entries from a lister, filters for routable statuses (Active, Tombstoned, or VerificationLapsed), and creates a RouteTable with deduplicated entries. It logs statistics about the loaded entries.
func loadRouteTable(ctx context.Context, lister DomainRouteLister, logger *slog.Logger) (RouteTable, error) {
	bindings, err := lister.ListDomainsForRouting(ctx)
	if err != nil {
		return nil, err
	}

	var activeCount, tombstonedCount, lapsedCount int
	entries := make([]RouteEntry, 0, len(bindings))
	for _, b := range bindings {
		switch b.Status {
		case StatusActive, StatusTombstoned, StatusVerificationLapsed:
			// These statuses are routable.
		default:
			continue
		}
		entries = append(entries, RouteEntry{
			DomainID:     b.ID,
			Hostname:     b.Hostname,
			Environment:  b.Environment,
			Status:       b.Status,
			RedirectMode: b.RedirectMode,
			TombstonedAt: b.TombstonedAt,
		})
		switch b.Status {
		case StatusActive:
			activeCount++
		case StatusTombstoned:
			tombstonedCount++
		case StatusVerificationLapsed:
			lapsedCount++
		}
	}

	rt := NewRouteTable(entries)

	if logger != nil {
		logger.Info("loaded route table",
			"total_entries", len(entries),
			"active", activeCount,
			"tombstoned", tombstonedCount,
			"lapsed", lapsedCount,
		)
	}

	return rt, nil
}

// LoadRouteTable loads the current domain routes from the domainStore and updates the server's internal route table. It does nothing if the domainStore is nil or does not support DomainRouteLister.
func (s *Server) LoadRouteTable(ctx context.Context, logger *slog.Logger) error {
	if s.domainStore == nil {
		return nil
	}

	lister, ok := s.domainStore.(DomainRouteLister)
	if !ok {
		return nil
	}

	rt, err := loadRouteTable(ctx, lister, logger)
	if err != nil {
		return err
	}

	s.setRouteTable(rt)
	return nil
}

func (s *Server) setRouteTable(rt RouteTable) {
	s.routeTableMu.Lock()
	s.routeTable = rt
	s.routeTableMu.Unlock()
}

func (s *Server) lookupRoute(hostname string) (RouteEntry, bool) {
	s.routeTableMu.RLock()
	rt := s.routeTable
	s.routeTableMu.RUnlock()
	return rt.Lookup(hostname)
}

// hostRouteMiddleware is HTTP middleware that extracts the request hostname, looks it up in the route table, and adds the matching RouteEntry to the request context. Tombstoned hostnames receive an HTTP 421 Misdirected Request response.
func (s *Server) hostRouteMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := normalizeRequestHost(r.Host)

		entry, ok := s.lookupRoute(host)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		if entry.Status == StatusTombstoned {
			httputil.WriteError(w, http.StatusMisdirectedRequest, "hostname is no longer active")
			return
		}

		ctx := context.WithValue(r.Context(), customDomainRouteKey{}, entry)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

const (
	JobTypeDomainRouteSync     = "domain_route_sync"
	JobTypeDomainTombstoneReap = "domain_tombstone_reap"
)

// DomainRouteSyncHandler returns a job handler that reloads the domain route table from the lister and updates the server's route table.
func DomainRouteSyncHandler(srv *Server, lister DomainRouteLister, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		rt, err := loadRouteTable(ctx, lister, logger)
		if err != nil {
			if logger != nil {
				logger.Warn("domain_route_sync: failed to load route table", "error", err)
			}
			return err
		}

		srv.setRouteTable(rt)

		if logger != nil {
			logger.Info("domain_route_sync: route table updated",
				"entries", len(rt),
			)
		}

		return nil
	}
}

// registerDomainSchedule is the shared helper for all domain-related schedule
// registrations. Every schedule uses UTC, Enabled=true, MaxAttempts=3.
func registerDomainSchedule(ctx context.Context, svc *jobs.Service, name, jobType, cronExpr string) error {
	if svc == nil {
		return fmt.Errorf("job service is nil")
	}
	schedule := &jobs.Schedule{
		Name:        name,
		JobType:     jobType,
		CronExpr:    cronExpr,
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	}
	next, err := jobs.CronNextTime(schedule.CronExpr, schedule.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("compute next_run_at for %s: %w", name, err)
	}
	schedule.NextRunAt = &next
	if _, err := svc.UpsertSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("upsert schedule %s: %w", name, err)
	}
	return nil
}

func RegisterDomainRouteSyncSchedule(ctx context.Context, svc *jobs.Service) error {
	return registerDomainSchedule(ctx, svc, "domain_route_sync_5m", JobTypeDomainRouteSync, "*/5 * * * *")
}

// DomainTombstoneReapHandler returns a job handler that removes expired tombstoned domain entries by calling the provided reaper.
func DomainTombstoneReapHandler(reaper DomainTombstoneReaper, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		count, err := reaper.ReapExpiredTombstones(ctx)
		if err != nil {
			if logger != nil {
				logger.Warn("domain_tombstone_reap: failed to reap tombstones", "error", err)
			}
			return err
		}

		if logger != nil {
			logger.Info("domain_tombstone_reap: removed expired tombstones",
				"count", count,
			)
		}

		return nil
	}
}

func RegisterDomainTombstoneReapSchedule(ctx context.Context, svc *jobs.Service) error {
	return registerDomainSchedule(ctx, svc, "domain_tombstone_reap_daily", JobTypeDomainTombstoneReap, "0 3 * * *")
}

// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/usage_handler_aggregate.go.
package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

const (
	defaultUsageListLimit      = 50
	maxUsageListLimit          = 500
	defaultUsageBreakdownLimit = 10
	maxUsageBreakdownLimit     = 100
)

type usageAggregateService interface {
	ListTenantUsageSummaries(ctx context.Context, opts billing.ListUsageOpts) ([]billing.TenantUsageSummaryRow, int, error)
	GetUsageTrends(ctx context.Context, opts billing.TrendOpts) ([]billing.TrendPoint, error)
	GetUsageBreakdown(ctx context.Context, opts billing.BreakdownOpts) ([]billing.BreakdownEntry, error)
	GetTenantUsageLimits(ctx context.Context, tenantID, period string, from, to time.Time) (*billing.UsageLimitsResponse, error)
}

type usageListResponse struct {
	Items  []billing.TenantUsageSummaryRow `json:"items"`
	Total  int                             `json:"total"`
	Limit  int                             `json:"limit"`
	Offset int                             `json:"offset"`
}

type usageTrendResponse struct {
	Metric      string               `json:"metric"`
	Granularity string               `json:"granularity"`
	Items       []billing.TrendPoint `json:"items"`
}

type usageBreakdownResponse struct {
	Metric  string                   `json:"metric"`
	GroupBy string                   `json:"groupBy"`
	Items   []billing.BreakdownEntry `json:"items"`
}

func resolveUsageDateRangeFromQuery(query url.Values, defaultPeriod string, now time.Time) (string, time.Time, time.Time, error) {
	period := strings.TrimSpace(query.Get("period"))
	if period == "" {
		period = defaultPeriod
	}
	fromRaw := strings.TrimSpace(query.Get("from"))
	toRaw := strings.TrimSpace(query.Get("to"))
	from, to, err := billing.ResolvePeriodRange(period, fromRaw, toRaw, now)
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	return period, from, to, nil
}

// TODO: Document parseUsageIntQuery.
func parseUsageIntQuery(raw, field string, defaultValue, maxValue int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, errors.New(field + " must be non-negative")
	}
	if parsed > maxValue {
		return 0, errors.New(field + " must be <= " + strconv.Itoa(maxValue))
	}
	return parsed, nil
}

func writeUsageLimitsResponse(w http.ResponseWriter, r *http.Request, aggregate usageAggregateService, tenantID string) {
	period, from, to, err := resolveUsageDateRangeFromQuery(r.URL.Query(), "month", time.Now().UTC())
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid period or date range")
		return
	}

	resp, err := aggregate.GetTenantUsageLimits(r.Context(), tenantID, period, from, to)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load usage limits")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// TODO: Document handleAdminUsageList.
func handleAdminUsageList(aggregate usageAggregateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if aggregate == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage aggregation service not configured")
			return
		}

		query := r.URL.Query()
		period, from, to, err := resolveUsageDateRangeFromQuery(query, "month", time.Now().UTC())
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid period or date range")
			return
		}

		sortColumn, sortDirection, err := billing.ParseUsageSort(strings.TrimSpace(query.Get("sort")))
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid sort")
			return
		}

		limit, err := parseUsageIntQuery(query.Get("limit"), "limit", defaultUsageListLimit, maxUsageListLimit)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		offset, err := parseUsageIntQuery(query.Get("offset"), "offset", 0, 1_000_000)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid offset")
			return
		}

		items, total, err := aggregate.ListTenantUsageSummaries(r.Context(), billing.ListUsageOpts{
			Period: period,
			From:   from,
			To:     to,
			Sort: billing.UsageSort{
				Column:    sortColumn,
				Direction: sortDirection,
			},
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to load usage summaries")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, usageListResponse{
			Items:  items,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}

// TODO: Document handleAdminUsageTrends.
func handleAdminUsageTrends(aggregate usageAggregateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if aggregate == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage aggregation service not configured")
			return
		}

		query := r.URL.Query()
		metric := strings.TrimSpace(query.Get("metric"))
		if metric == "" {
			httputil.WriteError(w, http.StatusBadRequest, "metric is required")
			return
		}
		if err := billing.ValidateMetric(metric); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid metric")
			return
		}

		granularity := strings.TrimSpace(query.Get("granularity"))
		if granularity == "" {
			granularity = "day"
		}
		if err := billing.ValidateGranularity(granularity, metric); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid granularity")
			return
		}

		_, from, to, err := resolveUsageDateRangeFromQuery(query, "month", time.Now().UTC())
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid date range")
			return
		}

		items, err := aggregate.GetUsageTrends(r.Context(), billing.TrendOpts{
			Metric:      metric,
			Granularity: granularity,
			From:        from,
			To:          to,
		})
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to load usage trends")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, usageTrendResponse{
			Metric:      metric,
			Granularity: granularity,
			Items:       items,
		})
	}
}

// TODO: Document handleAdminUsageBreakdown.
func handleAdminUsageBreakdown(aggregate usageAggregateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if aggregate == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage aggregation service not configured")
			return
		}

		query := r.URL.Query()
		metric := strings.TrimSpace(query.Get("metric"))
		if metric == "" {
			httputil.WriteError(w, http.StatusBadRequest, "metric is required")
			return
		}
		if err := billing.ValidateMetric(metric); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid metric")
			return
		}

		groupBy := strings.TrimSpace(query.Get("group_by"))
		if groupBy == "" {
			groupBy = "tenant"
		}
		if err := billing.ValidateGroupBy(groupBy); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid group_by")
			return
		}
		if err := billing.ValidateBreakdownMetricGroup(metric, groupBy); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid metric/group_by combination")
			return
		}

		_, from, to, err := resolveUsageDateRangeFromQuery(query, "month", time.Now().UTC())
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid date range")
			return
		}

		limit, err := parseUsageIntQuery(query.Get("limit"), "limit", defaultUsageBreakdownLimit, maxUsageBreakdownLimit)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid limit")
			return
		}

		items, err := aggregate.GetUsageBreakdown(r.Context(), billing.BreakdownOpts{
			Metric:  metric,
			GroupBy: groupBy,
			From:    from,
			To:      to,
			Limit:   limit,
		})
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to load usage breakdown")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, usageBreakdownResponse{
			Metric:  metric,
			GroupBy: groupBy,
			Items:   items,
		})
	}
}

// TODO: Document handleAdminUsageLimits.
func handleAdminUsageLimits(src usageDataSource, aggregate usageAggregateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if src == nil || aggregate == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage service not configured")
			return
		}

		tenantID, ok := validateTenantPathID(w, r)
		if !ok {
			return
		}

		if _, err := src.GetTenant(r.Context(), tenantID); err != nil {
			if errors.Is(err, tenant.ErrTenantNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "tenant not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to load tenant")
			return
		}

		writeUsageLimitsResponse(w, r, aggregate, tenantID)
	}
}

func handleTenantUsageLimits(src usageDataSource, aggregate usageAggregateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if src == nil || aggregate == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage service not configured")
			return
		}

		tenantID, ok := resolveTenantIDForSelfService(w, r)
		if !ok {
			return
		}

		writeUsageLimitsResponse(w, r, aggregate, tenantID)
	}
}

func (s *Server) registerAdminUsageRoutes(r chi.Router) {
	if s.usageSrc == nil || s.usageAggregate == nil {
		return
	}

	r.Route("/admin/usage", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", handleAdminUsageList(s.usageAggregate))
		r.Get("/trends", handleAdminUsageTrends(s.usageAggregate))
		r.Get("/breakdown", handleAdminUsageBreakdown(s.usageAggregate))
		r.Get("/{tenant_id}", handleAdminUsage(s.usageSrc))
		r.Get("/{tenant_id}/limits", handleAdminUsageLimits(s.usageSrc, s.usageAggregate))
	})
}

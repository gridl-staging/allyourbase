// Package server Provides HTTP handlers for retrieving tenant usage statistics, supporting both admin access to any tenant and self-service access for individual tenants.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

// usageDataSource is the minimal dependency surface required by usage handlers.
type usageDataSource interface {
	GetTenant(ctx context.Context, tenantID string) (*tenant.Tenant, error)
	GetUsageRange(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]tenant.TenantUsageDaily, error)
	GetBillingRecord(ctx context.Context, tenantID string) (*billing.BillingRecord, error)
}

type usageTenantReader interface {
	GetTenant(ctx context.Context, id string) (*tenant.Tenant, error)
	GetUsageRange(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]tenant.TenantUsageDaily, error)
}

type usageBillingReader interface {
	Get(ctx context.Context, tenantID string) (*billing.BillingRecord, error)
}

type usageDataSourceAdapter struct {
	tenantSvc   usageTenantReader
	billingRepo usageBillingReader
}

var (
	errResolveUsagePlan = errors.New("resolve usage plan")
	errLoadUsageRows    = errors.New("load usage rows")
)

func newUsageDataSource(tenantSvc usageTenantReader, billingRepo usageBillingReader) usageDataSource {
	if tenantSvc == nil || billingRepo == nil {
		return nil
	}
	return &usageDataSourceAdapter{tenantSvc: tenantSvc, billingRepo: billingRepo}
}

func (a *usageDataSourceAdapter) GetTenant(ctx context.Context, tenantID string) (*tenant.Tenant, error) {
	return a.tenantSvc.GetTenant(ctx, tenantID)
}

func (a *usageDataSourceAdapter) GetUsageRange(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]tenant.TenantUsageDaily, error) {
	return a.tenantSvc.GetUsageRange(ctx, tenantID, startDate, endDate)
}

func (a *usageDataSourceAdapter) GetBillingRecord(ctx context.Context, tenantID string) (*billing.BillingRecord, error) {
	return a.billingRepo.Get(ctx, tenantID)
}

func normalizedUsagePeriod(period string) string {
	period = strings.TrimSpace(period)
	if period == "" {
		return "month"
	}
	return period
}

func resolveUsageQueryRange(r *http.Request, now time.Time) (string, time.Time, time.Time, error) {
	period := normalizedUsagePeriod(r.URL.Query().Get("period"))
	fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
	toStr := strings.TrimSpace(r.URL.Query().Get("to"))
	start, end, err := billing.ResolvePeriodRange(period, fromStr, toStr, now)
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	return period, start, end, nil
}

func resolveUsagePlan(ctx context.Context, src usageDataSource, tenantID string) (billing.Plan, error) {
	rec, err := src.GetBillingRecord(ctx, tenantID)
	if err != nil {
		if errors.Is(err, billing.ErrBillingRecordNotFound) {
			return billing.PlanFree, nil
		}
		return "", err
	}
	if rec == nil || rec.Plan == "" {
		return billing.PlanFree, nil
	}
	return rec.Plan, nil
}

func validateTenantPathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	tenantID := strings.TrimSpace(chi.URLParam(r, "tenant_id"))
	if tenantID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "tenant_id is required")
		return "", false
	}
	if !httputil.IsValidUUID(tenantID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid tenant_id format")
		return "", false
	}
	return tenantID, true
}

// TODO: Document resolveTenantIDForSelfService.
func resolveTenantIDForSelfService(w http.ResponseWriter, r *http.Request) (string, bool) {
	tenantID := ""
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		tenantID = strings.TrimSpace(claims.TenantID)
		if tenantID == "" {
			// Do not trust header/context fallback when a JWT is present but missing tenant scope.
			httputil.WriteError(w, http.StatusForbidden, "tenant context required")
			return "", false
		}
	}
	if tenantID == "" {
		tenantID = strings.TrimSpace(tenant.TenantFromContext(r.Context()))
	}
	if tenantID == "" {
		httputil.WriteError(w, http.StatusForbidden, "tenant context required")
		return "", false
	}
	return tenantID, true
}

// Compiles usage statistics for a tenant within a specified date range. It retrieves the billing plan and usage records for the period, then constructs a summary. Returns an error if the plan resolution or usage data retrieval fails.
func buildUsageSummaryForTenant(ctx context.Context, src usageDataSource, tenantID, period string, start, end time.Time) (*billing.UsageSummary, error) {
	plan, err := resolveUsagePlan(ctx, src, tenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errResolveUsagePlan, err)
	}

	rows, err := src.GetUsageRange(ctx, tenantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errLoadUsageRows, err)
	}

	summary := billing.BuildUsageSummary(rows, plan, period)
	if summary.TenantID == "" {
		summary.TenantID = tenantID
	}
	return summary, nil
}

func writeUsageSummaryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errResolveUsagePlan):
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load billing plan")
	case errors.Is(err, errLoadUsageRows):
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load usage")
	default:
		httputil.WriteError(w, http.StatusInternalServerError, "failed to build usage summary")
	}
}

// Returns an HTTP handler for retrieving usage statistics for a specified tenant. The handler validates the tenant_id path parameter, verifies the tenant exists, and returns usage data for the specified period, defaulting to monthly. Returns service unavailable if the data source is not configured.
func handleAdminUsage(src usageDataSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if src == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage service not configured")
			return
		}

		tenantID, ok := validateTenantPathID(w, r)
		if !ok {
			return
		}

		period, start, end, err := resolveUsageQueryRange(r, time.Now().UTC())
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid period or date range")
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

		summary, err := buildUsageSummaryForTenant(r.Context(), src, tenantID, period, start, end)
		if err != nil {
			writeUsageSummaryError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, summary)
	}
}

// Returns an HTTP handler for retrieving a tenant's own usage statistics. The handler obtains the tenant ID from JWT claims or request context and validates that tenant context is present before returning usage data. Returns service unavailable if the data source is not configured.
func handleTenantUsage(src usageDataSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if src == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage service not configured")
			return
		}

		tenantID, ok := resolveTenantIDForSelfService(w, r)
		if !ok {
			return
		}

		period, start, end, err := resolveUsageQueryRange(r, time.Now().UTC())
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid period or date range")
			return
		}

		summary, err := buildUsageSummaryForTenant(r.Context(), src, tenantID, period, start, end)
		if err != nil {
			writeUsageSummaryError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, summary)
	}
}

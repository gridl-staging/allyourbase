// Package server provides middleware for enforcing tenant quotas on HTTP requests and WebSocket connections.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

// tenantQuotaReader returns quotas for a tenant.
type tenantQuotaReader interface {
	GetQuotas(ctx context.Context, tenantID string) (*tenant.TenantQuotas, error)
}

type tenantMetricsRecorder interface {
	RecordQuotaUtilization(ctx context.Context, tenantID, resource string, current, limit int64)
	IncrQuotaViolation(ctx context.Context, tenantID, resource string)
}

const (
	headerTenantQuotaWarning     = "X-Tenant-Quota-Warning"
	headerRetryAfter             = "Retry-After"
	headerRateLimitLimit         = "X-RateLimit-Limit"
	headerRateLimitRemaining     = "X-RateLimit-Remaining"
	headerRateLimitReset         = "X-RateLimit-Reset"
	tenantRequestRateWarningCode = "request_rate"
)

// tenantRequestRateMiddleware enforces tenant-request quotas.
func tenantRequestRateMiddleware(limiter *tenant.TenantRateLimiter, quotaReader tenantQuotaReader, acc *tenant.UsageAccumulator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enforceTenantRequestRate(w, r, next, limiter, quotaReader, acc, nil, nil)
		})
	}
}

func (s *Server) tenantRequestRateMiddlewareDynamic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s == nil {
			next.ServeHTTP(w, r)
			return
		}
		enforceTenantRequestRate(w, r, next, s.tenantRateLimiter, s.tenantQuotaReader, s.usageAccumulator, s.auditEmitter, s.tenantMetrics)
	})
}

// enforceTenantRequestRate enforces tenant request rate quotas on HTTP requests, checking against hard and soft limits and returning HTTP 429 if the hard limit is exceeded. It records quota utilization and violations, emits audit events, and sets rate limit response headers including Retry-After.
func enforceTenantRequestRate(w http.ResponseWriter, r *http.Request, next http.Handler, limiter *tenant.TenantRateLimiter, quotaReader tenantQuotaReader, acc *tenant.UsageAccumulator, emitter *tenant.AuditEmitter, tenantMetrics tenantMetricsRecorder) {
	if limiter == nil {
		next.ServeHTTP(w, r)
		return
	}

	tenantID := tenant.TenantFromContext(r.Context())
	if tenantID == "" {
		next.ServeHTTP(w, r)
		return
	}

	if quotaReader == nil {
		next.ServeHTTP(w, r)
		return
	}

	quotas, err := quotaReader.GetQuotas(r.Context(), tenantID)
	if err != nil {
		slog.Default().Error("loading tenant quotas failed", "tenant_id", tenantID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load tenant quotas")
		return
	}

	if quotas != nil {
		limitPerMin := effectiveRateLimitPerMinute(quotas.RequestRateRPSHard, quotas.RequestRateRPSSoft)
		allowed, softWarning, remaining, retryAfter := limiter.Allow(tenantID, quotas.RequestRateRPSHard, quotas.RequestRateRPSSoft)
		current := int64(limitPerMin - remaining)
		if current < 0 {
			current = 0
		}
		if tenantMetrics != nil && limitPerMin > 0 {
			tenantMetrics.RecordQuotaUtilization(r.Context(), tenantID, string(tenant.ResourceTypeRequestRate), current, int64(limitPerMin))
		}
		if !allowed {
			if tenantMetrics != nil {
				tenantMetrics.IncrQuotaViolation(r.Context(), tenantID, string(tenant.ResourceTypeRequestRate))
			}
			w.Header().Set(headerRateLimitLimit, strconv.Itoa(limitPerMin))
			w.Header().Set(headerRateLimitRemaining, strconv.Itoa(remaining))
			resetTime := time.Now().Add(retryAfter).Unix()
			w.Header().Set(headerRateLimitReset, strconv.FormatInt(resetTime, 10))
			retryAfterSeconds := int(time.Until(time.Unix(resetTime, 0)).Seconds())
			if retryAfterSeconds < 1 {
				retryAfterSeconds = 1
			}
			w.Header().Set(headerRetryAfter, strconv.Itoa(retryAfterSeconds))
			// Emit quota violation audit event.
			if emitter != nil {
				emitter.EmitQuotaViolation(r.Context(), tenantID, "request_rate", current, int64(limitPerMin), getActorID(r), getIPAddress(r))
			}
			httputil.WriteError(w, http.StatusTooManyRequests, "tenant request rate quota exceeded")
			return
		}

		if softWarning {
			w.Header().Set(headerTenantQuotaWarning, tenantRequestRateWarningCode)
		}
	}

	if acc != nil {
		acc.Record(tenantID, tenant.ResourceTypeRequestRate, 1)
	}
	next.ServeHTTP(w, r)
}

func effectiveRateLimitPerMinute(rpsHard, rpsSoft *int) int {
	switch {
	case rpsHard != nil:
		return *rpsHard * 60
	case rpsSoft != nil:
		return *rpsSoft * 60
	default:
		return 0
	}
}

// tenantWSAdmission performs pre-upgrade per-tenant websocket quota checks.
func tenantWSAdmission(counter *tenant.TenantConnCounter, quotaReader tenantQuotaReader, acc *tenant.UsageAccumulator, wsHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enforceTenantWSAdmission(w, r, wsHandler, counter, quotaReader, acc, nil, nil)
	})
}

func (s *Server) tenantWSAdmissionDynamic(wsHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s == nil {
			wsHandler.ServeHTTP(w, r)
			return
		}
		enforceTenantWSAdmission(w, r, wsHandler, s.tenantConnCounter, s.tenantQuotaReader, s.usageAccumulator, s.auditEmitter, s.tenantMetrics)
	})
}

// enforceTenantWSAdmission enforces tenant realtime connection quotas during WebSocket upgrade, checking against hard and soft limits and returning HTTP 429 if the hard limit is exceeded. It records quota utilization and violations, emits audit events, and releases the connection count when the handler completes.
func enforceTenantWSAdmission(w http.ResponseWriter, r *http.Request, wsHandler http.Handler, counter *tenant.TenantConnCounter, quotaReader tenantQuotaReader, acc *tenant.UsageAccumulator, emitter *tenant.AuditEmitter, tenantMetrics tenantMetricsRecorder) {
	if counter == nil || quotaReader == nil {
		wsHandler.ServeHTTP(w, r)
		return
	}

	tenantID := tenant.TenantFromContext(r.Context())
	if tenantID == "" {
		wsHandler.ServeHTTP(w, r)
		return
	}

	quotas, err := quotaReader.GetQuotas(r.Context(), tenantID)
	if err != nil {
		slog.Default().Error("loading tenant quotas failed", "tenant_id", tenantID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load tenant quotas")
		return
	}
	if quotas == nil {
		wsHandler.ServeHTTP(w, r)
		return
	}

	allowed, softWarning, count := counter.Admit(tenantID, quotas.RealtimeConnectionsHard, quotas.RealtimeConnectionsSoft)
	limit := effectiveConnLimit(quotas.RealtimeConnectionsHard, quotas.RealtimeConnectionsSoft)
	if tenantMetrics != nil && limit > 0 {
		tenantMetrics.RecordQuotaUtilization(r.Context(), tenantID, string(tenant.ResourceTypeRealtimeConns), count, int64(limit))
	}
	if !allowed {
		if tenantMetrics != nil {
			tenantMetrics.IncrQuotaViolation(r.Context(), tenantID, string(tenant.ResourceTypeRealtimeConns))
		}
		if emitter != nil {
			emitter.EmitQuotaViolation(r.Context(), tenantID, "realtime_connections", count, int64(limit), getActorID(r), getIPAddress(r))
		}
		httputil.WriteError(w, http.StatusTooManyRequests, "tenant websocket quota exceeded")
		return
	}
	if softWarning {
		slog.Default().Warn("tenant realtime connection soft threshold reached", "tenant_id", tenantID)
	}
	if acc != nil {
		acc.RecordPeak(tenantID, tenant.ResourceTypeRealtimeConns, count)
	}

	defer counter.Release(tenantID)
	wsHandler.ServeHTTP(w, r)
}

func effectiveConnLimit(hard, soft *int) int {
	switch {
	case hard != nil:
		return *hard
	case soft != nil:
		return *soft
	default:
		return 0
	}
}

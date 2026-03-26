// Package observability Provides HTTP middleware for recording tenant-aware request metrics.
package observability

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"

	"github.com/allyourbase/ayb/internal/tenant"
)

// TenantContextMiddleware returns an HTTP middleware that records request metrics with tenant context. It tracks request duration, status code, and active connections, recording them with tenant ID attributes when present. If another metrics middleware is managing connection accounting, this middleware marks the request as tenant-recorded to avoid double-counting.
func TenantContextMiddleware(httpMetrics *HTTPMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if httpMetrics == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := r.Context()
			scope := requestMetricsScopeFromContext(ctx)

			// When base HTTPMetrics middleware is present, let it own active-connection
			// accounting and mark this request as tenant-recorded to avoid double-counting.
			if scope != nil {
				scope.recordedByTenant = true
			} else {
				httpMetrics.activeConns.Add(ctx, 1)
				defer httpMetrics.activeConns.Add(ctx, -1)
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			tenantID := tenant.TenantFromContext(ctx)
			var extra []attribute.KeyValue
			if tenantID != "" {
				extra = append(extra, TenantIDAttr(tenantID))
			}
			httpMetrics.recordRequest(ctx, r, status, time.Since(start).Seconds(), extra...)
		})
	}
}

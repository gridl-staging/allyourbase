package server

import (
	"bufio"
	"errors"
	"net"
	"net/http"

	"github.com/allyourbase/ayb/internal/tenant"
)

// recordBreakerOutcome is middleware that uses HTTP response status codes to transition a circuit breaker between open and closed states, emitting audit events on state changes.
func (s *Server) recordBreakerOutcome(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant.TenantFromContext(r.Context())
		if tenantID == "" {
			next.ServeHTTP(w, r)
			return
		}

		if isTenantRecoveryEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)

		if s.tenantBreakerTracker != nil {
			if rec.statusCode >= 500 {
				prevState, newState := s.tenantBreakerTracker.RecordFailure(tenantID)
				if prevState != tenant.BreakerStateOpen && newState == tenant.BreakerStateOpen && s.auditEmitter != nil {
					snap := s.tenantBreakerTracker.StateSnapshot(tenantID)
					s.auditEmitter.EmitBreakerOpened(r.Context(), tenantID, snap.ConsecutiveFailures)
				}
			} else if rec.statusCode < 400 {
				prevState, _ := s.tenantBreakerTracker.RecordSuccess(tenantID)
				if prevState != tenant.BreakerStateClosed && s.auditEmitter != nil {
					s.auditEmitter.EmitBreakerClosed(r.Context(), tenantID)
				}
			}
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying ResponseWriter for Go 1.20+ middleware chains.
func (rec *statusRecorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}

// Flush forwards to the underlying writer so SSE streaming works through
// the recordBreakerOutcome middleware. Without this, the http.Flusher type
// assertion in the realtime SSE handler fails.
func (rec *statusRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards to the underlying writer so WebSocket upgrades work through
// the recordBreakerOutcome middleware. Without this, gorilla/websocket's
// Upgrader.Upgrade fails on the http.Hijacker type assertion.
func (rec *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rec.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

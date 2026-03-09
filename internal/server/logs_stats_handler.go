package server

import (
	"net/http"
	"runtime"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
)

// handleAdminLogs returns recent server log entries.
func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	// Return log buffer entries if available, otherwise a helpful message.
	if s.logBuffer == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"entries": []any{},
			"message": "log buffering not enabled",
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"entries": s.logBuffer.Entries(),
	})
}

// handleAdminStats returns server runtime statistics.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	stats := map[string]any{
		"uptime_seconds": int(time.Since(s.startTime).Seconds()),
		"go_version":     runtime.Version(),
		"goroutines":     runtime.NumGoroutine(),
		"memory_alloc":   mem.Alloc,
		"memory_sys":     mem.Sys,
		"gc_cycles":      mem.NumGC,
	}

	if s.pool != nil {
		poolStat := s.pool.Stat()
		stats["db_pool_total"] = poolStat.TotalConns()
		stats["db_pool_idle"] = poolStat.IdleConns()
		stats["db_pool_in_use"] = poolStat.AcquiredConns()
		stats["db_pool_max"] = poolStat.MaxConns()
	}

	httputil.WriteJSON(w, http.StatusOK, stats)
}

// handleAdminSecretsRotate generates a new JWT secret, invalidating all tokens.
// Route is only registered when authSvc != nil (see server.go).
func (s *Server) handleAdminSecretsRotate(w http.ResponseWriter, r *http.Request) {
	_, err := s.authSvc.RotateJWTSecret()
	if err != nil {
		s.logger.Error("JWT secret rotation failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to rotate secret")
		return
	}

	s.logger.Info("JWT secret rotated")

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "JWT secret rotated successfully. All existing tokens have been invalidated.",
	})
}

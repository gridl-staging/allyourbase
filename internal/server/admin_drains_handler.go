// Package server admin_drains_handler provides HTTP handlers for managing logging drains. It supports creation, listing, and deletion operations with automatic drain manager initialization and logger integration.
package server

import (
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListDrains(w http.ResponseWriter, r *http.Request) {
	manager := s.currentDrainManager()
	if manager == nil {
		httputil.WriteJSON(w, http.StatusOK, []logging.DrainInfo{})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, manager.ListDrains())
}

// handleCreateDrain creates and registers a logging drain from the request body. It decodes the LogDrainConfig, instantiates the drain, and if enabled, initializes the drain manager if needed, wraps the logger for fanout, and registers the drain with configured batch size and flush interval. Disabled drains return successfully without being registered.
func (s *Server) handleCreateDrain(w http.ResponseWriter, r *http.Request) {
	var cfg config.LogDrainConfig
	if !httputil.DecodeJSON(w, r, &cfg) {
		return
	}

	cfg = normalizeLogDrainConfig(cfg, int(time.Now().UnixNano()))
	var transport http.RoundTripper
	if s.tracerProvider != nil {
		transport = observability.NewOtelHTTPTransport(s.tracerProvider)
	}
	drain, err := newLogDrainFromConfig(cfg, transport)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		httputil.WriteJSON(w, http.StatusCreated, logging.DrainInfo{
			ID:    cfg.ID,
			Name:  cfg.Type,
			Stats: logging.DrainStats{},
		})
		return
	}

	s.observabilityMu.Lock()
	if s.drainManager == nil {
		s.drainManager = logging.NewDrainManager()
		s.drainManager.Start()
	}
	if s.logger != nil {
		if _, ok := s.logger.Handler().(*drainSlogHandler); !ok {
			s.logger = wrapLoggerForDrainFanout(s.logger, s.drainManager)
		}
	}
	manager := s.drainManager
	s.observabilityMu.Unlock()

	manager.AddDrain(cfg.ID, drain, logging.DrainWorkerConfig{
		BatchSize:     cfg.BatchSize,
		FlushInterval: time.Duration(cfg.FlushIntervalSecs) * time.Second,
		QueueSize:     10000,
	})

	httputil.WriteJSON(w, http.StatusCreated, logging.DrainInfo{
		ID:    cfg.ID,
		Name:  drain.Name(),
		Stats: drain.Stats(),
	})
}

func (s *Server) handleDeleteDrain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if manager := s.currentDrainManager(); manager != nil {
		manager.RemoveDrain(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

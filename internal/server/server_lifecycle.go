// Package server Manages the lifecycle of the HTTP server and its runtime services, including startup, request handling, and graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// StartWithReady begins listening. It closes the ready channel once the
// listener is bound, then blocks serving requests.
func (s *Server) StartWithReady(ready chan<- struct{}) error {
	httpServer := s.newHTTPServer(s.cfg.Address())
	s.http = httpServer

	ln, err := net.Listen("tcp", s.cfg.Address())
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	return s.serveHTTP("server starting", s.cfg.Address(), func() { close(ready) }, func() error {
		return httpServer.Serve(ln)
	})
}

// StartTLSWithReady begins serving TLS using the provided pre-created listener.
// The caller is responsible for creating the listener with the appropriate
// tls.Config (e.g. via certmagic or a self-signed cert for tests).
// It closes the ready channel once serving begins, then blocks until shutdown.
func (s *Server) StartTLSWithReady(ln net.Listener, ready chan<- struct{}) error {
	httpServer := s.newHTTPServer("")
	s.http = httpServer
	return s.serveHTTP("server starting with TLS", ln.Addr(), func() { close(ready) }, func() error {
		return httpServer.Serve(ln)
	})
}

func (s *Server) newHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func (s *Server) serveHTTP(logMessage string, address any, signalReady func(), serve func() error) error {
	s.logger.Info(logMessage, "address", address)
	if signalReady != nil {
		signalReady()
	}
	s.startRuntimeServices(context.Background())
	if err := serve(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func (s *Server) startRuntimeServices(ctx context.Context) {
	s.startHealthChecker()
	s.startStatusChecker(ctx)
	s.startRequestLogger(ctx)
	s.startDrainManager()
	s.startStoragePoller(ctx)
}

// startStoragePoller starts a background polling goroutine that periodically queries the database for storage usage metrics and updates infrastructure metrics. It returns early if the database pool, metrics, or poller is not available.
func (s *Server) startStoragePoller(ctx context.Context) {
	if s.pool == nil || s.infraMetrics == nil || s.storagePollerCancel != nil {
		return
	}

	pollCtx, cancel := context.WithCancel(ctx)
	s.storagePollerCancel = cancel

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		poll := func() {
			var total int64
			err := s.pool.QueryRow(pollCtx, "SELECT COALESCE(SUM(bytes_used), 0) FROM _ayb_storage_usage").Scan(&total)
			if err != nil {
				s.logger.Warn("reading storage usage for metrics failed", "error", err)
				return
			}
			s.infraMetrics.SetStorageBytes(total)
		}

		poll()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				poll()
			}
		}
	}()
}

func (s *Server) startRequestLogger(ctx context.Context) {
	if s.requestLogger == nil {
		return
	}
	s.requestLogger.Start(ctx)
}

func (s *Server) startStatusChecker(ctx context.Context) {
	if s.statusChecker == nil {
		return
	}
	s.statusChecker.Start(ctx)
}

func (s *Server) startHealthChecker() {
	if s.healthChecker == nil {
		return
	}
	s.healthChecker.Start()
}

func (s *Server) startDrainManager() {
	manager := s.currentDrainManager()
	if manager == nil {
		return
	}
	manager.Start()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	timeout := time.Duration(s.cfg.Server.ShutdownTimeout) * time.Second
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.logger.Info("shutting down server", "timeout", timeout)
	if s.authRL != nil {
		s.authRL.Stop()
	}
	if s.authSensitiveRL != nil {
		s.authSensitiveRL.Stop()
	}
	if s.assistantRL != nil {
		s.assistantRL.Stop()
	}
	if s.apiRL != nil {
		s.apiRL.Stop()
	}
	if s.apiAnonRL != nil {
		s.apiAnonRL.Stop()
	}
	if s.appRL != nil {
		s.appRL.Stop()
	}
	if s.adminRL != nil {
		s.adminRL.Stop()
	}
	if s.storageCDNPurgeAllRL != nil {
		s.storageCDNPurgeAllRL.Stop()
	}
	if s.jobService != nil {
		s.jobService.Stop()
	}
	if s.webhookDispatcher != nil {
		s.webhookDispatcher.Close()
	}
	if s.storagePollerCancel != nil {
		s.storagePollerCancel()
		s.storagePollerCancel = nil
	}
	if s.healthChecker != nil {
		s.healthChecker.Stop()
	}
	if s.statusChecker != nil {
		s.statusChecker.Stop()
	}
	shutdownErr := shutdownHTTPThenMetrics(
		shutdownCtx,
		func(ctx context.Context) error {
			if s.http == nil {
				return nil
			}
			return s.http.Shutdown(ctx)
		},
		nil,
	)
	if s.requestLogger != nil {
		if err := s.requestLogger.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("request logger shutdown error", "error", err)
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if manager := s.currentDrainManager(); manager != nil {
		manager.Stop()
	}
	if s.tracerProvider != nil {
		if err := s.tracerProvider.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("tracer provider shutdown error", "error", err)
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if s.httpMetrics != nil {
		if err := s.httpMetrics.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("metrics shutdown error", "error", err)
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	// Drain the connection manager before hard-closing transports: give existing
	// connections a chance to leave gracefully, then force-close any that remain.
	if s.connManager != nil {
		drainTimeout := time.Duration(s.cfg.Server.ShutdownTimeout) * time.Second
		s.connManager.Drain(drainTimeout)
	}
	if s.poolRouter != nil {
		s.poolRouter.Close()
	}
	if s.wsHandler != nil {
		s.wsHandler.Shutdown()
	}
	s.hub.Close()
	return shutdownErr
}

func shutdownHTTPThenMetrics(ctx context.Context, shutdownHTTP func(context.Context) error, shutdownMetrics func(context.Context) error) error {
	var errs []error
	if shutdownHTTP != nil {
		if err := shutdownHTTP(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if shutdownMetrics != nil {
		if err := shutdownMetrics(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

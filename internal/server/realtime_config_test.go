package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

func newRealtimeConfigTestServer(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)
	t.Cleanup(func() {
		if srv.connManager != nil {
			srv.connManager.Stop()
		}
	})
	return srv
}

func assertPerUserLimit(t *testing.T, cm *realtime.ConnectionManager, limit int) {
	t.Helper()
	for i := 0; i < limit; i++ {
		err := cm.Register(realtime.ConnectionMeta{
			ClientID:  fmt.Sprintf("c-%d", i),
			UserID:    "user-1",
			Transport: "ws",
			CloseFunc: func() {},
		})
		if err != nil {
			t.Fatalf("register %d/%d: %v", i+1, limit, err)
		}
	}
	err := cm.Register(realtime.ConnectionMeta{
		ClientID:  fmt.Sprintf("c-%d", limit),
		UserID:    "user-1",
		Transport: "ws",
		CloseFunc: func() {},
	})
	if !errors.Is(err, realtime.ErrLimitExceeded) {
		t.Fatalf("expected ErrLimitExceeded after %d registrations, got %v", limit, err)
	}
}

func TestServerAppliesNonDefaultRealtimeConfig(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	cfg.Realtime.MaxConnectionsPerUser = 2
	cfg.Realtime.HeartbeatIntervalSeconds = 33
	cfg.Realtime.BroadcastRateLimitPerSecond = 7
	cfg.Realtime.BroadcastMaxMessageBytes = 8192
	cfg.Realtime.PresenceLeaveTimeoutSeconds = 4

	srv := newRealtimeConfigTestServer(t, cfg)

	if got := srv.wsHandler.PingInterval; got != 33*time.Second {
		t.Fatalf("PingInterval=%v, want %v", got, 33*time.Second)
	}
	if got := srv.wsHandler.Broadcast.RateLimit; got != 7 {
		t.Fatalf("Broadcast.RateLimit=%d, want 7", got)
	}
	if got := srv.wsHandler.Broadcast.MaxPayloadBytes; got != 8192 {
		t.Fatalf("Broadcast.MaxPayloadBytes=%d, want 8192", got)
	}
	if got := srv.wsHandler.Presence.LeaveTimeout; got != 4*time.Second {
		t.Fatalf("Presence.LeaveTimeout=%v, want %v", got, 4*time.Second)
	}

	assertPerUserLimit(t, srv.connManager, 2)
}

func TestServerUsesDefaultRealtimeConfigBehavior(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	srv := newRealtimeConfigTestServer(t, cfg)

	if got := srv.wsHandler.PingInterval; got != 25*time.Second {
		t.Fatalf("PingInterval=%v, want %v", got, 25*time.Second)
	}
	if got := srv.wsHandler.Broadcast.RateLimit; got != 100 {
		t.Fatalf("Broadcast.RateLimit=%d, want 100", got)
	}
	if got := srv.wsHandler.Broadcast.MaxPayloadBytes; got != 262144 {
		t.Fatalf("Broadcast.MaxPayloadBytes=%d, want 262144", got)
	}
	if got := srv.wsHandler.Presence.LeaveTimeout; got != 10*time.Second {
		t.Fatalf("Presence.LeaveTimeout=%v, want %v", got, 10*time.Second)
	}

	assertPerUserLimit(t, srv.connManager, 100)
}

func TestRegisterAPIGraphQLRoutesWithoutAuthMountsGetAndPost(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)
	srv.graphqlHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	r := chi.NewRouter()
	srv.registerAPIGraphQLRoutes(r)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/graphql", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusAccepted, w.Code)
	}
}

func TestRegisterAPIGraphQLRoutesWithAuthProtectsPostOnly(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-characters-long", time.Hour, 7*24*time.Hour, 8, logger)
	srv := newServer(cfg, logger, ch, nil, authSvc, nil, nil)
	srv.graphqlHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	r := chi.NewRouter()
	srv.registerAPIGraphQLRoutes(r)

	unauthPostReq := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	unauthPostRes := httptest.NewRecorder()
	r.ServeHTTP(unauthPostRes, unauthPostReq)
	testutil.Equal(t, http.StatusUnauthorized, unauthPostRes.Code)

	unauthGetReq := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	unauthGetRes := httptest.NewRecorder()
	r.ServeHTTP(unauthGetRes, unauthGetReq)
	testutil.Equal(t, http.StatusAccepted, unauthGetRes.Code)

	jwt, err := authSvc.IssueTestToken("user-1", "test@example.com")
	testutil.NoError(t, err)
	authPostReq := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	authPostReq.Header.Set("Authorization", "Bearer "+jwt)
	authPostRes := httptest.NewRecorder()
	r.ServeHTTP(authPostRes, authPostReq)
	testutil.Equal(t, http.StatusAccepted, authPostRes.Code)
}

func TestMountAuthRouteGroupAllowsFormContentTypeForToken(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-characters-long", time.Hour, 7*24*time.Hour, 8, logger)
	srv := newServer(cfg, logger, ch, nil, authSvc, nil, nil)
	authHandler := auth.NewHandler(authSvc, logger)
	srv.authRL = auth.NewRateLimiter(10, time.Minute)
	srv.authSensitiveRL = auth.NewRateLimiter(10, time.Minute)
	t.Cleanup(func() {
		srv.authRL.Stop()
		srv.authSensitiveRL.Stop()
	})

	r := chi.NewRouter()
	srv.mountAuthRouteGroup(r, authHandler)

	req := httptest.NewRequest(http.MethodPost, "/auth/token", strings.NewReader("grant_type=password"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	testutil.Equal(t, http.StatusBadRequest, res.Code)
}

func TestMountAuthRouteGroupAppliesRateLimiter(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-characters-long", time.Hour, 7*24*time.Hour, 8, logger)
	srv := newServer(cfg, logger, ch, nil, authSvc, nil, nil)
	authHandler := auth.NewHandler(authSvc, logger)
	srv.authRL = auth.NewRateLimiter(1, time.Minute)
	srv.authSensitiveRL = auth.NewRateLimiter(1, time.Minute)
	t.Cleanup(func() {
		srv.authRL.Stop()
		srv.authSensitiveRL.Stop()
	})

	r := chi.NewRouter()
	srv.mountAuthRouteGroup(r, authHandler)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refreshToken":""}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.24:3333"
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if i == 0 {
			testutil.True(t, res.Code != http.StatusTooManyRequests, "first request should not be rate limited")
			continue
		}
		testutil.Equal(t, http.StatusTooManyRequests, res.Code)
		testutil.Equal(t, "1", res.Header().Get("X-RateLimit-Limit"))
	}
}

func TestRegisterAdminAnalyticsRoutesRequiresAdminAuth(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)

	r := chi.NewRouter()
	srv.registerAdminAnalyticsRoutes(r)

	for _, path := range []string{"/admin/analytics/requests", "/admin/analytics/queries"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		testutil.Equal(t, http.StatusUnauthorized, res.Code)
	}
}

func TestRegisterAdminAnalyticsRoutesRequestsHandlerMounted(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)

	r := chi.NewRouter()
	srv.registerAdminAnalyticsRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/requests", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminAuth.token())
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	testutil.Equal(t, http.StatusServiceUnavailable, res.Code)
}

func TestRegisterAdminAuthConfigRoutesWithoutAuthServiceSkipsHooksAndSAML(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)

	r := chi.NewRouter()
	srv.registerAdminAuthConfigRoutes(r)

	for _, tc := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/admin/auth-settings", wantStatus: http.StatusUnauthorized},
		{path: "/admin/auth/providers", wantStatus: http.StatusUnauthorized},
		{path: "/admin/auth/hooks", wantStatus: http.StatusNotFound},
		{path: "/admin/auth/saml", wantStatus: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		testutil.Equal(t, tc.wantStatus, res.Code)
	}
}

func TestRegisterAdminAuthConfigRoutesWithAuthServiceMountsHooksAndSAML(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-characters-long", time.Hour, 7*24*time.Hour, 8, logger)
	srv := newServer(cfg, logger, ch, nil, authSvc, nil, nil)

	r := chi.NewRouter()
	srv.registerAdminAuthConfigRoutes(r)

	for _, path := range []string{"/admin/auth/hooks", "/admin/auth/saml"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		testutil.Equal(t, http.StatusUnauthorized, res.Code)
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

type serverCDNProviderStub struct {
	purgeURLsCalls chan []string
	purgeAllCalls  chan struct{}
}

func (p *serverCDNProviderStub) Name() string { return "stub" }

func (p *serverCDNProviderStub) PurgeURLs(_ context.Context, publicURLs []string) error {
	copied := make([]string, len(publicURLs))
	copy(copied, publicURLs)
	p.purgeURLsCalls <- copied
	return nil
}

func (p *serverCDNProviderStub) PurgeAll(_ context.Context) error {
	p.purgeAllCalls <- struct{}{}
	return nil
}

func newServerForStorageCDNTests(t *testing.T) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	cfg.Storage.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	backend, err := storage.NewLocalBackend(t.TempDir())
	testutil.NoError(t, err)
	storageSvc := storage.NewService(nil, backend, "sign-key-for-test", logger, 0)

	return New(cfg, logger, ch, nil, nil, storageSvc)
}

func adminBearerToken(t *testing.T, srv *Server) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", bytes.NewBufferString(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var payload map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	token := payload["token"]
	testutil.True(t, token != "", "admin token should not be empty")
	return token
}

func TestStorageCDNProviderBuiltOnceAndReused(t *testing.T) {
	provider := &serverCDNProviderStub{
		purgeURLsCalls: make(chan []string, 4),
		purgeAllCalls:  make(chan struct{}, 4),
	}

	var builderCalls int32
	prevBuilder := newStorageCDNProvider
	newStorageCDNProvider = func(_ config.CDNConfig, _ *slog.Logger) storage.CDNProvider {
		atomic.AddInt32(&builderCalls, 1)
		return provider
	}
	t.Cleanup(func() {
		newStorageCDNProvider = prevBuilder
	})

	srv := newServerForStorageCDNTests(t)
	testutil.Equal(t, int32(1), atomic.LoadInt32(&builderCalls))

	token := adminBearerToken(t, srv)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/storage/cdn/purge", bytes.NewBufferString(`{"urls":["https://cdn.example.com/a"]}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusAccepted, w.Code)
	}

	for i := 0; i < 2; i++ {
		select {
		case <-provider.purgeURLsCalls:
		case <-time.After(time.Second):
			t.Fatal("expected targeted purge call")
		}
	}
	testutil.Equal(t, int32(1), atomic.LoadInt32(&builderCalls))
}

func TestStorageCDNProviderEmptyConfigUsesNop(t *testing.T) {
	t.Parallel()
	srv := newServerForStorageCDNTests(t)
	testutil.Equal(t, "nop", srv.storageHandler.CDNProviderName())
}

func TestAdminStorageCDNPurgeRequiresAdminAuth(t *testing.T) {
	t.Parallel()
	srv := newServerForStorageCDNTests(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/storage/cdn/purge", bytes.NewBufferString(`{"urls":["https://cdn.example.com/a"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminStorageCDNPurgeValidation(t *testing.T) {
	t.Parallel()
	srv := newServerForStorageCDNTests(t)
	token := adminBearerToken(t, srv)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing mode", body: `{}`},
		{name: "both modes", body: `{"urls":["https://cdn.example.com/a"],"purge_all":true}`},
		{name: "empty urls", body: `{"urls":["  ",""]}`},
		{name: "invalid url", body: `{"urls":["/relative/path"]}`},
		{name: "invalid scheme", body: `{"urls":["ftp://cdn.example.com/a"]}`},
		{name: "unknown field", body: `{"urls":["https://cdn.example.com/a"],"provider_path":"/bad"}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/admin/storage/cdn/purge", bytes.NewBufferString(tt.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			srv.Router().ServeHTTP(w, req)
			testutil.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestAdminStorageCDNPurgeDispatchAndRateLimit(t *testing.T) {
	provider := &serverCDNProviderStub{
		purgeURLsCalls: make(chan []string, 4),
		purgeAllCalls:  make(chan struct{}, 4),
	}

	prevBuilder := newStorageCDNProvider
	newStorageCDNProvider = func(_ config.CDNConfig, _ *slog.Logger) storage.CDNProvider {
		return provider
	}
	t.Cleanup(func() {
		newStorageCDNProvider = prevBuilder
	})

	srv := newServerForStorageCDNTests(t)
	token := adminBearerToken(t, srv)

	// Targeted purge should dispatch PurgeURLs with unchanged public URLs.
	targetedBody := `{"urls":["https://cdn.example.com/a?download=1","https://cdn.example.com/b"]}`
	targetedW := httptest.NewRecorder()
	targetedReq := httptest.NewRequest(http.MethodPost, "/api/admin/storage/cdn/purge", bytes.NewBufferString(targetedBody))
	targetedReq.Header.Set("Authorization", "Bearer "+token)
	targetedReq.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(targetedW, targetedReq)
	testutil.Equal(t, http.StatusAccepted, targetedW.Code)

	select {
	case urls := <-provider.purgeURLsCalls:
		testutil.SliceLen(t, urls, 2)
		testutil.Equal(t, "https://cdn.example.com/a?download=1", urls[0])
		testutil.Equal(t, "https://cdn.example.com/b", urls[1])
	case <-time.After(time.Second):
		t.Fatal("expected PurgeURLs dispatch")
	}

	// First purge_all is accepted and dispatched.
	purgeAllW := httptest.NewRecorder()
	purgeAllReq := httptest.NewRequest(http.MethodPost, "/api/admin/storage/cdn/purge", bytes.NewBufferString(`{"purge_all":true}`))
	purgeAllReq.Header.Set("Authorization", "Bearer "+token)
	purgeAllReq.Header.Set("Content-Type", "application/json")
	purgeAllReq.RemoteAddr = "198.51.100.40:1234"
	srv.Router().ServeHTTP(purgeAllW, purgeAllReq)
	testutil.Equal(t, http.StatusAccepted, purgeAllW.Code)

	select {
	case <-provider.purgeAllCalls:
	case <-time.After(time.Second):
		t.Fatal("expected PurgeAll dispatch")
	}

	// Second purge_all from same IP should be rate-limited.
	secondPurgeAllW := httptest.NewRecorder()
	secondPurgeAllReq := httptest.NewRequest(http.MethodPost, "/api/admin/storage/cdn/purge", bytes.NewBufferString(`{"purge_all":true}`))
	secondPurgeAllReq.Header.Set("Authorization", "Bearer "+token)
	secondPurgeAllReq.Header.Set("Content-Type", "application/json")
	secondPurgeAllReq.RemoteAddr = "198.51.100.40:1234"
	srv.Router().ServeHTTP(secondPurgeAllW, secondPurgeAllReq)
	testutil.Equal(t, http.StatusTooManyRequests, secondPurgeAllW.Code)

	// Targeted purges should remain available despite purge_all limiter.
	targetedAgainW := httptest.NewRecorder()
	targetedAgainReq := httptest.NewRequest(http.MethodPost, "/api/admin/storage/cdn/purge", bytes.NewBufferString(`{"urls":["https://cdn.example.com/c"]}`))
	targetedAgainReq.Header.Set("Authorization", "Bearer "+token)
	targetedAgainReq.Header.Set("Content-Type", "application/json")
	targetedAgainReq.RemoteAddr = "198.51.100.40:1234"
	srv.Router().ServeHTTP(targetedAgainW, targetedAgainReq)
	testutil.Equal(t, http.StatusAccepted, targetedAgainW.Code)
}

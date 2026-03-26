package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type fakeHookEdgeFuncAdmin struct {
	invokeResp edgefunc.Response
	invokeErr  error
	lastName   string
	lastReq    edgefunc.Request
	writer     edgefunc.InvocationLogWriter
}

func (f *fakeHookEdgeFuncAdmin) Deploy(_ context.Context, _ string, _ string, _ edgefunc.DeployOptions) (*edgefunc.EdgeFunction, error) {
	return nil, nil
}

func (f *fakeHookEdgeFuncAdmin) Get(_ context.Context, _ uuid.UUID) (*edgefunc.EdgeFunction, error) {
	return nil, nil
}

func (f *fakeHookEdgeFuncAdmin) GetByName(_ context.Context, _ string) (*edgefunc.EdgeFunction, error) {
	return nil, nil
}

func (f *fakeHookEdgeFuncAdmin) List(_ context.Context, _, _ int) ([]*edgefunc.EdgeFunction, error) {
	return nil, nil
}

func (f *fakeHookEdgeFuncAdmin) Update(_ context.Context, _ uuid.UUID, _ string, _ edgefunc.DeployOptions) (*edgefunc.EdgeFunction, error) {
	return nil, nil
}

func (f *fakeHookEdgeFuncAdmin) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (f *fakeHookEdgeFuncAdmin) Invoke(_ context.Context, name string, req edgefunc.Request) (edgefunc.Response, error) {
	f.lastName = name
	f.lastReq = req
	if f.invokeErr != nil {
		return edgefunc.Response{}, f.invokeErr
	}
	return f.invokeResp, nil
}

func (f *fakeHookEdgeFuncAdmin) ListLogs(_ context.Context, _ uuid.UUID, _ edgefunc.LogListOptions) ([]*edgefunc.LogEntry, error) {
	return nil, nil
}

func (f *fakeHookEdgeFuncAdmin) SetInvocationLogWriter(writer edgefunc.InvocationLogWriter) {
	f.writer = writer
}

func parseClaims(t *testing.T, token, secret string) *auth.Claims {
	t.Helper()
	claims := &auth.Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	testutil.NoError(t, err)
	return claims
}

func TestSetEdgeFuncService_WiresAuthHookDispatcher(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-that-is-at-least-32-chars-long"
	cfg.Auth.Hooks.CustomAccessToken = "custom-token-hook"

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, time.Hour, 24*time.Hour, 8, logger)
	srv := New(cfg, logger, ch, nil, authSvc, nil)

	beforeToken, err := authSvc.IssueTestToken("user-1", "user@example.com")
	testutil.NoError(t, err)
	beforeClaims := parseClaims(t, beforeToken, cfg.Auth.JWTSecret)
	testutil.Nil(t, beforeClaims.CustomClaims)

	payload, err := json.Marshal(map[string]any{
		"custom_claims": map[string]any{"role": "admin"},
	})
	testutil.NoError(t, err)
	fake := &fakeHookEdgeFuncAdmin{
		invokeResp: edgefunc.Response{StatusCode: 200, Body: payload},
	}

	srv.SetEdgeFuncService(fake)

	afterToken, err := authSvc.IssueTestToken("user-1", "user@example.com")
	testutil.NoError(t, err)
	afterClaims := parseClaims(t, afterToken, cfg.Auth.JWTSecret)

	testutil.Equal(t, "custom-token-hook", fake.lastName)
	testutil.Equal(t, "admin", afterClaims.CustomClaims["role"])
}

func TestSetEdgeFuncService_WiresInvocationLogWriter(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Logging.Drains = []config.LogDrainConfig{{
		ID:      "drain-a",
		Type:    "http",
		URL:     "http://localhost:9999/mock",
		Headers: map[string]string{},
	}}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(nil, logger)
	srv := New(cfg, logger, ch, nil, nil, nil)

	fake := &fakeHookEdgeFuncAdmin{}
	srv.SetEdgeFuncService(fake)

	testutil.True(t, fake.writer != nil, "expected SetInvocationLogWriter to be called")
	_, ok := fake.writer.(*edgeFuncDrainWriter)
	testutil.True(t, ok, "expected edge function drain writer to be wired")
}

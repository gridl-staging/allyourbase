package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockHookInvoker is a HookInvoker that records calls and returns preset responses.
type mockHookInvoker struct {
	responses map[string]hookInvokerResponse
	calls     []hookInvokerCall
	onInvoke  func() // optional callback fired on each invocation
}

type hookInvokerResponse struct {
	body []byte
	err  error
}

type hookInvokerCall struct {
	name    string
	payload []byte
}

func newMockHookInvoker() *mockHookInvoker {
	return &mockHookInvoker{responses: make(map[string]hookInvokerResponse)}
}

func (m *mockHookInvoker) InvokeHook(_ context.Context, name string, payload []byte) ([]byte, error) {
	m.calls = append(m.calls, hookInvokerCall{name: name, payload: payload})
	if m.onInvoke != nil {
		m.onInvoke()
	}
	if r, ok := m.responses[name]; ok {
		return r.body, r.err
	}
	return nil, errors.New("function not found: " + name)
}

// setJSON registers a preset JSON response for a named function.
func (m *mockHookInvoker) setJSON(name string, v any) {
	b, _ := json.Marshal(v)
	m.responses[name] = hookInvokerResponse{body: b}
}

// setError registers an error for a named function.
func (m *mockHookInvoker) setError(name string, err error) {
	m.responses[name] = hookInvokerResponse{err: err}
}

// ---------------------------------------------------------------------------
// HookDispatcher tests
// ---------------------------------------------------------------------------

func newTestDispatcher(hooks config.AuthHooks, inv HookInvoker) *HookDispatcher {
	return NewHookDispatcher(hooks, inv, slog.Default())
}

// --- BeforeSignUp ---

func TestHookDispatcher_BeforeSignUp_Allow(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("before-signup", map[string]any{"allow": true})

	d := newTestDispatcher(config.AuthHooks{BeforeSignUp: "before-signup"}, inv)

	err := d.BeforeSignUp(context.Background(), "user@example.com", nil, "127.0.0.1")
	if err != nil {
		t.Fatalf("expected nil error for allow=true, got: %v", err)
	}
	if len(inv.calls) != 1 || inv.calls[0].name != "before-signup" {
		t.Fatalf("expected one call to before-signup")
	}
}

func TestHookDispatcher_BeforeSignUp_Reject(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("before-signup", map[string]any{"allow": false, "error": "blocked domain"})

	d := newTestDispatcher(config.AuthHooks{BeforeSignUp: "before-signup"}, inv)

	err := d.BeforeSignUp(context.Background(), "user@blocked.com", nil, "127.0.0.1")
	if err == nil {
		t.Fatal("expected error for allow=false, got nil")
	}
	if !errors.Is(err, ErrHookRejected) {
		t.Fatalf("expected ErrHookRejected, got: %v", err)
	}
}

func TestHookDispatcher_BeforeSignUp_NoHook(t *testing.T) {
	inv := newMockHookInvoker()
	d := newTestDispatcher(config.AuthHooks{}, inv) // no hook configured

	err := d.BeforeSignUp(context.Background(), "user@example.com", nil, "127.0.0.1")
	if err != nil {
		t.Fatalf("expected nil when no hook configured, got: %v", err)
	}
	if len(inv.calls) != 0 {
		t.Fatal("expected no invocations when hook not configured")
	}
}

func TestHookDispatcher_BeforeSignUp_Timeout(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("before-signup", context.DeadlineExceeded)

	d := newTestDispatcher(config.AuthHooks{BeforeSignUp: "before-signup"}, inv)

	err := d.BeforeSignUp(context.Background(), "user@example.com", nil, "127.0.0.1")
	if err == nil {
		t.Fatal("expected error on timeout, got nil")
	}
}

func TestHookDispatcher_BeforeSignUp_FunctionNotFound(t *testing.T) {
	inv := newMockHookInvoker()
	// no response set → default "function not found" error

	d := newTestDispatcher(config.AuthHooks{BeforeSignUp: "nonexistent-fn"}, inv)

	err := d.BeforeSignUp(context.Background(), "user@example.com", nil, "127.0.0.1")
	if err == nil {
		t.Fatal("expected error when function not found, got nil")
	}
}

// --- AfterSignUp ---

func TestHookDispatcher_AfterSignUp_AsyncFailureDoesNotBlock(t *testing.T) {
	invoked := make(chan struct{}, 1)
	inv := newMockHookInvoker()
	inv.setError("after-signup", errors.New("edge function crashed"))
	inv.onInvoke = func() { invoked <- struct{}{} }

	d := newTestDispatcher(config.AuthHooks{AfterSignUp: "after-signup"}, inv)

	// AfterSignUp returns immediately (fire-and-forget).
	d.AfterSignUp(context.Background(), "user-id-123", "user@example.com", nil)

	// Wait for the async goroutine to actually invoke the hook.
	select {
	case <-invoked:
		// Hook was called — verify it was the right function.
		if len(inv.calls) != 1 || inv.calls[0].name != "after-signup" {
			t.Fatalf("expected one call to after-signup, got %d calls", len(inv.calls))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AfterSignUp async goroutine never invoked the hook")
	}
}

func TestHookDispatcher_AfterSignUp_NoHook(t *testing.T) {
	inv := newMockHookInvoker()
	d := newTestDispatcher(config.AuthHooks{}, inv)

	d.AfterSignUp(context.Background(), "uid", "e@x.com", nil)
	if len(inv.calls) != 0 {
		t.Fatal("expected no invocations when hook not configured")
	}
}

// --- CustomAccessToken ---

func TestHookDispatcher_CustomAccessToken_AddsClaims(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("custom-token", map[string]any{
		"custom_claims": map[string]any{
			"role": "admin",
			"org":  "acme",
		},
	})

	d := newTestDispatcher(config.AuthHooks{CustomAccessToken: "custom-token"}, inv)

	claims := map[string]any{
		"sub": "user-123",
		"exp": float64(9999999999),
		"iat": float64(1000000000),
	}
	result, err := d.CustomAccessToken(context.Background(), "user-123", claims)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	cc, ok := result["custom_claims"].(map[string]any)
	if !ok {
		t.Fatal("expected custom_claims key in result")
	}
	if cc["role"] != "admin" {
		t.Fatalf("expected role=admin, got: %v", cc["role"])
	}
}

func TestHookDispatcher_CustomAccessToken_CannotOverwriteProtectedClaims(t *testing.T) {
	inv := newMockHookInvoker()
	// Hook tries to overwrite protected claims
	inv.setJSON("custom-token", map[string]any{
		"sub":           "evil-user",
		"exp":           float64(1), // tries to expire immediately
		"iat":           float64(1),
		"iss":           "evil-issuer",
		"aal":           "aal0",
		"amr":           []any{"evil"},
		"custom_claims": map[string]any{"legit": "value"},
	})

	d := newTestDispatcher(config.AuthHooks{CustomAccessToken: "custom-token"}, inv)

	originalClaims := map[string]any{
		"sub": "user-123",
		"exp": float64(9999999999),
		"iat": float64(1000000000),
		"iss": "ayb",
		"aal": "aal1",
		"amr": []any{"password"},
	}
	result, err := d.CustomAccessToken(context.Background(), "user-123", originalClaims)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Protected claims must NOT be overwritten
	if result["sub"] != "user-123" {
		t.Fatalf("sub was overwritten: %v", result["sub"])
	}
	if result["exp"] != float64(9999999999) {
		t.Fatalf("exp was overwritten: %v", result["exp"])
	}
	if result["iat"] != float64(1000000000) {
		t.Fatalf("iat was overwritten: %v", result["iat"])
	}
	if result["iss"] != "ayb" {
		t.Fatalf("iss was overwritten: %v", result["iss"])
	}
	// Custom claims must be present
	if _, ok := result["custom_claims"]; !ok {
		t.Fatal("custom_claims missing from result")
	}
}

func TestHookDispatcher_CustomAccessToken_CannotOverwriteJTIAndNBF(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("custom-token", map[string]any{
		"jti":           "malicious-jti",
		"nbf":           float64(1),
		"custom_claims": map[string]any{"ok": true},
	})

	d := newTestDispatcher(config.AuthHooks{CustomAccessToken: "custom-token"}, inv)

	claims := map[string]any{
		"sub": "user-123",
		"jti": "trusted-jti",
		"nbf": float64(2000000000),
	}
	result, err := d.CustomAccessToken(context.Background(), "user-123", claims)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if result["jti"] != "trusted-jti" {
		t.Fatalf("jti was overwritten: %v", result["jti"])
	}
	if result["nbf"] != float64(2000000000) {
		t.Fatalf("nbf was overwritten: %v", result["nbf"])
	}
}

func TestHookDispatcher_CustomAccessToken_NoHook(t *testing.T) {
	inv := newMockHookInvoker()
	d := newTestDispatcher(config.AuthHooks{}, inv)

	claims := map[string]any{"sub": "u1", "exp": float64(9999)}
	result, err := d.CustomAccessToken(context.Background(), "u1", claims)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result["sub"] != "u1" {
		t.Fatal("expected original claims returned when no hook configured")
	}
	if len(inv.calls) != 0 {
		t.Fatal("expected no invocations when hook not configured")
	}
}

func TestHookDispatcher_CustomAccessToken_Timeout(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("custom-token", context.DeadlineExceeded)

	d := newTestDispatcher(config.AuthHooks{CustomAccessToken: "custom-token"}, inv)

	_, err := d.CustomAccessToken(context.Background(), "u1", map[string]any{"sub": "u1"})
	if err == nil {
		t.Fatal("expected error on timeout")
	}
}

// --- BeforePasswordReset ---

func TestHookDispatcher_BeforePasswordReset_Allow(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("before-reset", map[string]any{"allow": true})

	d := newTestDispatcher(config.AuthHooks{BeforePasswordReset: "before-reset"}, inv)

	err := d.BeforePasswordReset(context.Background(), "user@example.com", "10.0.0.1")
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestHookDispatcher_BeforePasswordReset_Reject(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("before-reset", map[string]any{"allow": false})

	d := newTestDispatcher(config.AuthHooks{BeforePasswordReset: "before-reset"}, inv)

	err := d.BeforePasswordReset(context.Background(), "user@example.com", "10.0.0.1")
	if !errors.Is(err, ErrHookRejected) {
		t.Fatalf("expected ErrHookRejected, got: %v", err)
	}
}

func TestHookDispatcher_BeforePasswordReset_NoHook(t *testing.T) {
	inv := newMockHookInvoker()
	d := newTestDispatcher(config.AuthHooks{}, inv)

	err := d.BeforePasswordReset(context.Background(), "user@example.com", "10.0.0.1")
	if err != nil {
		t.Fatalf("expected nil when no hook configured, got: %v", err)
	}
}

// --- SendEmail ---

func TestHookDispatcher_SendEmail_ReplacesMailer(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("send-email", map[string]any{"sent": true})

	d := newTestDispatcher(config.AuthHooks{SendEmail: "send-email"}, inv)

	err := d.SendEmail(context.Background(), "to@example.com", "Subject", "<p>Hi</p>", "Hi", "signup")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if len(inv.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(inv.calls))
	}

	var payload map[string]any
	if err := json.Unmarshal(inv.calls[0].payload, &payload); err != nil {
		t.Fatalf("could not unmarshal hook payload: %v", err)
	}
	if payload["to"] != "to@example.com" {
		t.Fatalf("expected to=to@example.com, got %v", payload["to"])
	}
	if payload["type"] != "signup" {
		t.Fatalf("expected type=signup, got %v", payload["type"])
	}
}

func TestHookDispatcher_SendEmail_NoHook(t *testing.T) {
	inv := newMockHookInvoker()
	d := newTestDispatcher(config.AuthHooks{}, inv)

	err := d.SendEmail(context.Background(), "to@example.com", "Sub", "<p>", "text", "recovery")
	if !errors.Is(err, ErrHookNotConfigured) {
		t.Fatalf("expected ErrHookNotConfigured, got: %v", err)
	}
}

func TestHookDispatcher_SendEmail_Timeout(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("send-email", context.DeadlineExceeded)

	d := newTestDispatcher(config.AuthHooks{SendEmail: "send-email"}, inv)

	err := d.SendEmail(context.Background(), "to@example.com", "Sub", "<p>", "text", "recovery")
	if err == nil {
		t.Fatal("expected error on timeout")
	}
}

// --- SendSMS ---

func TestHookDispatcher_SendSMS_ReplacesProvider(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("send-sms", map[string]any{"sent": true})

	d := newTestDispatcher(config.AuthHooks{SendSMS: "send-sms"}, inv)

	err := d.SendSMS(context.Background(), "+15551234567", "Your code is: 123456", "sms_signup")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if len(inv.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(inv.calls))
	}

	var payload map[string]any
	if err := json.Unmarshal(inv.calls[0].payload, &payload); err != nil {
		t.Fatalf("could not unmarshal hook payload: %v", err)
	}
	if payload["phone"] != "+15551234567" {
		t.Fatalf("expected phone=+15551234567, got %v", payload["phone"])
	}
	if payload["type"] != "sms_signup" {
		t.Fatalf("expected type=sms_signup, got %v", payload["type"])
	}
}

func TestHookDispatcher_SendSMS_NoHook(t *testing.T) {
	inv := newMockHookInvoker()
	d := newTestDispatcher(config.AuthHooks{}, inv)

	err := d.SendSMS(context.Background(), "+15551234567", "msg", "sms_mfa")
	if !errors.Is(err, ErrHookNotConfigured) {
		t.Fatalf("expected ErrHookNotConfigured, got: %v", err)
	}
}

// --- Nil invoker safety ---

func TestHookDispatcher_NilInvoker_BeforeSignUp(t *testing.T) {
	hooks := config.AuthHooks{BeforeSignUp: "some-function"}
	d := NewHookDispatcher(hooks, nil, slog.Default())

	err := d.BeforeSignUp(context.Background(), "user@example.com", nil, "127.0.0.1")
	if err != nil {
		t.Fatalf("expected nil (no-op) with nil invoker, got: %v", err)
	}
}

func TestHookDispatcher_NilInvoker_CustomAccessToken(t *testing.T) {
	hooks := config.AuthHooks{CustomAccessToken: "some-function"}
	d := NewHookDispatcher(hooks, nil, slog.Default())

	claims := map[string]any{"sub": "u1"}
	result, err := d.CustomAccessToken(context.Background(), "u1", claims)
	if err != nil {
		t.Fatalf("expected nil error with nil invoker, got: %v", err)
	}
	if result["sub"] != "u1" {
		t.Fatal("expected original claims returned with nil invoker")
	}
}

func TestHookDispatcher_NilInvoker_SendEmail(t *testing.T) {
	hooks := config.AuthHooks{SendEmail: "some-function"}
	d := NewHookDispatcher(hooks, nil, slog.Default())

	err := d.SendEmail(context.Background(), "to@example.com", "Sub", "<p>", "text", "recovery")
	if !errors.Is(err, ErrHookNotConfigured) {
		t.Fatalf("expected ErrHookNotConfigured with nil invoker, got: %v", err)
	}
}

func TestHookDispatcher_NilInvoker_SendSMS(t *testing.T) {
	hooks := config.AuthHooks{SendSMS: "some-function"}
	d := NewHookDispatcher(hooks, nil, slog.Default())

	err := d.SendSMS(context.Background(), "+15551234567", "msg", "sms_mfa")
	if !errors.Is(err, ErrHookNotConfigured) {
		t.Fatalf("expected ErrHookNotConfigured with nil invoker, got: %v", err)
	}
}

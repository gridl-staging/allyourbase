package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/golang-jwt/jwt/v5"
)

// ---------------------------------------------------------------------------
// Test doubles for wiring tests
// ---------------------------------------------------------------------------

// mockMailer records sent emails.
type mockMailer struct {
	sent []*mailer.Message
}

func (m *mockMailer) Send(_ context.Context, msg *mailer.Message) error {
	m.sent = append(m.sent, msg)
	return nil
}

// mockSMSProvider records sent SMS messages.
type mockSMSProvider struct {
	sent []smsSent
}

type smsSent struct {
	phone, message string
}

func (m *mockSMSProvider) Send(_ context.Context, phone, message string) (*sms.SendResult, error) {
	m.sent = append(m.sent, smsSent{phone: phone, message: message})
	return &sms.SendResult{MessageID: "test-sid", Status: "sent"}, nil
}

// ---------------------------------------------------------------------------
// SetHookDispatcher
// ---------------------------------------------------------------------------

func TestService_SetHookDispatcher(t *testing.T) {
	svc := &Service{jwtSecret: []byte("test-secret"), tokenDur: time.Hour}
	inv := newMockHookInvoker()
	d := NewHookDispatcher(config.AuthHooks{BeforeSignUp: "fn"}, inv, slog.Default())

	svc.SetHookDispatcher(d)

	if svc.hookDispatcher == nil {
		t.Fatal("expected hookDispatcher to be set")
	}
}

func TestEmailDeliveryConfigured_HookOnly(t *testing.T) {
	svc := &Service{
		hookDispatcher: NewHookDispatcher(config.AuthHooks{SendEmail: "send-email"}, newMockHookInvoker(), slog.Default()),
	}

	if !svc.emailDeliveryConfigured() {
		t.Fatal("expected email delivery to be configured when send_email hook is set")
	}
}

func TestSMSDeliveryConfigured_HookOnly(t *testing.T) {
	svc := &Service{
		hookDispatcher: NewHookDispatcher(config.AuthHooks{SendSMS: "send-sms"}, newMockHookInvoker(), slog.Default()),
	}

	if !svc.smsDeliveryConfigured() {
		t.Fatal("expected SMS delivery to be configured when send_sms hook is set")
	}
}

// ---------------------------------------------------------------------------
// generateTokenWithOpts + CustomAccessToken hook
// ---------------------------------------------------------------------------

func TestGenerateTokenWithOpts_CustomAccessTokenHook(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("custom-token", map[string]any{
		"custom_claims": map[string]any{
			"role": "admin",
			"org":  "acme",
		},
	})
	d := NewHookDispatcher(config.AuthHooks{CustomAccessToken: "custom-token"}, inv, slog.Default())

	svc := &Service{
		jwtSecret: []byte("test-secret-32-bytes-long-enough"),
		tokenDur:  time.Hour,
	}
	svc.SetHookDispatcher(d)

	user := &User{ID: "user-123", Email: "user@example.com"}
	tokenStr, err := svc.generateTokenWithOpts(context.Background(), user, nil)
	if err != nil {
		t.Fatalf("generateTokenWithOpts: %v", err)
	}

	// Parse the JWT and check for custom_claims.
	claims := &Claims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return []byte("test-secret-32-bytes-long-enough"), nil
	})
	if err != nil {
		t.Fatalf("parsing JWT: %v", err)
	}

	if claims.CustomClaims == nil {
		t.Fatal("expected custom_claims in JWT, got nil")
	}
	if claims.CustomClaims["role"] != "admin" {
		t.Fatalf("expected custom_claims.role=admin, got %v", claims.CustomClaims["role"])
	}
	if claims.CustomClaims["org"] != "acme" {
		t.Fatalf("expected custom_claims.org=acme, got %v", claims.CustomClaims["org"])
	}
}

func TestGenerateTokenWithOpts_NoHookDispatcher(t *testing.T) {
	svc := &Service{
		jwtSecret: []byte("test-secret-32-bytes-long-enough"),
		tokenDur:  time.Hour,
	}
	// No hook dispatcher set — should work fine without custom claims.
	user := &User{ID: "user-123", Email: "user@example.com"}
	tokenStr, err := svc.generateTokenWithOpts(context.Background(), user, nil)
	if err != nil {
		t.Fatalf("generateTokenWithOpts: %v", err)
	}

	claims := &Claims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return []byte("test-secret-32-bytes-long-enough"), nil
	})
	if err != nil {
		t.Fatalf("parsing JWT: %v", err)
	}
	if claims.CustomClaims != nil {
		t.Fatalf("expected no custom_claims when no hook configured, got %v", claims.CustomClaims)
	}
}

func TestGenerateTokenWithOpts_HookError_FailsGracefully(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("custom-token", errors.New("edge function crashed"))
	d := NewHookDispatcher(config.AuthHooks{CustomAccessToken: "custom-token"}, inv, slog.Default())

	svc := &Service{
		jwtSecret: []byte("test-secret-32-bytes-long-enough"),
		tokenDur:  time.Hour,
	}
	svc.SetHookDispatcher(d)

	user := &User{ID: "user-123", Email: "user@example.com"}
	_, err := svc.generateTokenWithOpts(context.Background(), user, nil)
	if err == nil {
		t.Fatal("expected error when custom_access_token hook fails")
	}
}

// ---------------------------------------------------------------------------
// Register + BeforeSignUp hook wiring
// ---------------------------------------------------------------------------

func TestRegister_BeforeSignUpHookRejects(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("before-signup", map[string]any{"allow": false})
	d := NewHookDispatcher(config.AuthHooks{BeforeSignUp: "before-signup"}, inv, slog.Default())

	svc := &Service{
		jwtSecret:  []byte("test-secret-32-bytes-long-enough"),
		tokenDur:   time.Hour,
		refreshDur: 24 * time.Hour,
		minPwLen:   8,
		logger:     slog.Default(),
	}
	svc.SetHookDispatcher(d)

	ctx := contextWithRequestMetadata(context.Background(), "test-agent", "203.0.113.8")
	_, _, _, err := svc.Register(ctx, "user@example.com", "password123")
	if !errors.Is(err, ErrHookRejected) {
		t.Fatalf("expected ErrHookRejected, got %v", err)
	}
}

func TestRegister_BeforeSignUpHookError_Fails(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("before-signup", errors.New("edge function timeout"))
	d := NewHookDispatcher(config.AuthHooks{BeforeSignUp: "before-signup"}, inv, slog.Default())

	svc := &Service{
		jwtSecret:  []byte("test-secret-32-bytes-long-enough"),
		tokenDur:   time.Hour,
		refreshDur: 24 * time.Hour,
		minPwLen:   8,
		logger:     slog.Default(),
	}
	svc.SetHookDispatcher(d)

	ctx := contextWithRequestMetadata(context.Background(), "test-agent", "203.0.113.8")
	_, _, _, err := svc.Register(ctx, "user@example.com", "password123")
	if err == nil {
		t.Fatal("expected error when before_sign_up hook fails")
	}
}

// ---------------------------------------------------------------------------
// sendAuthEmail: hook-first-then-mailer pattern
// ---------------------------------------------------------------------------

func TestSendAuthEmail_HookConfigured_SkipsMailer(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("send-email", map[string]any{"sent": true})
	d := NewHookDispatcher(config.AuthHooks{SendEmail: "send-email"}, inv, slog.Default())

	ml := &mockMailer{}
	svc := &Service{mailer: ml, logger: slog.Default()}
	svc.SetHookDispatcher(d)

	err := svc.sendAuthEmail(context.Background(), "to@test.com", "Subject", "<p>Hi</p>", "Hi", "signup")
	if err != nil {
		t.Fatalf("sendAuthEmail: %v", err)
	}

	if len(inv.calls) != 1 {
		t.Fatalf("expected hook to be called, got %d calls", len(inv.calls))
	}
	if len(ml.sent) != 0 {
		t.Fatalf("expected mailer NOT called when hook handles email, got %d sends", len(ml.sent))
	}
}

func TestSendAuthEmail_NoHook_FallsBackToMailer(t *testing.T) {
	// No hook configured — should use the built-in mailer.
	d := NewHookDispatcher(config.AuthHooks{}, newMockHookInvoker(), slog.Default())

	ml := &mockMailer{}
	svc := &Service{mailer: ml, logger: slog.Default()}
	svc.SetHookDispatcher(d)

	err := svc.sendAuthEmail(context.Background(), "to@test.com", "Subject", "<p>Hi</p>", "Hi", "signup")
	if err != nil {
		t.Fatalf("sendAuthEmail: %v", err)
	}

	if len(ml.sent) != 1 {
		t.Fatalf("expected mailer to be called, got %d sends", len(ml.sent))
	}
	if ml.sent[0].To != "to@test.com" {
		t.Fatalf("expected to=to@test.com, got %s", ml.sent[0].To)
	}
}

func TestSendAuthEmail_HookError_Fails(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("send-email", errors.New("edge function crashed"))
	d := NewHookDispatcher(config.AuthHooks{SendEmail: "send-email"}, inv, slog.Default())

	ml := &mockMailer{}
	svc := &Service{mailer: ml, logger: slog.Default()}
	svc.SetHookDispatcher(d)

	err := svc.sendAuthEmail(context.Background(), "to@test.com", "Sub", "<p>", "text", "recovery")
	if err == nil {
		t.Fatal("expected error when send_email hook fails")
	}

	if len(ml.sent) != 0 {
		t.Fatalf("expected no mailer fallback when hook fails, got %d sends", len(ml.sent))
	}
}

func TestSendAuthEmail_NoDispatcher_UsesMailer(t *testing.T) {
	ml := &mockMailer{}
	svc := &Service{mailer: ml, logger: slog.Default()}
	// No dispatcher set at all.

	err := svc.sendAuthEmail(context.Background(), "to@test.com", "Sub", "<p>", "text", "signup")
	if err != nil {
		t.Fatalf("sendAuthEmail: %v", err)
	}

	if len(ml.sent) != 1 {
		t.Fatalf("expected mailer to be called with no dispatcher, got %d sends", len(ml.sent))
	}
}

// ---------------------------------------------------------------------------
// sendAuthSMS: hook-first-then-provider pattern
// ---------------------------------------------------------------------------

func TestSendAuthSMS_HookConfigured_SkipsProvider(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("send-sms", map[string]any{"sent": true})
	d := NewHookDispatcher(config.AuthHooks{SendSMS: "send-sms"}, inv, slog.Default())

	sp := &mockSMSProvider{}
	svc := &Service{smsProvider: sp, logger: slog.Default()}
	svc.SetHookDispatcher(d)

	err := svc.sendAuthSMS(context.Background(), "+15551234567", "Your code: 123456", "sms_mfa")
	if err != nil {
		t.Fatalf("sendAuthSMS: %v", err)
	}

	if len(inv.calls) != 1 {
		t.Fatalf("expected hook to be called, got %d calls", len(inv.calls))
	}
	if len(sp.sent) != 0 {
		t.Fatalf("expected SMS provider NOT called when hook handles SMS, got %d sends", len(sp.sent))
	}
}

func TestSendAuthSMS_NoHook_FallsBackToProvider(t *testing.T) {
	d := NewHookDispatcher(config.AuthHooks{}, newMockHookInvoker(), slog.Default())

	sp := &mockSMSProvider{}
	svc := &Service{smsProvider: sp, logger: slog.Default()}
	svc.SetHookDispatcher(d)

	err := svc.sendAuthSMS(context.Background(), "+15551234567", "Your code: 123456", "sms_mfa")
	if err != nil {
		t.Fatalf("sendAuthSMS: %v", err)
	}

	if len(sp.sent) != 1 {
		t.Fatalf("expected SMS provider to be called, got %d sends", len(sp.sent))
	}
}

func TestSendAuthSMS_HookError_Fails(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("send-sms", errors.New("edge function crashed"))
	d := NewHookDispatcher(config.AuthHooks{SendSMS: "send-sms"}, inv, slog.Default())

	sp := &mockSMSProvider{}
	svc := &Service{smsProvider: sp, logger: slog.Default()}
	svc.SetHookDispatcher(d)

	err := svc.sendAuthSMS(context.Background(), "+15551234567", "msg", "sms_signup")
	if err == nil {
		t.Fatal("expected error when send_sms hook fails")
	}

	if len(sp.sent) != 0 {
		t.Fatalf("expected no SMS provider fallback when hook fails, got %d sends", len(sp.sent))
	}
}

// ---------------------------------------------------------------------------
// RequestPasswordReset + BeforePasswordReset hook wiring
// ---------------------------------------------------------------------------

func TestRequestPasswordReset_BeforePasswordResetHookRejects_ReturnsNil(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("before-reset", map[string]any{"allow": false})
	d := NewHookDispatcher(config.AuthHooks{BeforePasswordReset: "before-reset"}, inv, slog.Default())

	svc := &Service{
		logger: slog.Default(),
		mailer: &mockMailer{},
	}
	svc.SetHookDispatcher(d)

	ctx := contextWithRequestMetadata(context.Background(), "test-agent", "198.51.100.10")
	err := svc.RequestPasswordReset(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("expected nil for hook rejection (anti-enumeration), got %v", err)
	}
}

func TestRequestPasswordReset_BeforePasswordResetHookError_Fails(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setError("before-reset", errors.New("edge function timeout"))
	d := NewHookDispatcher(config.AuthHooks{BeforePasswordReset: "before-reset"}, inv, slog.Default())

	svc := &Service{
		logger: slog.Default(),
		mailer: &mockMailer{},
	}
	svc.SetHookDispatcher(d)

	ctx := contextWithRequestMetadata(context.Background(), "test-agent", "198.51.100.10")
	err := svc.RequestPasswordReset(ctx, "user@example.com")
	if err == nil {
		t.Fatal("expected error when before_password_reset hook fails")
	}
}

// ---------------------------------------------------------------------------
// sendAuthEmail payload verification
// ---------------------------------------------------------------------------

func TestSendAuthEmail_HookReceivesCorrectPayload(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("send-email", map[string]any{"sent": true})
	d := NewHookDispatcher(config.AuthHooks{SendEmail: "send-email"}, inv, slog.Default())

	svc := &Service{logger: slog.Default()}
	svc.SetHookDispatcher(d)

	_ = svc.sendAuthEmail(context.Background(), "to@test.com", "Subj", "<html>", "plain", "magiclink")

	if len(inv.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(inv.calls))
	}

	var payload map[string]any
	if err := json.Unmarshal(inv.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["to"] != "to@test.com" {
		t.Fatalf("expected to=to@test.com, got %v", payload["to"])
	}
	if payload["type"] != "magiclink" {
		t.Fatalf("expected type=magiclink, got %v", payload["type"])
	}
}

func TestSendEmailMFACode_UsesHookWhenConfigured(t *testing.T) {
	inv := newMockHookInvoker()
	inv.setJSON("send-email", map[string]any{"sent": true})
	d := NewHookDispatcher(config.AuthHooks{SendEmail: "send-email"}, inv, slog.Default())

	svc := &Service{logger: slog.Default()}
	svc.SetHookDispatcher(d)

	err := svc.sendEmailMFACode(context.Background(), "to@test.com", "123456", "auth.mfa_email_enroll")
	if err != nil {
		t.Fatalf("sendEmailMFACode: %v", err)
	}

	if len(inv.calls) != 1 {
		t.Fatalf("expected hook to be called once, got %d calls", len(inv.calls))
	}
	var payload map[string]any
	if err := json.Unmarshal(inv.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal hook payload: %v", err)
	}
	if payload["type"] != "mfa_email" {
		t.Fatalf("expected email hook type=mfa_email, got %v", payload["type"])
	}
}

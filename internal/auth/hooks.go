package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/allyourbase/ayb/internal/config"
)

var (
	ErrHookRejected      = errors.New("hook rejected request")
	ErrHookNotConfigured = errors.New("hook not configured")
)

// HookInvoker invokes named edge functions with a raw JSON payload.
// Implemented by the server package's edgefunc adapter to avoid import cycles.
type HookInvoker interface {
	InvokeHook(ctx context.Context, name string, payload []byte) ([]byte, error)
}

// HookDispatcher dispatches auth lifecycle hooks to edge functions.
type HookDispatcher struct {
	hooks  config.AuthHooks
	inv    HookInvoker
	logger *slog.Logger
}

// NewHookDispatcher creates a HookDispatcher. inv may be nil (all hooks no-op).
func NewHookDispatcher(hooks config.AuthHooks, inv HookInvoker, logger *slog.Logger) *HookDispatcher {
	return &HookDispatcher{hooks: hooks, inv: inv, logger: logger}
}

// protectedClaims lists JWT claim names the custom_access_token hook cannot modify.
var protectedClaims = map[string]bool{
	"sub": true, "exp": true, "iat": true, "iss": true, "jti": true,
	"aal": true, "amr": true, "nbf": true,
}

// BeforeSignUp calls the before_sign_up hook. Returns ErrHookRejected if allow=false.
// Returns nil immediately if no hook is configured or no invoker is available.
func (d *HookDispatcher) BeforeSignUp(ctx context.Context, email string, metadata map[string]any, ipAddress string) error {
	if d.hooks.BeforeSignUp == "" || d.inv == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"email":      email,
		"metadata":   metadata,
		"ip_address": ipAddress,
	})
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := d.inv.InvokeHook(ctx, d.hooks.BeforeSignUp, payload)
	if err != nil {
		return fmt.Errorf("before_sign_up hook: %w", err)
	}
	var result struct {
		Allow bool   `json:"allow"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("before_sign_up hook response: %w", err)
	}
	if !result.Allow {
		return ErrHookRejected
	}
	return nil
}

// AfterSignUp fires the after_sign_up hook asynchronously (fire-and-forget).
func (d *HookDispatcher) AfterSignUp(ctx context.Context, userID, email string, metadata map[string]any) {
	if d.hooks.AfterSignUp == "" || d.inv == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"user_id":  userID,
		"email":    email,
		"metadata": metadata,
	})
	go func() {
		hookCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := d.inv.InvokeHook(hookCtx, d.hooks.AfterSignUp, payload); err != nil {
			d.logger.Error("after_sign_up hook failed", "error", err)
		}
	}()
}

// CustomAccessToken calls the custom_access_token hook and returns merged claims.
// Hook output under "custom_claims" key is merged into claims; protected claims are never overwritten.
// Returns original claims unchanged if no hook is configured.
func (d *HookDispatcher) CustomAccessToken(ctx context.Context, userID string, claims map[string]any) (map[string]any, error) {
	if d.hooks.CustomAccessToken == "" || d.inv == nil {
		return claims, nil
	}
	payload, _ := json.Marshal(map[string]any{
		"user_id": userID,
		"claims":  claims,
	})
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := d.inv.InvokeHook(ctx, d.hooks.CustomAccessToken, payload)
	if err != nil {
		return nil, fmt.Errorf("custom_access_token hook: %w", err)
	}
	var hookResult map[string]any
	if err := json.Unmarshal(resp, &hookResult); err != nil {
		return nil, fmt.Errorf("custom_access_token hook response: %w", err)
	}
	result := make(map[string]any, len(claims))
	for k, v := range claims {
		result[k] = v
	}
	if cc, ok := hookResult["custom_claims"]; ok {
		result["custom_claims"] = cc
	}
	// Restore protected claims the hook may have tried to overwrite
	for k := range protectedClaims {
		if orig, ok := claims[k]; ok {
			result[k] = orig
		}
	}
	return result, nil
}

// BeforePasswordReset calls the before_password_reset hook. Returns ErrHookRejected if allow=false.
func (d *HookDispatcher) BeforePasswordReset(ctx context.Context, email, ipAddress string) error {
	if d.hooks.BeforePasswordReset == "" || d.inv == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"email":      email,
		"ip_address": ipAddress,
	})
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := d.inv.InvokeHook(ctx, d.hooks.BeforePasswordReset, payload)
	if err != nil {
		return fmt.Errorf("before_password_reset hook: %w", err)
	}
	var result struct {
		Allow bool `json:"allow"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("before_password_reset hook response: %w", err)
	}
	if !result.Allow {
		return ErrHookRejected
	}
	return nil
}

// SendEmail calls the send_email hook. Returns ErrHookNotConfigured if no hook is set
// (caller should fall back to built-in mailer).
func (d *HookDispatcher) SendEmail(ctx context.Context, to, subject, html, text, emailType string) error {
	if d.hooks.SendEmail == "" || d.inv == nil {
		return ErrHookNotConfigured
	}
	payload, _ := json.Marshal(map[string]any{
		"to":      to,
		"subject": subject,
		"html":    html,
		"text":    text,
		"type":    emailType,
	})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := d.inv.InvokeHook(ctx, d.hooks.SendEmail, payload); err != nil {
		return fmt.Errorf("send_email hook: %w", err)
	}
	return nil
}

// SendSMS calls the send_sms hook. Returns ErrHookNotConfigured if no hook is set
// (caller should fall back to built-in SMS provider).
func (d *HookDispatcher) SendSMS(ctx context.Context, phone, message, smsType string) error {
	if d.hooks.SendSMS == "" || d.inv == nil {
		return ErrHookNotConfigured
	}
	payload, _ := json.Marshal(map[string]any{
		"phone":   phone,
		"message": message,
		"type":    smsType,
	})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := d.inv.InvokeHook(ctx, d.hooks.SendSMS, payload); err != nil {
		return fmt.Errorf("send_sms hook: %w", err)
	}
	return nil
}

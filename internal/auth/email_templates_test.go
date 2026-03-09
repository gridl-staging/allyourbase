package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/testutil"
)

// mockEmailTemplateRenderer is a fake that implements EmailTemplateRenderer.
type mockEmailTemplateRenderer struct {
	renderFunc func(ctx context.Context, key string, vars map[string]string) (string, string, string, error)
}

func (m *mockEmailTemplateRenderer) RenderWithFallback(ctx context.Context, key string, vars map[string]string) (string, string, string, error) {
	return m.renderFunc(ctx, key, vars)
}

func TestRenderAuthEmail_UnknownKeyLegacy(t *testing.T) {
	t.Parallel()
	svc := &Service{appName: "TestApp"}

	_, _, _, err := svc.renderAuthEmail(context.Background(), "unknown.key", nil)
	testutil.True(t, err != nil, "unknown key should error in legacy mode")
	testutil.True(t, strings.Contains(err.Error(), "unknown"), "error should mention unknown")
}

func TestRenderAuthEmail_TemplateServiceUsed_AllKeys(t *testing.T) {
	t.Parallel()

	keys := []string{"auth.password_reset", "auth.email_verification", "auth.magic_link"}
	for _, key := range keys {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			called := false
			mock := &mockEmailTemplateRenderer{
				renderFunc: func(_ context.Context, k string, vars map[string]string) (string, string, string, error) {
					called = true
					testutil.Equal(t, key, k)
					return "Custom: " + k, "<p>Custom</p>", "Custom text", nil
				},
			}

			svc := &Service{appName: "TestApp", emailTplSvc: mock}
			vars := map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com"}

			subject, html, text, err := svc.renderAuthEmail(context.Background(), key, vars)
			testutil.NoError(t, err)
			testutil.True(t, called, "template service should have been called for %s", key)
			testutil.Equal(t, "Custom: "+key, subject)
			testutil.Equal(t, "<p>Custom</p>", html)
			testutil.Equal(t, "Custom text", text)
		})
	}
}

func TestRenderAuthEmail_GracefulDegradation(t *testing.T) {
	t.Parallel()

	// When the template service returns an error, renderAuthEmail propagates
	// the error. Graceful degradation (custom → builtin fallback) is handled
	// inside emailtemplates.Service.RenderWithFallback, not in auth.Service.
	// This test verifies that the auth layer correctly surfaces template
	// service errors (the fallback logic is tested in emailtemplates_test.go).
	mock := &mockEmailTemplateRenderer{
		renderFunc: func(_ context.Context, key string, vars map[string]string) (string, string, string, error) {
			return "", "", "", fmt.Errorf("template render failed: broken custom template")
		},
	}

	svc := &Service{appName: "TestApp", emailTplSvc: mock}
	vars := map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com"}

	_, _, _, err := svc.renderAuthEmail(context.Background(), "auth.password_reset", vars)
	testutil.True(t, err != nil, "should propagate template service error")
	testutil.True(t, strings.Contains(err.Error(), "broken custom template"),
		"error should contain the underlying message, got: %v", err)
}

func TestRenderAuthEmail_AllKeysLegacyPath(t *testing.T) {
	t.Parallel()

	// Verify all three system keys render correctly through the legacy path.
	svc := &Service{appName: "TestApp"}
	vars := map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/action"}

	tests := []struct {
		key     string
		subject string
	}{
		{"auth.password_reset", mailer.DefaultPasswordResetSubject},
		{"auth.email_verification", mailer.DefaultVerificationSubject},
		{"auth.magic_link", mailer.DefaultMagicLinkSubject},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			subject, html, text, err := svc.renderAuthEmail(context.Background(), tc.key, vars)
			testutil.NoError(t, err)
			testutil.Equal(t, tc.subject, subject)
			testutil.True(t, strings.Contains(html, "TestApp"), "HTML for %s should contain AppName", tc.key)
			testutil.True(t, strings.Contains(html, "https://example.com/action"), "HTML for %s should contain ActionURL", tc.key)
			testutil.True(t, len(text) > 0, "plaintext for %s should not be empty", tc.key)
		})
	}
}

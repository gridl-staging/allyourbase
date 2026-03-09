package mailer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestLogMailerSend(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	m := NewLogMailer(logger)

	err := m.Send(context.Background(), &Message{
		To:      "user@example.com",
		Subject: "Test Subject",
		HTML:    "<p>Hello</p>",
		Text:    "Hello",
	})
	testutil.NoError(t, err)

	// Verify the logger actually received the message fields.
	output := buf.String()
	testutil.Contains(t, output, "user@example.com")
	testutil.Contains(t, output, "Test Subject")
}

func TestWebhookMailerSend(t *testing.T) {
	t.Parallel()
	var received webhookPayload
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-AYB-Signature")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("handler: read body: %v", err)
			return
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Errorf("handler: unmarshal payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	secret := "test-webhook-secret"
	m := NewWebhookMailer(WebhookConfig{
		URL:    srv.URL,
		Secret: secret,
	})

	msg := &Message{
		To:      "user@example.com",
		Subject: "Test",
		HTML:    "<p>Hi</p>",
		Text:    "Hi",
	}
	err := m.Send(context.Background(), msg)
	testutil.NoError(t, err)

	testutil.Equal(t, "user@example.com", received.To)
	testutil.Equal(t, "Test", received.Subject)
	testutil.Equal(t, "<p>Hi</p>", received.HTML)
	testutil.Equal(t, "Hi", received.Text)

	// Verify HMAC signature.
	testutil.True(t, gotSig != "", "signature header should be set")
	payload, _ := json.Marshal(received)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	testutil.Equal(t, expectedSig, gotSig)
}

func TestWebhookMailerNoSecret(t *testing.T) {
	t.Parallel()
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-AYB-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewWebhookMailer(WebhookConfig{URL: srv.URL})
	err := m.Send(context.Background(), &Message{To: "a@b.com", Subject: "x"})
	testutil.NoError(t, err)
	testutil.Equal(t, "", gotSig)
}

func TestWebhookMailerDefaultTimeout(t *testing.T) {
	t.Parallel()
	m := NewWebhookMailer(WebhookConfig{URL: "http://localhost"})
	testutil.Equal(t, float64(10), m.client.Timeout.Seconds())
}

func TestWebhookMailerCustomTimeout(t *testing.T) {
	t.Parallel()
	m := NewWebhookMailer(WebhookConfig{URL: "http://localhost", Timeout: 30 * time.Second})
	testutil.Equal(t, float64(30), m.client.Timeout.Seconds())
}

func TestWebhookMailerNon2xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewWebhookMailer(WebhookConfig{URL: srv.URL})
	err := m.Send(context.Background(), &Message{To: "a@b.com", Subject: "x"})
	testutil.ErrorContains(t, err, "status 500")
}

func TestRenderPasswordReset(t *testing.T) {
	t.Parallel()
	html, text, err := RenderPasswordReset(TemplateData{
		AppName:   "MyApp",
		ActionURL: "https://example.com/reset?token=abc123",
	})
	testutil.NoError(t, err)
	testutil.Contains(t, html, "Reset your password")
	testutil.Contains(t, html, "MyApp")
	testutil.Contains(t, html, "https://example.com/reset?token=abc123")
	testutil.Contains(t, text, "Reset your password")
	testutil.True(t, len(text) > 0, "text fallback should not be empty")
}

func TestRenderVerification(t *testing.T) {
	t.Parallel()
	html, text, err := RenderVerification(TemplateData{
		AppName:   "MyApp",
		ActionURL: "https://example.com/verify?token=xyz",
	})
	testutil.NoError(t, err)
	testutil.Contains(t, html, "Verify your email")
	testutil.Contains(t, html, "MyApp")
	testutil.Contains(t, html, "https://example.com/verify?token=xyz")
	testutil.Contains(t, text, "Verify your email")
}

func TestRenderMagicLink(t *testing.T) {
	t.Parallel()
	html, text, err := RenderMagicLink(TemplateData{
		AppName:   "Sigil",
		ActionURL: "https://example.com/auth/magic-link/confirm?token=tok123",
	})
	testutil.NoError(t, err)
	testutil.Contains(t, html, "login link")
	testutil.Contains(t, html, "Sigil")
	testutil.Contains(t, html, "https://example.com/auth/magic-link/confirm?token=tok123")
	testutil.Contains(t, text, "login link")
	testutil.True(t, len(text) > 0, "text fallback should not be empty")
}

func TestStripHTML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"simple tag", "<p>hello</p>", "hello"},
		{"nested tags", "<div><p>hello</p></div>", "hello"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stripHTML(tt.in)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestSMTPMailerConfigStored(t *testing.T) {
	t.Parallel()
	m := NewSMTPMailer(SMTPConfig{
		Host: "smtp.example.com",
		Port: 587,
		From: "noreply@example.com",
	})
	testutil.NotNil(t, m)
	testutil.Equal(t, "smtp.example.com", m.cfg.Host)
	testutil.Equal(t, 587, m.cfg.Port)
	testutil.Equal(t, "noreply@example.com", m.cfg.From)
}

func TestSMTPMailerFormatFrom(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		from     string
		fromName string
		want     string
	}{
		{"address only", "noreply@example.com", "", "noreply@example.com"},
		{"with display name", "noreply@example.com", "MyApp", "MyApp <noreply@example.com>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &SMTPMailer{cfg: SMTPConfig{From: tt.from, FromName: tt.fromName}}
			testutil.Equal(t, tt.want, m.formatFrom())
		})
	}
}

func TestSMTPMailerAuthTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method   string
		wantSame bool // true if should return same as PLAIN (the default)
	}{
		{"PLAIN", true},
		{"LOGIN", false},
		{"CRAM-MD5", false},
		{"", true}, // unknown defaults to PLAIN
	}

	plainMailer := &SMTPMailer{cfg: SMTPConfig{AuthMethod: "PLAIN"}}
	plainResult := plainMailer.authType()

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			t.Parallel()
			m := &SMTPMailer{cfg: SMTPConfig{AuthMethod: tt.method}}
			result := m.authType()
			if tt.wantSame {
				testutil.Equal(t, plainResult, result)
			} else {
				testutil.True(t, result != plainResult,
					"expected %q to produce different auth type than PLAIN", tt.method)
			}
		})
	}

	// Verify LOGIN and CRAM-MD5 are distinct from each other.
	loginMailer := &SMTPMailer{cfg: SMTPConfig{AuthMethod: "LOGIN"}}
	cramMailer := &SMTPMailer{cfg: SMTPConfig{AuthMethod: "CRAM-MD5"}}
	testutil.True(t, loginMailer.authType() != cramMailer.authType(),
		"LOGIN and CRAM-MD5 should produce different auth types")
}

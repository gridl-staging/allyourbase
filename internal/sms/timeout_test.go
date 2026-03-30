package sms

import (
	"testing"
	"time"
)

// TestProviderHTTPClientTimeouts verifies every SMS provider sets a non-zero
// Timeout on its HTTP client.  A zero timeout means the request can hang
// indefinitely if the upstream SMS API is unresponsive — a denial-of-service
// vector in production.
func TestProviderHTTPClientTimeouts(t *testing.T) {
	// Expected timeout for all SMS providers — external API calls that
	// should complete within seconds.
	const want = 10 * time.Second

	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"Twilio", NewTwilioProvider("sid", "tok", "+1", "").client.Timeout},
		{"Vonage", NewVonageProvider("key", "sec", "+1", "").client.Timeout},
		{"Plivo", NewPlivoProvider("id", "tok", "+1", "").client.Timeout},
		{"Telnyx", NewTelnyxProvider("key", "+1", "").client.Timeout},
		{"MSG91", NewMSG91Provider("key", "tmpl", "").client.Timeout},
		{"Webhook", NewWebhookProvider("http://example.com", "secret").client.Timeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.timeout != want {
				t.Errorf("HTTP client timeout = %v, want %v", tt.timeout, want)
			}
		})
	}
}

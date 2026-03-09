package cli

import (
	"log/slog"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildSMSProvider_Log(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "log"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.LogProvider)
	testutil.True(t, ok, "expected *sms.LogProvider")
}

func TestBuildSMSProvider_Twilio(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "twilio"
	cfg.Auth.TwilioSID = "AC_test_sid"
	cfg.Auth.TwilioToken = "test_token"
	cfg.Auth.TwilioFrom = "+15551234567"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.TwilioProvider)
	testutil.True(t, ok, "expected *sms.TwilioProvider")
}

func TestBuildSMSProvider_Plivo(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "plivo"
	cfg.Auth.PlivoAuthID = "PLIVO_ID"
	cfg.Auth.PlivoAuthToken = "PLIVO_TOKEN"
	cfg.Auth.PlivoFrom = "+15551234567"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.PlivoProvider)
	testutil.True(t, ok, "expected *sms.PlivoProvider")
}

func TestBuildSMSProvider_Telnyx(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "telnyx"
	cfg.Auth.TelnyxAPIKey = "KEY_123"
	cfg.Auth.TelnyxFrom = "+15551234567"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.TelnyxProvider)
	testutil.True(t, ok, "expected *sms.TelnyxProvider")
}

func TestBuildSMSProvider_MSG91(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "msg91"
	cfg.Auth.MSG91AuthKey = "AUTH_KEY"
	cfg.Auth.MSG91TemplateID = "TMPL_123"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.MSG91Provider)
	testutil.True(t, ok, "expected *sms.MSG91Provider")
}

func TestBuildSMSProvider_SNS(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "sns"
	cfg.Auth.AWSRegion = "us-east-1"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.SNSProvider)
	testutil.True(t, ok, "expected *sms.SNSProvider")
}

func TestBuildSMSProvider_Vonage(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "vonage"
	cfg.Auth.VonageAPIKey = "KEY"
	cfg.Auth.VonageAPISecret = "SECRET"
	cfg.Auth.VonageFrom = "+15551234567"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.VonageProvider)
	testutil.True(t, ok, "expected *sms.VonageProvider")
}

func TestBuildSMSProvider_Webhook(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = "webhook"
	cfg.Auth.SMSWebhookURL = "https://example.com/sms"
	cfg.Auth.SMSWebhookSecret = "secret123"
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.WebhookProvider)
	testutil.True(t, ok, "expected *sms.WebhookProvider")
}

func TestBuildSMSProvider_Default(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.SMSProvider = ""
	logger := slog.Default()

	p := buildSMSProvider(cfg, logger)
	_, ok := p.(*sms.LogProvider)
	testutil.True(t, ok, "expected *sms.LogProvider for empty/unknown provider")

	// Also test unknown provider string.
	cfg.Auth.SMSProvider = "unknown"
	p = buildSMSProvider(cfg, logger)
	_, ok = p.(*sms.LogProvider)
	testutil.True(t, ok, "expected *sms.LogProvider for unknown provider")
}

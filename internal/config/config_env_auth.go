package config

import (
	"os"
	"strings"
)

// applyAuthEnv applies AYB_AUTH_* environment variable overrides to the config.
func applyAuthEnv(cfg *Config) error {
	if v := os.Getenv("AYB_AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if err := envInt("AYB_AUTH_REFRESH_TOKEN_DURATION", &cfg.Auth.RefreshTokenDuration); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_RATE_LIMIT", &cfg.Auth.RateLimit); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_ANONYMOUS_RATE_LIMIT", &cfg.Auth.AnonymousRateLimit); err != nil {
		return err
	}
	if v := os.Getenv("AYB_AUTH_RATE_LIMIT_AUTH"); v != "" {
		cfg.Auth.RateLimitAuth = v
	}
	if err := envInt("AYB_AUTH_MIN_PASSWORD_LENGTH", &cfg.Auth.MinPasswordLength); err != nil {
		return err
	}
	if v := os.Getenv("AYB_AUTH_OAUTH_REDIRECT_URL"); v != "" {
		cfg.Auth.OAuthRedirectURL = v
	}
	if v := os.Getenv("AYB_AUTH_OAUTH_PROVIDER_ENABLED"); v != "" {
		cfg.Auth.OAuthProviderMode.Enabled = v == "true" || v == "1"
	}
	if err := envInt("AYB_AUTH_OAUTH_PROVIDER_ACCESS_TOKEN_DURATION", &cfg.Auth.OAuthProviderMode.AccessTokenDuration); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_OAUTH_PROVIDER_REFRESH_TOKEN_DURATION", &cfg.Auth.OAuthProviderMode.RefreshTokenDuration); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_OAUTH_PROVIDER_AUTH_CODE_DURATION", &cfg.Auth.OAuthProviderMode.AuthCodeDuration); err != nil {
		return err
	}
	if v := os.Getenv("AYB_AUTH_MAGIC_LINK_ENABLED"); v != "" {
		cfg.Auth.MagicLinkEnabled = v == "true" || v == "1"
	}
	if err := envInt("AYB_AUTH_MAGIC_LINK_DURATION", &cfg.Auth.MagicLinkDuration); err != nil {
		return err
	}
	if v := os.Getenv("AYB_AUTH_EMAIL_MFA_ENABLED"); v != "" {
		cfg.Auth.EmailMFAEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_ANONYMOUS_AUTH_ENABLED"); v != "" {
		cfg.Auth.AnonymousAuthEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_TOTP_ENABLED"); v != "" {
		cfg.Auth.TOTPEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_ENCRYPTION_KEY"); v != "" {
		cfg.Auth.EncryptionKey = v
	}
	// SMS config.
	if v := os.Getenv("AYB_AUTH_SMS_ENABLED"); v != "" {
		cfg.Auth.SMSEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_SMS_PROVIDER"); v != "" {
		cfg.Auth.SMSProvider = v
	}
	if v := os.Getenv("AYB_AUTH_TWILIO_SID"); v != "" {
		cfg.Auth.TwilioSID = v
	}
	if v := os.Getenv("AYB_AUTH_TWILIO_TOKEN"); v != "" {
		cfg.Auth.TwilioToken = v
	}
	if v := os.Getenv("AYB_AUTH_TWILIO_FROM"); v != "" {
		cfg.Auth.TwilioFrom = v
	}
	// Plivo
	if v := os.Getenv("AYB_AUTH_PLIVO_AUTH_ID"); v != "" {
		cfg.Auth.PlivoAuthID = v
	}
	if v := os.Getenv("AYB_AUTH_PLIVO_AUTH_TOKEN"); v != "" {
		cfg.Auth.PlivoAuthToken = v
	}
	if v := os.Getenv("AYB_AUTH_PLIVO_FROM"); v != "" {
		cfg.Auth.PlivoFrom = v
	}
	// Telnyx
	if v := os.Getenv("AYB_AUTH_TELNYX_API_KEY"); v != "" {
		cfg.Auth.TelnyxAPIKey = v
	}
	if v := os.Getenv("AYB_AUTH_TELNYX_FROM"); v != "" {
		cfg.Auth.TelnyxFrom = v
	}
	// MSG91
	if v := os.Getenv("AYB_AUTH_MSG91_AUTH_KEY"); v != "" {
		cfg.Auth.MSG91AuthKey = v
	}
	if v := os.Getenv("AYB_AUTH_MSG91_TEMPLATE_ID"); v != "" {
		cfg.Auth.MSG91TemplateID = v
	}
	// AWS SNS
	if v := os.Getenv("AYB_AUTH_AWS_REGION"); v != "" {
		cfg.Auth.AWSRegion = v
	}
	// Vonage
	if v := os.Getenv("AYB_AUTH_VONAGE_API_KEY"); v != "" {
		cfg.Auth.VonageAPIKey = v
	}
	if v := os.Getenv("AYB_AUTH_VONAGE_API_SECRET"); v != "" {
		cfg.Auth.VonageAPISecret = v
	}
	if v := os.Getenv("AYB_AUTH_VONAGE_FROM"); v != "" {
		cfg.Auth.VonageFrom = v
	}
	// SMS Webhook
	if v := os.Getenv("AYB_AUTH_SMS_WEBHOOK_URL"); v != "" {
		cfg.Auth.SMSWebhookURL = v
	}
	if v := os.Getenv("AYB_AUTH_SMS_WEBHOOK_SECRET"); v != "" {
		cfg.Auth.SMSWebhookSecret = v
	}

	applyOAuthEnv(cfg, "google")
	applyOAuthEnv(cfg, "github")
	applyOAuthEnv(cfg, "microsoft")
	applyOAuthEnv(cfg, "apple")

	return nil
}

// applyOAuthEnv reads environment variables for the specified OAuth provider and applies them to the config. The provider name is used to form the environment variable prefix, for example AYB_AUTH_OAUTH_GOOGLE_CLIENT_ID for the google provider.
func applyOAuthEnv(cfg *Config, provider string) {
	prefix := "AYB_AUTH_OAUTH_" + strings.ToUpper(provider) + "_"
	id := os.Getenv(prefix + "CLIENT_ID")
	secret := os.Getenv(prefix + "CLIENT_SECRET")
	enabled := os.Getenv(prefix + "ENABLED")
	storeProviderTokens := os.Getenv(prefix + "STORE_PROVIDER_TOKENS")
	tenantID := os.Getenv(prefix + "TENANT_ID")
	teamID := os.Getenv(prefix + "TEAM_ID")
	keyID := os.Getenv(prefix + "KEY_ID")
	privateKey := os.Getenv(prefix + "PRIVATE_KEY")
	if id == "" && secret == "" && enabled == "" && storeProviderTokens == "" && tenantID == "" && teamID == "" && keyID == "" && privateKey == "" {
		return
	}
	if cfg.Auth.OAuth == nil {
		cfg.Auth.OAuth = make(map[string]OAuthProvider)
	}
	p := cfg.Auth.OAuth[provider]
	if id != "" {
		p.ClientID = id
	}
	if secret != "" {
		p.ClientSecret = secret
	}
	if enabled != "" {
		p.Enabled = enabled == "true" || enabled == "1"
	}
	if storeProviderTokens != "" {
		p.StoreProviderTokens = storeProviderTokens == "true" || storeProviderTokens == "1"
	}
	if tenantID != "" {
		p.TenantID = tenantID
	}
	if teamID != "" {
		p.TeamID = teamID
	}
	if keyID != "" {
		p.KeyID = keyID
	}
	if privateKey != "" {
		p.PrivateKey = privateKey
	}
	cfg.Auth.OAuth[provider] = p
}

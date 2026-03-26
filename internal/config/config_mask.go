package config

import "github.com/allyourbase/ayb/internal/urlutil"

// maskSecret replaces a non-empty secret string with "***".
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	return "***"
}

// MaskedCopy returns a deep copy of the config with all secret fields redacted.
// Use this for display purposes (e.g. ayb config) to avoid leaking credentials.
func (c *Config) MaskedCopy() *Config {
	cp := *c

	// Admin.
	cp.Admin.Password = maskSecret(c.Admin.Password)

	// Auth secrets.
	cp.Auth.JWTSecret = maskSecret(c.Auth.JWTSecret)
	cp.Auth.EncryptionKey = maskSecret(c.Auth.EncryptionKey)
	cp.Auth.TwilioToken = maskSecret(c.Auth.TwilioToken)
	cp.Auth.TwilioSID = maskSecret(c.Auth.TwilioSID)
	cp.Auth.PlivoAuthToken = maskSecret(c.Auth.PlivoAuthToken)
	cp.Auth.TelnyxAPIKey = maskSecret(c.Auth.TelnyxAPIKey)
	cp.Auth.MSG91AuthKey = maskSecret(c.Auth.MSG91AuthKey)
	cp.Auth.VonageAPIKey = maskSecret(c.Auth.VonageAPIKey)
	cp.Auth.VonageAPISecret = maskSecret(c.Auth.VonageAPISecret)
	cp.Auth.SMSWebhookSecret = maskSecret(c.Auth.SMSWebhookSecret)

	// Vault secrets.
	cp.Vault.MasterKey = maskSecret(c.Vault.MasterKey)

	// Mask OAuth client secrets (make a new map to avoid mutating the original).
	if len(c.Auth.OAuth) > 0 {
		cp.Auth.OAuth = make(map[string]OAuthProvider, len(c.Auth.OAuth))
		for name, p := range c.Auth.OAuth {
			p.ClientSecret = maskSecret(p.ClientSecret)
			cp.Auth.OAuth[name] = p
		}
	}

	// Mask OIDC client secrets.
	if len(c.Auth.OIDC) > 0 {
		cp.Auth.OIDC = make(map[string]OIDCProvider, len(c.Auth.OIDC))
		for name, p := range c.Auth.OIDC {
			p.ClientSecret = maskSecret(p.ClientSecret)
			cp.Auth.OIDC[name] = p
		}
	}

	// Email secrets.
	cp.Email.SMTP.Password = maskSecret(c.Email.SMTP.Password)
	cp.Email.Webhook.Secret = maskSecret(c.Email.Webhook.Secret)
	cp.Support.WebhookSecret = maskSecret(c.Support.WebhookSecret)

	// Storage secrets.
	cp.Storage.S3AccessKey = maskSecret(c.Storage.S3AccessKey)
	cp.Storage.S3SecretKey = maskSecret(c.Storage.S3SecretKey)

	// Database URL may contain credentials.
	cp.Database.URL = urlutil.RedactURL(c.Database.URL)

	return &cp
}

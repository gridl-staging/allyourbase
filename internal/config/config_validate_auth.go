// Package config Provides validation functions for authentication configuration including basic auth, OAuth, OIDC, SMS, SAML, and OAuth provider mode settings.
package config

import (
	"fmt"
)

// validateAuthConfig validates core authentication settings including minimum password length, rate limits, API size limits, and JWT secret configuration.
func validateAuthConfig(c *Config) error {
	if c.Auth.MinPasswordLength < 1 {
		return fmt.Errorf("auth.min_password_length must be at least 1, got %d", c.Auth.MinPasswordLength)
	}
	if _, _, err := ParseRateLimitSpec(c.Auth.RateLimitAuth); err != nil {
		return fmt.Errorf("auth.rate_limit_auth: %w", err)
	}
	if _, _, err := ParseRateLimitSpec(c.RateLimit.API); err != nil {
		return fmt.Errorf("rate_limit.api: %w", err)
	}
	if _, _, err := ParseRateLimitSpec(c.RateLimit.APIAnonymous); err != nil {
		return fmt.Errorf("rate_limit.api_anonymous: %w", err)
	}
	if c.API.ImportMaxSizeMB <= 0 {
		return fmt.Errorf("api.import_max_size_mb must be positive, got %d", c.API.ImportMaxSizeMB)
	}
	if c.API.ImportMaxRows <= 0 {
		return fmt.Errorf("api.import_max_rows must be positive, got %d", c.API.ImportMaxRows)
	}
	if c.API.ExportMaxRows <= 0 {
		return fmt.Errorf("api.export_max_rows must be positive, got %d", c.API.ExportMaxRows)
	}
	if c.Auth.Enabled && c.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required when auth is enabled")
	}
	if c.Auth.JWTSecret != "" && len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 characters, got %d", len(c.Auth.JWTSecret))
	}
	return nil
}

func validateAuthFeatureConfig(c *Config) error {
	if c.Auth.MagicLinkEnabled && !c.Auth.Enabled {
		return fmt.Errorf("auth.enabled must be true to use magic link authentication")
	}
	if c.Auth.EmailMFAEnabled && !c.Auth.Enabled {
		return fmt.Errorf("email_mfa_enabled requires auth.enabled")
	}
	if c.Auth.AnonymousAuthEnabled && !c.Auth.Enabled {
		return fmt.Errorf("anonymous_auth_enabled requires auth.enabled")
	}
	if c.Auth.TOTPEnabled && !c.Auth.Enabled {
		return fmt.Errorf("totp_enabled requires auth.enabled")
	}
	return nil
}

// validateSMSConfig validates SMS configuration when enabled, validating provider-specific credentials, code length and expiry constraints, and allowed country codes.
func validateSMSConfig(c *Config) error {
	if !c.Auth.SMSEnabled {
		return nil
	}
	if !c.Auth.Enabled {
		return fmt.Errorf("sms_enabled requires auth.enabled")
	}
	switch c.Auth.SMSProvider {
	case "twilio":
		if c.Auth.TwilioSID == "" {
			return fmt.Errorf("auth.twilio_sid is required when sms_provider is \"twilio\"")
		}
		if c.Auth.TwilioToken == "" {
			return fmt.Errorf("auth.twilio_token is required when sms_provider is \"twilio\"")
		}
		if c.Auth.TwilioFrom == "" {
			return fmt.Errorf("auth.twilio_from is required when sms_provider is \"twilio\"")
		}
	case "plivo":
		if c.Auth.PlivoAuthID == "" {
			return fmt.Errorf("auth.plivo_auth_id is required when sms_provider is \"plivo\"")
		}
		if c.Auth.PlivoAuthToken == "" {
			return fmt.Errorf("auth.plivo_auth_token is required when sms_provider is \"plivo\"")
		}
		if c.Auth.PlivoFrom == "" {
			return fmt.Errorf("auth.plivo_from is required when sms_provider is \"plivo\"")
		}
	case "telnyx":
		if c.Auth.TelnyxAPIKey == "" {
			return fmt.Errorf("auth.telnyx_api_key is required when sms_provider is \"telnyx\"")
		}
		if c.Auth.TelnyxFrom == "" {
			return fmt.Errorf("auth.telnyx_from is required when sms_provider is \"telnyx\"")
		}
	case "msg91":
		if c.Auth.MSG91AuthKey == "" {
			return fmt.Errorf("auth.msg91_auth_key is required when sms_provider is \"msg91\"")
		}
		if c.Auth.MSG91TemplateID == "" {
			return fmt.Errorf("auth.msg91_template_id is required when sms_provider is \"msg91\"")
		}
	case "sns":
		if c.Auth.AWSRegion == "" {
			return fmt.Errorf("auth.aws_region is required when sms_provider is \"sns\"")
		}
	case "vonage":
		if c.Auth.VonageAPIKey == "" {
			return fmt.Errorf("auth.vonage_api_key is required when sms_provider is \"vonage\"")
		}
		if c.Auth.VonageAPISecret == "" {
			return fmt.Errorf("auth.vonage_api_secret is required when sms_provider is \"vonage\"")
		}
		if c.Auth.VonageFrom == "" {
			return fmt.Errorf("auth.vonage_from is required when sms_provider is \"vonage\"")
		}
	case "webhook":
		if c.Auth.SMSWebhookURL == "" {
			return fmt.Errorf("auth.sms_webhook_url is required when sms_provider is \"webhook\"")
		}
		if c.Auth.SMSWebhookSecret == "" {
			return fmt.Errorf("auth.sms_webhook_secret is required when sms_provider is \"webhook\"")
		}
	case "log":
	default:
		return fmt.Errorf("auth.sms_provider must be one of: \"log\", \"twilio\", \"plivo\", \"telnyx\", \"msg91\", \"sns\", \"vonage\", \"webhook\"; got %q", c.Auth.SMSProvider)
	}
	if c.Auth.SMSCodeLength < 4 || c.Auth.SMSCodeLength > 8 {
		return fmt.Errorf("auth.sms_code_length must be between 4 and 8, got %d", c.Auth.SMSCodeLength)
	}
	if c.Auth.SMSCodeExpiry < 60 || c.Auth.SMSCodeExpiry > 600 {
		return fmt.Errorf("auth.sms_code_expiry must be between 60 and 600, got %d", c.Auth.SMSCodeExpiry)
	}
	if c.Auth.SMSDailyLimit < 0 {
		return fmt.Errorf("auth.sms_daily_limit must be non-negative, got %d", c.Auth.SMSDailyLimit)
	}
	for _, code := range c.Auth.SMSAllowedCountries {
		if !validISO3166Alpha2[code] {
			return fmt.Errorf("auth.sms_allowed_countries: %q is not a valid ISO 3166-1 alpha-2 country code", code)
		}
	}
	return nil
}

// validateOAuthConfig validates enabled OAuth provider configurations, ensuring all required credentials and client identifiers are present for each provider.
func validateOAuthConfig(c *Config) error {
	for name, provider := range c.Auth.OAuth {
		if !provider.Enabled {
			continue
		}
		if !c.Auth.Enabled {
			return fmt.Errorf("auth.enabled must be true to use OAuth provider %q", name)
		}
		if provider.ClientID == "" {
			return fmt.Errorf("auth.oauth.%s.client_id is required when enabled", name)
		}
		switch name {
		case "google", "github", "microsoft",
			"discord", "twitter", "facebook", "linkedin",
			"spotify", "twitch", "gitlab", "bitbucket",
			"slack", "zoom", "notion", "figma":
			if provider.ClientSecret == "" {
				return fmt.Errorf("auth.oauth.%s.client_secret is required when enabled", name)
			}
		case "apple":
			if provider.TeamID == "" {
				return fmt.Errorf("auth.oauth.apple.team_id is required when enabled")
			}
			if provider.KeyID == "" {
				return fmt.Errorf("auth.oauth.apple.key_id is required when enabled")
			}
			if provider.PrivateKey == "" {
				return fmt.Errorf("auth.oauth.apple.private_key is required when enabled")
			}
		default:
			return fmt.Errorf("unsupported OAuth provider %q", name)
		}
	}
	return nil
}

// validateOIDCConfig validates enabled OIDC provider configurations, checking for required issuer URL and credentials while preventing name conflicts with built-in OAuth providers.
func validateOIDCConfig(c *Config) error {
	builtInOAuthProviders := map[string]bool{
		"google": true, "github": true, "microsoft": true, "apple": true,
		"discord": true, "twitter": true, "facebook": true, "linkedin": true,
		"spotify": true, "twitch": true, "gitlab": true, "bitbucket": true,
		"slack": true, "zoom": true, "notion": true, "figma": true,
	}
	for name, provider := range c.Auth.OIDC {
		if !provider.Enabled {
			continue
		}
		if !c.Auth.Enabled {
			return fmt.Errorf("auth.enabled must be true to use OIDC provider %q", name)
		}
		if builtInOAuthProviders[name] {
			return fmt.Errorf("auth.oidc.%s: name conflicts with built-in OAuth provider", name)
		}
		if provider.IssuerURL == "" {
			return fmt.Errorf("auth.oidc.%s.issuer_url is required when enabled", name)
		}
		if provider.ClientID == "" {
			return fmt.Errorf("auth.oidc.%s.client_id is required when enabled", name)
		}
		if provider.ClientSecret == "" {
			return fmt.Errorf("auth.oidc.%s.client_secret is required when enabled", name)
		}
	}
	return nil
}

// validateSAMLConfig validates enabled SAML provider configurations, checking for required entity ID and metadata while ensuring provider names are unique.
func validateSAMLConfig(c *Config) error {
	samlNames := make(map[string]struct{}, len(c.Auth.SAMLProviders))
	for index, provider := range c.Auth.SAMLProviders {
		if !provider.Enabled {
			continue
		}
		if !c.Auth.Enabled {
			return fmt.Errorf("auth.enabled must be true to use SAML provider %q", provider.Name)
		}
		if provider.Name == "" {
			return fmt.Errorf("auth.saml_providers[%d].name is required when enabled", index)
		}
		if _, exists := samlNames[provider.Name]; exists {
			return fmt.Errorf("auth.saml_providers[%d].name %q is duplicated", index, provider.Name)
		}
		samlNames[provider.Name] = struct{}{}
		if provider.EntityID == "" {
			return fmt.Errorf("auth.saml_providers[%d].entity_id is required when enabled", index)
		}
		if provider.IDPMetadataURL == "" && provider.IDPMetadataXML == "" {
			return fmt.Errorf("auth.saml_providers[%d] requires idp_metadata_url or idp_metadata_xml when enabled", index)
		}
	}
	return nil
}

// validateOAuthModeConfig validates OAuth provider mode configuration, ensuring the JWT secret is present and token durations meet minimum requirements.
func validateOAuthModeConfig(c *Config) error {
	if !c.Auth.OAuthProviderMode.Enabled {
		return nil
	}
	if !c.Auth.Enabled {
		return fmt.Errorf("auth.enabled must be true to use OAuth provider mode")
	}
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required for OAuth provider mode (used by consent flow session auth)")
	}
	if c.Auth.OAuthProviderMode.AccessTokenDuration < 1 {
		return fmt.Errorf("auth.oauth_provider.access_token_duration must be at least 1, got %d", c.Auth.OAuthProviderMode.AccessTokenDuration)
	}
	if c.Auth.OAuthProviderMode.RefreshTokenDuration < 1 {
		return fmt.Errorf("auth.oauth_provider.refresh_token_duration must be at least 1, got %d", c.Auth.OAuthProviderMode.RefreshTokenDuration)
	}
	if c.Auth.OAuthProviderMode.AuthCodeDuration < 1 {
		return fmt.Errorf("auth.oauth_provider.auth_code_duration must be at least 1, got %d", c.Auth.OAuthProviderMode.AuthCodeDuration)
	}
	return nil
}

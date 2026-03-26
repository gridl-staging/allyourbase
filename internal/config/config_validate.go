// Package config Provides validation logic for all configuration sections of the Config struct.
package config

// Validate performs comprehensive validation across all configuration sections including server, database, authentication, billing, GraphQL, SMS, OAuth, OIDC, SAML, email, storage, edge functions, logging, metrics, telemetry, jobs, status, push notifications, backup, AI, and realtime settings. It returns an error if any validation check fails.
func (c *Config) Validate() error {
	if err := validateServerConfig(c); err != nil {
		return err
	}
	if err := validateDatabaseConfig(c); err != nil {
		return err
	}
	if err := validateAuthConfig(c); err != nil {
		return err
	}
	if err := validateBillingConfig(c); err != nil {
		return err
	}
	if err := validateGraphQLConfig(c); err != nil {
		return err
	}
	if err := validateBillingStripeConfig(c); err != nil {
		return err
	}
	if err := validateAuthFeatureConfig(c); err != nil {
		return err
	}
	if err := validateSMSConfig(c); err != nil {
		return err
	}
	if err := validateOAuthConfig(c); err != nil {
		return err
	}
	if err := validateOIDCConfig(c); err != nil {
		return err
	}
	if err := validateSAMLConfig(c); err != nil {
		return err
	}
	if err := validateOAuthModeConfig(c); err != nil {
		return err
	}
	if err := validateEmailConfig(c); err != nil {
		return err
	}
	if err := validateStorageConfig(c); err != nil {
		return err
	}
	if err := validateEdgeFuncConfig(c); err != nil {
		return err
	}
	if err := validateLoggingConfig(c); err != nil {
		return err
	}
	if err := validateMetricsConfig(c); err != nil {
		return err
	}
	if err := validateTelemetryConfig(c); err != nil {
		return err
	}
	if err := validateJobsConfig(c); err != nil {
		return err
	}
	if err := validateStatusConfig(c); err != nil {
		return err
	}
	if err := validatePushConfig(c); err != nil {
		return err
	}
	if err := validateBackupConfig(c); err != nil {
		return err
	}
	if err := validateAIConfig(c); err != nil {
		return err
	}
	if err := validateRealtimeConfig(c); err != nil {
		return err
	}
	return nil
}

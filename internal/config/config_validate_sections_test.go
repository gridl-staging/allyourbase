package config

import "testing"

func TestValidateSectionExtractionsExist(t *testing.T) {
	_ = validateServerConfig
	_ = validateDatabaseConfig
	_ = validateAuthConfig
	_ = validateBillingConfig
	_ = validateGraphQLConfig
	_ = validateSMSConfig
	_ = validateOAuthConfig
	_ = validateOIDCConfig
	_ = validateSAMLConfig
	_ = validateOAuthModeConfig
	_ = validateEmailConfig
	_ = validateStorageConfig
	_ = validateEdgeFuncConfig
	_ = validateLoggingConfig
	_ = validateMetricsConfig
	_ = validateTelemetryConfig
	_ = validateJobsConfig
	_ = validateStatusConfig
	_ = validatePushConfig
	_ = validateBackupConfig
	_ = validateAIConfig
	_ = validateRealtimeConfig
}

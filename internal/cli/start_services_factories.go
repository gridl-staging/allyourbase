// Package cli Factory functions for constructing and initializing services during CLI startup, including billing, email, SMS, and push notification providers.
package cli

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/push"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/jackc/pgx/v5/pgxpool"
)

type oauthProviderModeConfigSetter interface {
	SetOAuthProviderModeConfig(auth.OAuthProviderModeConfig)
}

type edgeFuncRuntimeConfig struct {
	PoolSize                 int
	DefaultTimeout           time.Duration
	MaxRequestBodyBytes      int64
	FetchDomainAllowlist     []string
	MemoryLimitMB            int
	MaxConcurrentInvocations int
	CodeCacheSize            int
}

const totpEncryptionKeyDerivationLabel = "ayb-totp-encryption"

// buildEdgeFuncRuntimeConfig constructs the runtime configuration for edge functions with sensible defaults, applying overrides from the provided configuration.
func buildEdgeFuncRuntimeConfig(cfg *config.Config) edgeFuncRuntimeConfig {
	runtimeCfg := edgeFuncRuntimeConfig{
		PoolSize:                 1,
		DefaultTimeout:           edgefunc.DefaultTimeout,
		MaxRequestBodyBytes:      server.MaxEdgeFuncBodySize,
		MemoryLimitMB:            128,
		MaxConcurrentInvocations: 50,
		CodeCacheSize:            256,
	}
	if cfg == nil {
		return runtimeCfg
	}
	if cfg.EdgeFunctions.PoolSize > 0 {
		runtimeCfg.PoolSize = cfg.EdgeFunctions.PoolSize
	}
	if cfg.EdgeFunctions.DefaultTimeoutMs > 0 {
		runtimeCfg.DefaultTimeout = time.Duration(cfg.EdgeFunctions.DefaultTimeoutMs) * time.Millisecond
	}
	if cfg.EdgeFunctions.MaxRequestBodyBytes > 0 {
		runtimeCfg.MaxRequestBodyBytes = cfg.EdgeFunctions.MaxRequestBodyBytes
	}
	if len(cfg.EdgeFunctions.FetchDomainAllowlist) > 0 {
		runtimeCfg.FetchDomainAllowlist = append([]string(nil), cfg.EdgeFunctions.FetchDomainAllowlist...)
	}
	if cfg.EdgeFunctions.MemoryLimitMB > 0 {
		runtimeCfg.MemoryLimitMB = cfg.EdgeFunctions.MemoryLimitMB
	}
	if cfg.EdgeFunctions.MaxConcurrentInvocations > 0 {
		runtimeCfg.MaxConcurrentInvocations = cfg.EdgeFunctions.MaxConcurrentInvocations
	}
	if cfg.EdgeFunctions.CodeCacheSize > 0 {
		runtimeCfg.CodeCacheSize = cfg.EdgeFunctions.CodeCacheSize
	}
	return runtimeCfg
}

func applyOAuthProviderModeConfig(target oauthProviderModeConfigSetter, cfg *config.Config) {
	if target == nil || cfg == nil || !cfg.Auth.OAuthProviderMode.Enabled {
		return
	}
	target.SetOAuthProviderModeConfig(auth.OAuthProviderModeConfig{
		AccessTokenDuration:  time.Duration(cfg.Auth.OAuthProviderMode.AccessTokenDuration) * time.Second,
		RefreshTokenDuration: time.Duration(cfg.Auth.OAuthProviderMode.RefreshTokenDuration) * time.Second,
		AuthCodeDuration:     time.Duration(cfg.Auth.OAuthProviderMode.AuthCodeDuration) * time.Second,
	})
}

// buildBillingService constructs a billing service based on configuration, returning a Stripe-backed service if Stripe is configured as the provider, otherwise returning a noop service.
func buildBillingService(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger) billing.BillingService {
	if cfg == nil || pool == nil {
		return billing.NewNoopBillingService()
	}
	if cfg.Billing.Provider != "stripe" {
		return billing.NewNoopBillingService()
	}

	stripeAdapter := billing.NewStripeHTTPAdapter(
		cfg.Billing.StripeSecretKey,
		billing.StripeAdapterConfig{},
	)
	return billing.NewStripeBillingService(
		billing.NewStore(pool),
		cfg.Billing,
		stripeAdapter,
		logger,
	)
}

func buildSupportService(cfg *config.Config, pool *pgxpool.Pool) support.SupportService {
	if cfg == nil || !cfg.Support.Enabled || pool == nil {
		return support.NewNoopSupportService()
	}
	return support.NewService(support.NewStore(pool))
}

// wireBillingUsageSyncJobs registers billing usage synchronization jobs and their schedules with the job service when a Stripe billing provider is configured.
func wireBillingUsageSyncJobs(
	ctx context.Context,
	cfg *config.Config,
	jobSvc *jobs.Service,
	billingSvc billing.BillingService,
	pool *pgxpool.Pool,
	logger *slog.Logger,
) {
	if cfg == nil || jobSvc == nil || billingSvc == nil || pool == nil {
		return
	}
	if cfg.Billing.Provider != "stripe" {
		return
	}
	registerBillingUsageSyncHandler(jobSvc, billingSvc, pool)
	if err := registerBillingUsageSyncSchedule(ctx, jobSvc, cfg.Billing.UsageSyncIntervalSecs); err != nil {
		if logger != nil {
			logger.Warn("failed to register billing usage sync schedule", "error", err)
		}
	}
}

func resolveTOTPEncryptionKey(authCfg config.AuthConfig) ([]byte, error) {
	if raw := strings.TrimSpace(authCfg.EncryptionKey); raw != "" {
		return parseConfiguredTOTPEncryptionKey(raw)
	}
	return deriveTOTPEncryptionKey(authCfg.JWTSecret)
}

// parseConfiguredTOTPEncryptionKey decodes and validates a TOTP encryption key from a hex or base64 encoded string, returning an error if the decoded key is not exactly 32 bytes.
func parseConfiguredTOTPEncryptionKey(raw string) ([]byte, error) {
	if decodedHex, err := hex.DecodeString(raw); err == nil {
		if len(decodedHex) != 32 {
			return nil, fmt.Errorf("auth.encryption_key decoded from hex must be 32 bytes, got %d", len(decodedHex))
		}
		return decodedHex, nil
	}

	base64Decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}
	for _, decode := range base64Decoders {
		decoded, err := decode(raw)
		if err != nil {
			continue
		}
		if len(decoded) != 32 {
			return nil, fmt.Errorf("auth.encryption_key decoded from base64 must be 32 bytes, got %d", len(decoded))
		}
		return decoded, nil
	}

	return nil, fmt.Errorf("auth.encryption_key must be a 32-byte key encoded as hex or base64")
}

func deriveTOTPEncryptionKey(jwtSecret string) ([]byte, error) {
	if strings.TrimSpace(jwtSecret) == "" {
		return nil, fmt.Errorf("auth.jwt_secret is required to derive TOTP encryption key")
	}
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(totpEncryptionKeyDerivationLabel))
	return mac.Sum(nil), nil
}

// buildMailer constructs a mailer based on configuration, supporting SMTP, webhook, and log-based email backends with appropriate defaults.
func buildMailer(cfg *config.Config, logger *slog.Logger) mailer.Mailer {
	switch cfg.Email.Backend {
	case "smtp":
		port := cfg.Email.SMTP.Port
		if port == 0 {
			port = 587
		}
		return mailer.NewSMTPMailer(mailer.SMTPConfig{
			Host:       cfg.Email.SMTP.Host,
			Port:       port,
			Username:   cfg.Email.SMTP.Username,
			Password:   cfg.Email.SMTP.Password,
			From:       cfg.Email.From,
			FromName:   cfg.Email.FromName,
			TLS:        cfg.Email.SMTP.TLS,
			AuthMethod: cfg.Email.SMTP.AuthMethod,
		})
	case "webhook":
		timeout := time.Duration(cfg.Email.Webhook.Timeout) * time.Second
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		return mailer.NewWebhookMailer(mailer.WebhookConfig{
			URL:     cfg.Email.Webhook.URL,
			Secret:  cfg.Email.Webhook.Secret,
			Timeout: timeout,
		})
	default:
		return mailer.NewLogMailer(logger)
	}
}

// buildSMSProvider constructs an SMS provider based on configuration, supporting multiple providers including Twilio, Plivo, Telnyx, MSG91, AWS SNS, Vonage, and webhook, falling back to a log provider.
func buildSMSProvider(cfg *config.Config, logger *slog.Logger) sms.Provider {
	switch cfg.Auth.SMSProvider {
	case "twilio":
		return sms.NewTwilioProvider(cfg.Auth.TwilioSID, cfg.Auth.TwilioToken, cfg.Auth.TwilioFrom, "")
	case "plivo":
		return sms.NewPlivoProvider(cfg.Auth.PlivoAuthID, cfg.Auth.PlivoAuthToken, cfg.Auth.PlivoFrom, "")
	case "telnyx":
		return sms.NewTelnyxProvider(cfg.Auth.TelnyxAPIKey, cfg.Auth.TelnyxFrom, "")
	case "msg91":
		return sms.NewMSG91Provider(cfg.Auth.MSG91AuthKey, cfg.Auth.MSG91TemplateID, "")
	case "sns":
		publisher, err := newSNSPublisher(cfg.Auth.AWSRegion)
		if err != nil {
			logger.Error("failed to create AWS SNS client, falling back to log provider", "error", err)
			return sms.NewLogProvider(logger)
		}
		return sms.NewSNSProvider(publisher)
	case "vonage":
		return sms.NewVonageProvider(cfg.Auth.VonageAPIKey, cfg.Auth.VonageAPISecret, cfg.Auth.VonageFrom, "")
	case "webhook":
		return sms.NewWebhookProvider(cfg.Auth.SMSWebhookURL, cfg.Auth.SMSWebhookSecret)
	default:
		return sms.NewLogProvider(logger)
	}
}

// buildPushProviders constructs a map of push notification providers including FCM and APNS, falling back to log providers if provider initialization fails.
func buildPushProviders(cfg *config.Config, logger *slog.Logger) map[string]push.Provider {
	providers := map[string]push.Provider{}
	logProvider := push.NewLogProvider(logger)

	providers[push.ProviderFCM] = logProvider
	if strings.TrimSpace(cfg.Push.FCM.CredentialsFile) != "" {
		p, err := push.NewFCMProvider(cfg.Push.FCM.CredentialsFile, "")
		if err != nil {
			logger.Error("failed to create FCM push provider, falling back to log provider", "error", err)
		} else {
			providers[push.ProviderFCM] = p
		}
	}

	providers[push.ProviderAPNS] = logProvider
	if strings.TrimSpace(cfg.Push.APNS.KeyFile) != "" &&
		strings.TrimSpace(cfg.Push.APNS.TeamID) != "" &&
		strings.TrimSpace(cfg.Push.APNS.KeyID) != "" &&
		strings.TrimSpace(cfg.Push.APNS.BundleID) != "" {
		p, err := push.NewAPNSProvider(push.APNSConfig{
			KeyFile:     cfg.Push.APNS.KeyFile,
			TeamID:      cfg.Push.APNS.TeamID,
			KeyID:       cfg.Push.APNS.KeyID,
			BundleID:    cfg.Push.APNS.BundleID,
			Environment: strings.TrimSpace(cfg.Push.APNS.Environment),
		})
		if err != nil {
			logger.Error("failed to create APNS push provider, falling back to log provider", "error", err)
		} else {
			providers[push.ProviderAPNS] = p
		}
	}

	return providers
}

func pushProviderNames(providers map[string]push.Provider) []string {
	names := make([]string, 0, len(providers))
	for name, provider := range providers {
		if provider == nil {
			continue
		}
		names = append(names, strings.ToLower(strings.TrimSpace(name)))
	}
	sort.Strings(names)
	return names
}

type scheduleUpserter interface {
	UpsertSchedule(ctx context.Context, sched *jobs.Schedule) (*jobs.Schedule, error)
}

// registerPushTokenCleanupSchedule registers a daily scheduled job to clean up expired push tokens at 2 AM UTC.
func registerPushTokenCleanupSchedule(ctx context.Context, store scheduleUpserter, logger *slog.Logger) {
	if store == nil {
		return
	}

	const (
		scheduleName = "push_token_cleanup_daily"
		cronExpr     = "0 2 * * *"
		timezone     = "UTC"
	)

	nextRunAt, err := jobs.CronNextTime(cronExpr, timezone, time.Now())
	if err != nil {
		logger.Error("failed to compute push token cleanup next run", "error", err)
		return
	}

	_, err = store.UpsertSchedule(ctx, &jobs.Schedule{
		Name:        scheduleName,
		JobType:     push.JobTypePushTokenClean,
		CronExpr:    cronExpr,
		Timezone:    timezone,
		Enabled:     true,
		MaxAttempts: 3,
		NextRunAt:   &nextRunAt,
	})
	if err != nil {
		logger.Error("failed to register push token cleanup schedule", "error", err)
	}
}

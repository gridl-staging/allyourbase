// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/storage_cdn_provider.go.
package server

import (
	"context"
	"log/slog"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/storage"
)

const cloudFrontDefaultRegion = "us-east-1"

var newStorageCDNProvider = buildStorageCDNProvider

// TODO: Document buildStorageCDNProvider.
func buildStorageCDNProvider(cfg config.CDNConfig, logger *slog.Logger) storage.CDNProvider {
	switch cfg.NormalizedProvider() {
	case "":
		return storage.NopCDNProvider{}
	case "cloudflare":
		return storage.NewCloudflareCDNProvider(storage.CloudflareCDNOptions{
			ZoneID:   cfg.Cloudflare.ZoneID,
			APIToken: cfg.Cloudflare.APIToken,
		})
	case "cloudfront":
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cloudFrontDefaultRegion))
		if err != nil {
			if logger != nil {
				logger.Warn("failed to initialize cloudfront CDN provider; falling back to nop", "error", err)
			}
			return storage.NopCDNProvider{}
		}
		return storage.NewCloudFrontCDNProvider(storage.CloudFrontCDNOptions{
			DistributionID: cfg.CloudFront.DistributionID,
			Client:         cloudfront.NewFromConfig(awsCfg),
		})
	case "webhook":
		return storage.NewWebhookCDNProvider(storage.WebhookCDNOptions{
			Endpoint:      cfg.Webhook.Endpoint,
			SigningSecret: cfg.Webhook.SigningSecret,
		})
	default:
		if logger != nil {
			logger.Warn("invalid storage CDN provider configured; falling back to nop", "provider", cfg.Provider)
		}
		return storage.NopCDNProvider{}
	}
}

package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/smithy-go"

	"github.com/allyourbase/ayb/internal/backoff"
)

const (
	cloudfrontProviderName        = "cloudfront"
	cloudfrontPurgeURLsBatchLimit = 3000
	cloudfrontWildcardPath        = "/*"
)

var cloudfrontCallerReferenceCounter uint64

var retryableCloudFrontErrorCodes = map[string]struct{}{
	"priorrequestnotcomplete":        {},
	"requestlimitexceeded":           {},
	"requestthrottled":               {},
	"slowdown":                       {},
	"throttledexception":             {},
	"throttling":                     {},
	"throttlingexception":            {},
	"toomanyinvalidationsinprogress": {},
	"toomanyrequestsexception":       {},
}

// cloudfrontCreateInvalidationClient is a narrow seam for mocking AWS SDK calls.
type cloudfrontCreateInvalidationClient interface {
	CreateInvalidation(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error)
}

// CloudFrontCDNOptions captures construction dependencies for a CloudFront provider.
type CloudFrontCDNOptions struct {
	DistributionID string
	Client         cloudfrontCreateInvalidationClient
	MaxRetries     int
	BackoffConfig  backoff.Config
}

// CloudFrontCDNProvider invalidates objects via AWS CloudFront invalidation.
type CloudFrontCDNProvider struct {
	distributionID string
	client         cloudfrontCreateInvalidationClient
	maxRetries     int
	backoffConfig  backoff.Config
}

// NewCloudFrontCDNProvider constructs a CloudFrontCDNProvider with defaults.
func NewCloudFrontCDNProvider(opts CloudFrontCDNOptions) *CloudFrontCDNProvider {
	maxRetries, backoffConfig := resolveCDNRetrySettings(opts.MaxRetries, opts.BackoffConfig)

	return &CloudFrontCDNProvider{
		distributionID: strings.TrimSpace(opts.DistributionID),
		client:         opts.Client,
		maxRetries:     maxRetries,
		backoffConfig:  backoffConfig,
	}
}

// Name returns the provider name used for logging and diagnostics.
func (p *CloudFrontCDNProvider) Name() string {
	return cloudfrontProviderName
}

// PurgeURLs invalidates public URLs by converting them to CloudFront paths.
func (p *CloudFrontCDNProvider) PurgeURLs(ctx context.Context, publicURLs []string) error {
	urls := sanitizePublicURLs(publicURLs)
	if len(urls) == 0 {
		return nil
	}

	paths := make([]string, 0, len(urls))
	for _, publicURL := range urls {
		path, err := publicURLToCloudFrontInvalidationPath(publicURL)
		if err != nil {
			return err
		}
		paths = append(paths, path)
	}

	for _, pathChunk := range chunkStrings(paths, cloudfrontPurgeURLsBatchLimit) {
		if err := p.purgePaths(ctx, pathChunk); err != nil {
			return err
		}
	}

	return nil
}

// PurgeAll creates a wildcard invalidation request for the full distribution.
func (p *CloudFrontCDNProvider) PurgeAll(ctx context.Context) error {
	return p.purgePaths(ctx, []string{cloudfrontWildcardPath})
}

// purgePaths submits an invalidation request with retry behavior.
func (p *CloudFrontCDNProvider) purgePaths(ctx context.Context, paths []string) error {
	if p.client == nil {
		return fmt.Errorf("cloudfront client is required")
	}
	if len(paths) == 0 {
		return nil
	}
	if strings.TrimSpace(p.distributionID) == "" {
		return fmt.Errorf("cloudfront distribution ID is required")
	}

	input := &cloudfront.CreateInvalidationInput{
		DistributionId: &p.distributionID,
		InvalidationBatch: &types.InvalidationBatch{
			Paths: &types.Paths{
				Quantity: aws.Int32(int32(len(paths))),
				Items:    paths,
			},
		},
	}

	return doWithRetry(
		ctx,
		p.maxRetries,
		p.backoffConfig,
		p.isRetryableCloudFrontError,
		func(ctx context.Context) error {
			batch := input
			batch.InvalidationBatch.CallerReference = aws.String(newCloudFrontCallerReference())
			_, err := p.client.CreateInvalidation(ctx, batch)
			if err != nil {
				return err
			}
			return nil
		},
	)
}

// isRetryableCloudFrontError classifies transient transport/service failures.
func (p *CloudFrontCDNProvider) isRetryableCloudFrontError(err error) bool {
	if isRetryableHTTPAdapterError(err) {
		return true
	}

	var apiErr interface {
		ErrorCode() string
		ErrorFault() smithy.ErrorFault
	}
	if errors.As(err, &apiErr) {
		errorCode := normalizeCloudFrontErrorCode(apiErr.ErrorCode())
		if errorCode == "accessdenied" || errorCode == "accessdeniedexception" {
			return false
		}
		if _, ok := retryableCloudFrontErrorCodes[errorCode]; ok {
			return true
		}
		return apiErr.ErrorFault() == smithy.FaultServer
	}

	return false
}

func normalizeCloudFrontErrorCode(errorCode string) string {
	errorCode = strings.TrimSpace(strings.ToLower(errorCode))
	return strings.ReplaceAll(errorCode, " ", "")
}

// publicURLToCloudFrontInvalidationPath converts a public URL to an invalidation path.
func publicURLToCloudFrontInvalidationPath(publicURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil {
		return "", fmt.Errorf("parsing public URL %q: %w", publicURL, err)
	}

	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}

	if parsed.RawQuery != "" {
		path = path + "?" + parsed.RawQuery
	}

	return path, nil
}

// newCloudFrontCallerReference generates unique identifiers for invalidation requests.
func newCloudFrontCallerReference() string {
	return fmt.Sprintf("ayb-cloudfront-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&cloudfrontCallerReferenceCounter, 1))
}

var _ CDNProvider = (*CloudFrontCDNProvider)(nil)

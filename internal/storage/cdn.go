package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	sharedbackoff "github.com/allyourbase/ayb/internal/backoff"
)

// CDNProvider performs cache invalidation operations for a storage CDN.
type CDNProvider interface {
	Name() string
	PurgeURLs(ctx context.Context, publicURLs []string) error
	PurgeAll(ctx context.Context) error
}

const (
	nopCDNProviderName = "nop"
)

const (
	cdnDefaultMaxRetries = 3
)

var cdnDefaultBackoffConfig = sharedbackoff.Config{
	Base: 250 * time.Millisecond,
	Cap:  5 * time.Second,
}

// resolveCDNRetrySettings applies shared zero-value defaults for adapter retry options.
func resolveCDNRetrySettings(maxRetries int, backoffConfig sharedbackoff.Config) (int, sharedbackoff.Config) {
	if maxRetries <= 0 {
		maxRetries = cdnDefaultMaxRetries
	}
	if backoffConfig.Base <= 0 {
		backoffConfig.Base = cdnDefaultBackoffConfig.Base
	}
	if backoffConfig.Cap <= 0 {
		backoffConfig.Cap = cdnDefaultBackoffConfig.Cap
	}
	return maxRetries, backoffConfig
}

// HTTPDoer matches the subset of *http.Client used by CDN adapters for testability.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NopCDNProvider performs no-op purges.
//
// It keeps the caller contract stable for deployments that only use URL rewriting.
type NopCDNProvider struct{}

// Name returns a stable provider name.
func (NopCDNProvider) Name() string { return nopCDNProviderName }

// PurgeURLs succeeds without side effects.
func (NopCDNProvider) PurgeURLs(_ context.Context, _ []string) error { return nil }

// PurgeAll succeeds without side effects.
func (NopCDNProvider) PurgeAll(_ context.Context) error { return nil }

// NormalizePublicURLs normalizes and deduplicates public URLs while preserving order.
func NormalizePublicURLs(publicURLs []string) []string {
	if len(publicURLs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(publicURLs))
	out := make([]string, 0, len(publicURLs))
	for _, publicURL := range publicURLs {
		clean := strings.TrimSpace(publicURL)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

// sanitizePublicURLs keeps existing internal call sites stable.
func sanitizePublicURLs(publicURLs []string) []string {
	return NormalizePublicURLs(publicURLs)
}

// chunkStrings splits a slice into batches of the requested size.
func chunkStrings(values []string, batchSize int) [][]string {
	if batchSize <= 0 || len(values) == 0 {
		return [][]string{}
	}

	chunks := make([][]string, 0, (len(values)+batchSize-1)/batchSize)
	for i := 0; i < len(values); i += batchSize {
		end := i + batchSize
		if end > len(values) {
			end = len(values)
		}
		chunk := make([]string, end-i)
		copy(chunk, values[i:end])
		chunks = append(chunks, chunk)
	}
	return chunks
}

// responseError captures response metadata with status body to support retry classification.
type responseError struct {
	statusCode int
	body       string
}

// Error returns a generic message describing the response error.
func (e *responseError) Error() string {
	if e == nil {
		return "cdn response error"
	}
	return fmt.Sprintf("cdn response error: status=%d body=%q", e.statusCode, e.body)
}

// statusCodeError extracts *responseError when available.
func statusCodeError(err error) (*responseError, bool) {
	var respErr *responseError
	if errors.As(err, &respErr) {
		return respErr, true
	}
	return nil, false
}

// extractHTTPStatusCode returns an HTTP status code if the error exposes it.
func extractHTTPStatusCode(err error) (int, bool) {
	var statusErr interface{ HTTPStatusCode() int }
	if errors.As(err, &statusErr) {
		return statusErr.HTTPStatusCode(), true
	}

	if respErr, ok := statusCodeError(err); ok {
		return respErr.statusCode, true
	}

	return 0, false
}

// readResponseBody reads and closes an HTTP response body, returning at most 4 KB.
func readResponseBody(resp *http.Response) (string, error) {
	if resp == nil || resp.Body == nil {
		return "", nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// isRetryableStatusCode returns true for transport-safe transient HTTP status codes.
func isRetryableStatusCode(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	return statusCode >= http.StatusInternalServerError && statusCode < 600
}

// isRetryableHTTPAdapterError covers shared transport and HTTP retry semantics.
func isRetryableHTTPAdapterError(err error) bool {
	if isRetryableTransportError(err) {
		return true
	}

	if statusCode, ok := extractHTTPStatusCode(err); ok {
		return isRetryableStatusCode(statusCode)
	}

	return false
}

// isRetryableTransportError reports transient network-level failures.
func isRetryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return true
		}
	}

	return false
}

// doWithRetry executes fn and retries on classification-matched errors.
func doWithRetry(
	ctx context.Context,
	maxRetries int,
	backoffConfig sharedbackoff.Config,
	isRetryable func(error) bool,
	fn func(context.Context) error,
) error {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if isRetryable == nil {
		isRetryable = func(error) bool { return false }
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		if attempt > 0 {
			delay := sharedbackoff.Exponential(attempt, backoffConfig)
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if !isRetryable(err) || attempt >= maxRetries {
			return err
		}
	}

	return lastErr
}

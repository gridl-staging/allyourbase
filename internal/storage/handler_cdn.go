// Package storage Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/storage/handler_cdn.go.
package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const defaultCDNPurgeTimeout = 5 * time.Second

type handlerMutations struct {
	upload                  func(context.Context, string, string, string, *string, io.Reader) (*Object, error)
	getObject               func(context.Context, string, string) (*Object, error)
	deleteObject            func(context.Context, string, string) error
	appendResumableUpload   func(context.Context, string, int64, *string, io.Reader) (*ResumableUpload, bool, error)
	finalizeResumableUpload func(context.Context, string, *string) (*Object, error)
	reserveQuota            func(context.Context, string, int64) error
	incrementUsage          func(context.Context, string, int64) error
	decrementUsage          func(context.Context, string, int64) error
}

// TODO: Document newHandlerMutations.
func newHandlerMutations(svc *Service) handlerMutations {
	if svc == nil {
		return handlerMutations{
			upload: func(_ context.Context, _, _, _ string, _ *string, _ io.Reader) (*Object, error) {
				return nil, fmt.Errorf("storage service is not configured")
			},
			getObject: func(_ context.Context, _, _ string) (*Object, error) {
				return nil, fmt.Errorf("storage service is not configured")
			},
			deleteObject: func(_ context.Context, _, _ string) error {
				return fmt.Errorf("storage service is not configured")
			},
			appendResumableUpload: func(_ context.Context, _ string, _ int64, _ *string, _ io.Reader) (*ResumableUpload, bool, error) {
				return nil, false, fmt.Errorf("storage service is not configured")
			},
			finalizeResumableUpload: func(_ context.Context, _ string, _ *string) (*Object, error) {
				return nil, fmt.Errorf("storage service is not configured")
			},
			reserveQuota: func(_ context.Context, _ string, _ int64) error {
				return fmt.Errorf("storage service is not configured")
			},
			incrementUsage: func(_ context.Context, _ string, _ int64) error {
				return fmt.Errorf("storage service is not configured")
			},
			decrementUsage: func(_ context.Context, _ string, _ int64) error {
				return fmt.Errorf("storage service is not configured")
			},
		}
	}

	return handlerMutations{
		upload:                  svc.Upload,
		getObject:               svc.GetObject,
		deleteObject:            svc.DeleteObject,
		appendResumableUpload:   svc.AppendResumableUpload,
		finalizeResumableUpload: svc.FinalizeResumableUpload,
		reserveQuota:            svc.ReserveQuota,
		incrementUsage:          svc.IncrementUsage,
		decrementUsage:          svc.DecrementUsage,
	}
}

type cdnPurgeCoordinator struct {
	provider CDNProvider
	logger   *slog.Logger
	timeout  time.Duration
}

func newCDNPurgeCoordinator(provider CDNProvider, logger *slog.Logger, timeout time.Duration) *cdnPurgeCoordinator {
	if provider == nil {
		provider = NopCDNProvider{}
	}
	if timeout <= 0 {
		timeout = defaultCDNPurgeTimeout
	}
	return &cdnPurgeCoordinator{provider: provider, logger: logger, timeout: timeout}
}

func (c *cdnPurgeCoordinator) setProvider(provider CDNProvider) {
	if provider == nil {
		provider = NopCDNProvider{}
	}
	c.provider = provider
}

func (c *cdnPurgeCoordinator) providerName() string {
	if c == nil || c.provider == nil {
		return nopCDNProviderName
	}
	return c.provider.Name()
}

func (c *cdnPurgeCoordinator) enqueuePurgeURLs(parent context.Context, publicURLs []string) {
	if c == nil {
		return
	}
	provider := c.provider
	if provider == nil {
		provider = NopCDNProvider{}
	}
	go c.run(parent, provider.Name(), func(ctx context.Context) error {
		return provider.PurgeURLs(ctx, publicURLs)
	})
}

func (c *cdnPurgeCoordinator) enqueuePurgeAll(parent context.Context) {
	if c == nil {
		return
	}
	provider := c.provider
	if provider == nil {
		provider = NopCDNProvider{}
	}
	go c.run(parent, provider.Name(), func(ctx context.Context) error {
		return provider.PurgeAll(ctx)
	})
}

func (c *cdnPurgeCoordinator) run(parent context.Context, providerName string, fn func(context.Context) error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), c.timeout)
	defer cancel()
	if err := fn(ctx); err != nil && c.logger != nil {
		c.logger.Error("cdn purge failed", "provider", providerName, "error", err)
	}
}

func objectWasOverwritten(obj *Object) bool {
	if obj == nil {
		return false
	}
	return !obj.CreatedAt.Equal(obj.UpdatedAt)
}

func (h *Handler) enqueueObjectURLPurge(r *http.Request, bucket, name string) {
	publicURL := RewritePublicURL(h.publicObjectURL(r, bucket, name), h.cdnURL)
	h.cdnPurgeCoordinator.enqueuePurgeURLs(r.Context(), []string{publicURL})
}

func (h *Handler) SetCDNProvider(provider CDNProvider) {
	h.cdnPurgeCoordinator.setProvider(provider)
}

func (h *Handler) CDNProviderName() string {
	return h.cdnPurgeCoordinator.providerName()
}

func (h *Handler) EnqueuePurgeURLs(ctx context.Context, publicURLs []string) {
	h.cdnPurgeCoordinator.enqueuePurgeURLs(ctx, publicURLs)
}

func (h *Handler) EnqueuePurgeAll(ctx context.Context) {
	h.cdnPurgeCoordinator.enqueuePurgeAll(ctx)
}

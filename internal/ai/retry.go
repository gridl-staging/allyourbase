// Package ai Provides wrappers that add retry and timeout logic to AI providers. Implements exponential backoff with jitter for transient failures.
package ai

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"sync"
	"time"

	sharedbackoff "github.com/allyourbase/ayb/internal/backoff"
)

var retryBackoffConfig = sharedbackoff.Config{
	Base: time.Second,
	Cap:  30 * time.Second,
	Jitter: func(delay time.Duration) time.Duration {
		return time.Duration(rand.Int64N(int64(delay / 2)))
	},
}

func withRetryTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// RetryProvider wraps a Provider with timeout and retry logic.
type RetryProvider struct {
	inner      Provider
	maxRetries int
	timeout    time.Duration
}

// NewRetryProvider wraps p with retry and per-call timeout behavior.
// Optional provider interfaces (embedding, streaming) are preserved.
func NewRetryProvider(p Provider, maxRetries int, timeout time.Duration) Provider {
	rp := &RetryProvider{inner: p, maxRetries: maxRetries, timeout: timeout}
	_, hasEmbedding := p.(EmbeddingProvider)
	_, hasStreaming := p.(StreamingProvider)
	switch {
	case hasEmbedding && hasStreaming:
		return &retryEmbeddingStreamingProvider{RetryProvider: rp}
	case hasEmbedding:
		return &retryEmbeddingProvider{RetryProvider: rp}
	case hasStreaming:
		return &retryStreamingProvider{RetryProvider: rp}
	default:
		return rp
	}
}

// GenerateText calls the wrapped provider with exponential backoff retry and per-call timeout, retrying only on errors marked retryable by IsRetryable, up to maxRetries times.
func (rp *RetryProvider) GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= rp.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := sharedbackoff.Exponential(attempt, retryBackoffConfig)
			select {
			case <-ctx.Done():
				return GenerateTextResponse{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		callCtx, cancel := withRetryTimeout(ctx, rp.timeout)

		resp, err := rp.inner.GenerateText(callCtx, req)
		cancel()
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Only retry on retryable errors.
		var pe *ProviderError
		if errors.As(err, &pe) && pe.IsRetryable() {
			continue
		}
		// Not retryable — return immediately.
		return GenerateTextResponse{}, err
	}

	return GenerateTextResponse{}, lastErr
}

// retryEmbeddingProvider extends RetryProvider with EmbeddingProvider support.
type retryEmbeddingProvider struct {
	*RetryProvider
}

// GenerateEmbedding calls the wrapped embedding provider with exponential backoff retry and per-call timeout, retrying only on errors marked retryable by IsRetryable, up to maxRetries times.
func (rp *retryEmbeddingProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	ep := rp.inner.(EmbeddingProvider) // safe: checked in NewRetryProvider
	var lastErr error

	for attempt := 0; attempt <= rp.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := sharedbackoff.Exponential(attempt, retryBackoffConfig)
			select {
			case <-ctx.Done():
				return EmbeddingResponse{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		callCtx, cancel := withRetryTimeout(ctx, rp.timeout)

		resp, err := ep.GenerateEmbedding(callCtx, req)
		cancel()
		if err == nil {
			return resp, nil
		}

		lastErr = err

		var pe *ProviderError
		if errors.As(err, &pe) && pe.IsRetryable() {
			continue
		}
		return EmbeddingResponse{}, err
	}

	return EmbeddingResponse{}, lastErr
}

// retryStreamingProvider extends RetryProvider with StreamingProvider support.
type retryStreamingProvider struct {
	*RetryProvider
}

// GenerateTextStream retries stream setup errors using the same retry policy as GenerateText.
func (rp *retryStreamingProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	sp := rp.inner.(StreamingProvider) // safe: checked in NewRetryProvider
	var lastErr error

	for attempt := 0; attempt <= rp.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := sharedbackoff.Exponential(attempt, retryBackoffConfig)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		callCtx, cancel := withRetryTimeout(ctx, rp.timeout)
		reader, err := sp.GenerateTextStream(callCtx, req)
		if err == nil {
			return newContextManagedReadCloser(reader, cancel), nil
		}
		cancel()
		lastErr = err

		var pe *ProviderError
		if errors.As(err, &pe) && pe.IsRetryable() {
			continue
		}
		return nil, err
	}

	return nil, lastErr
}

// retryEmbeddingStreamingProvider preserves both optional interfaces.
type retryEmbeddingStreamingProvider struct {
	*RetryProvider
}

func (rp *retryEmbeddingStreamingProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	return (&retryEmbeddingProvider{RetryProvider: rp.RetryProvider}).GenerateEmbedding(ctx, req)
}

func (rp *retryEmbeddingStreamingProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	return (&retryStreamingProvider{RetryProvider: rp.RetryProvider}).GenerateTextStream(ctx, req)
}

type contextManagedReadCloser struct {
	inner  io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func newContextManagedReadCloser(inner io.ReadCloser, cancel context.CancelFunc) io.ReadCloser {
	return &contextManagedReadCloser{
		inner:  inner,
		cancel: cancel,
	}
}

func (r *contextManagedReadCloser) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if err != nil {
		r.finish()
	}
	return n, err
}

func (r *contextManagedReadCloser) Close() error {
	err := r.inner.Close()
	r.finish()
	return err
}

func (r *contextManagedReadCloser) finish() {
	r.once.Do(r.cancel)
}

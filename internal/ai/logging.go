// Package ai LoggingProvider and loggingEmbeddingProvider wrap AI providers to record all GenerateText and GenerateEmbedding calls with metrics and errors to a LogStore.
package ai

import (
	"context"
	"io"
	"time"
)

// LoggingProvider wraps a Provider and records every call to a LogStore.
type LoggingProvider struct {
	inner        Provider
	providerName string
	store        LogStore
}

// NewLoggingProvider wraps inner and logs every GenerateText call.
// Optional provider interfaces (embedding, streaming) are preserved.
func NewLoggingProvider(inner Provider, providerName string, store LogStore) Provider {
	lp := &LoggingProvider{inner: inner, providerName: providerName, store: store}
	_, hasEmbedding := inner.(EmbeddingProvider)
	_, hasStreaming := inner.(StreamingProvider)
	switch {
	case hasEmbedding && hasStreaming:
		return &loggingEmbeddingStreamingProvider{LoggingProvider: lp}
	case hasEmbedding:
		return &loggingEmbeddingProvider{LoggingProvider: lp}
	case hasStreaming:
		return &loggingStreamingProvider{LoggingProvider: lp}
	default:
		return lp
	}
}

// GenerateText calls the wrapped provider's GenerateText and logs the call with execution duration, token usage, and status to the LogStore. Logging failures do not affect the returned response or error.
func (lp *LoggingProvider) GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	start := time.Now()
	resp, err := lp.inner.GenerateText(ctx, req)
	duration := time.Since(start)

	log := CallLog{
		Provider:     lp.providerName,
		Model:        req.Model,
		DurationMs:   int(duration.Milliseconds()),
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}

	if err != nil {
		log.Status = "error"
		log.ErrorMessage = err.Error()
	} else {
		log.Status = "success"
		log.Model = resp.Model // use the model returned by the provider
	}

	// Fire-and-forget: don't fail the main call if logging fails.
	_ = lp.store.Insert(ctx, log)

	return resp, err
}

// loggingEmbeddingProvider extends LoggingProvider with EmbeddingProvider support.
type loggingEmbeddingProvider struct {
	*LoggingProvider
}

// GenerateEmbedding calls the wrapped provider's GenerateEmbedding and logs the call with execution duration and token usage to the LogStore. Logging failures do not affect the returned response or error.
func (lp *loggingEmbeddingProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	ep := lp.inner.(EmbeddingProvider) // safe: checked in NewLoggingProvider
	start := time.Now()
	resp, err := ep.GenerateEmbedding(ctx, req)
	duration := time.Since(start)

	log := CallLog{
		Provider:    lp.providerName,
		Model:       req.Model,
		DurationMs:  int(duration.Milliseconds()),
		InputTokens: resp.Usage.InputTokens,
	}

	if err != nil {
		log.Status = "error"
		log.ErrorMessage = err.Error()
	} else {
		log.Status = "success"
		log.Model = resp.Model
	}

	_ = lp.store.Insert(ctx, log)

	return resp, err
}

// loggingStreamingProvider extends LoggingProvider with StreamingProvider support.
type loggingStreamingProvider struct {
	*LoggingProvider
}

// GenerateTextStream logs stream setup calls. Provider-side streaming token
// usage is not currently available in the common interface, so usage metrics
// are recorded as zero.
func (lp *loggingStreamingProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	sp := lp.inner.(StreamingProvider) // safe: checked in NewLoggingProvider
	start := time.Now()
	reader, err := sp.GenerateTextStream(ctx, req)
	duration := time.Since(start)

	log := CallLog{
		Provider:   lp.providerName,
		Model:      req.Model,
		DurationMs: int(duration.Milliseconds()),
	}
	if err != nil {
		log.Status = "error"
		log.ErrorMessage = err.Error()
	} else {
		log.Status = "success"
	}
	_ = lp.store.Insert(ctx, log)
	return reader, err
}

// loggingEmbeddingStreamingProvider preserves both optional interfaces.
type loggingEmbeddingStreamingProvider struct {
	*LoggingProvider
}

func (lp *loggingEmbeddingStreamingProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	return (&loggingEmbeddingProvider{LoggingProvider: lp.LoggingProvider}).GenerateEmbedding(ctx, req)
}

func (lp *loggingEmbeddingStreamingProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	return (&loggingStreamingProvider{LoggingProvider: lp.LoggingProvider}).GenerateTextStream(ctx, req)
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

type semanticRegressionEmbeddingProvider struct {
	embeddingCalls int
}

func (p *semanticRegressionEmbeddingProvider) GenerateText(_ context.Context, _ ai.GenerateTextRequest) (ai.GenerateTextResponse, error) {
	return ai.GenerateTextResponse{}, nil
}

func (p *semanticRegressionEmbeddingProvider) GenerateEmbedding(_ context.Context, _ ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	p.embeddingCalls++
	return ai.EmbeddingResponse{
		Embeddings: [][]float64{{0.1, 0.2, 0.3}},
		Model:      "test-embed",
	}, nil
}

// testHandlerWithEmbedder creates a handler with an embedFn for semantic query tests.
func testHandlerWithEmbedder(sc *schema.SchemaCache, fn EmbedFunc) http.Handler {
	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, nil, nil, nil, nil)
	h.ApplyOptions(WithEmbedder(fn))
	return h.Routes()
}

// --- handleSemanticQuery validation tests ---

func TestSemanticQuery_NoEmbedder(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "not configured")
}

func TestSemanticQuery_NoPgVector(t *testing.T) {
	t.Parallel()
	sc := vectorSchemaCache()
	sc.HasPgVector = false
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, nil
	}
	h := testHandlerWithEmbedder(sc, embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "pgvector")
}

func TestSemanticQuery_MutualExclusionWithNearest(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello&nearest=[0.1,0.2,0.3]", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "cannot use both")
}

func TestSemanticQuery_InvalidVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello&vector_column=title", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "not a vector column")
}

func TestSemanticQuery_AmbiguousVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/multi?semantic_query=hello", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "multiple vector columns")
}

// --- Successful flow tests ---

func TestSemanticQuery_AutoSelectColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		testutil.Equal(t, 1, len(texts))
		testutil.Equal(t, "hello world", texts[0])
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello+world", "")
	// With nil pool, validation passes but query fails at DB → 500
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSemanticQuery_ExplicitColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3, 0.4, 0.5}}, nil
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/multi?semantic_query=hello&vector_column=emb_b", "")
	// With nil pool, validation passes but query fails at DB → 500
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- Embedding error mapping tests ---

func TestSemanticQuery_EmbedTimeout(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, context.DeadlineExceeded
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusGatewayTimeout, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "timed out")
}

func TestSemanticQuery_EmbedAuthError(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, &ai.ProviderError{StatusCode: 401, Message: "invalid key", Provider: "openai"}
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusBadGateway, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "authentication failed")
}

func TestSemanticQuery_EmbedRateLimit(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, &ai.ProviderError{StatusCode: 429, Message: "rate limited", Provider: "openai"}
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "rate limited")
}

func TestSemanticQuery_EmbedDimensionMismatch(t *testing.T) {
	t.Parallel()
	// Return a 5-dim vector for a vector(3) column
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3, 0.4, 0.5}}, nil
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "embedding dimension mismatch")
	testutil.Contains(t, resp.Message, "5")
	testutil.Contains(t, resp.Message, "3")
}

func TestSemanticQuery_EmbedGenericError(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, errors.New("connection refused")
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusBadGateway, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "embedding provider error")
}

func TestSemanticQuery_EmbedEmptyResult(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{}, nil
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusBadGateway, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "returned no embedding")
}

// --- Mock-provider integration test ---

func TestSemanticQuery_FullFlowMockProvider(t *testing.T) {
	t.Parallel()
	var capturedVec []float64
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		if len(texts) == 0 {
			return nil, fmt.Errorf("no texts provided")
		}
		vec := []float64{0.1, 0.2, 0.3}
		capturedVec = vec
		return [][]float64{vec}, nil
	}

	sc := vectorSchemaCache()
	h := testHandlerWithEmbedder(sc, embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=find+similar+items&distance=l2", "")

	// Should reach DB execution (500 with nil pool, not 400)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify the embedding was generated with correct vector
	testutil.Equal(t, 3, len(capturedVec))

	// Verify the error is "internal error" (DB-level), not a validation error
	var errResp struct {
		Message string `json:"message"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)
	testutil.Equal(t, "internal error", errResp.Message)
}

func TestSemanticQuery_BreakerOpenError(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, &ai.BreakerOpenError{Provider: "openai"}
	}
	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "temporarily unavailable")
}

func TestSemanticQuery_ConfiguredDimensionMismatch(t *testing.T) {
	t.Parallel()
	embedCalled := false
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		embedCalled = true
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	sc := vectorSchemaCache()
	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, nil, nil, nil, nil)
	h.ApplyOptions(WithEmbedder(embedFn), WithConfiguredEmbeddingDimension(999))
	w := doRequest(h.Routes(), "GET", "/collections/documents?semantic_query=hello", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "semantic search misconfiguration")
	testutil.Equal(t, false, embedCalled)
}

func TestSemanticQuery_DefaultClosedBreakerRegression(t *testing.T) {
	t.Parallel()
	inner := &semanticRegressionEmbeddingProvider{}
	breakerWrapped := ai.NewBreakerProvider(inner, "regression-provider", ai.NewProviderHealthTracker(ai.BreakerConfig{}, nil))
	embedder := breakerWrapped.(ai.EmbeddingProvider)
	embedFn := func(ctx context.Context, texts []string) ([][]float64, error) {
		resp, err := embedder.GenerateEmbedding(ctx, ai.EmbeddingRequest{
			Model: "text-embedding-3-small",
			Input: texts,
		})
		if err != nil {
			return nil, err
		}
		return resp.Embeddings, nil
	}

	h := testHandlerWithEmbedder(vectorSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/documents?semantic_query=hello", "")

	// Regression assertion: with breaker default-closed, Stage 2 semantic flow
	// still reaches the DB layer (nil pool -> internal error), not 503.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, inner.embeddingCalls)
	resp := decodeError(t, w)
	testutil.Equal(t, "internal error", resp.Message)
}

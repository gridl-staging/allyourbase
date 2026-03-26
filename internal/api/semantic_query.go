package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/schema"
)

// handleSemanticQuery embeds query text via the configured AI provider and
// performs vector nearest-neighbor search on the resulting embedding.
// Called from handleList when the "semantic_query" param is present.
func (h *Handler) handleSemanticQuery(w http.ResponseWriter, r *http.Request, tbl *schema.Table,
	semanticQuery, vectorColName, distanceParam string, limit int,
	filterSQL string, filterArgs []any) {

	// Check that embedding is configured.
	if h.embedFn == nil {
		writeError(w, http.StatusBadRequest,
			"semantic search is not configured — no embedding provider available")
		return
	}

	// Check pgvector availability.
	sc := h.schema.Get()
	if sc == nil || !sc.HasPgVector {
		writeError(w, http.StatusBadRequest,
			"semantic search requires the pgvector extension")
		return
	}

	// Resolve vector column.
	col, err := findVectorColumn(tbl, vectorColName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate distance metric.
	metric, err := resolveDistanceMetric(distanceParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate configured embedding dimension before provider execution so
	// misconfiguration fails fast and deterministically.
	if h.configEmbeddingDim > 0 && col.VectorDim > 0 && h.configEmbeddingDim != col.VectorDim {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("semantic search misconfiguration: configured embedding dimension %d does not match column %q dimension %d",
				h.configEmbeddingDim, col.Name, col.VectorDim))
		return
	}

	// Embed the query text.
	embeddings, err := h.embedFn(r.Context(), []string{semanticQuery})
	if err != nil {
		mapEmbeddingError(w, err)
		return
	}
	if len(embeddings) == 0 {
		writeError(w, http.StatusBadGateway, "embedding provider returned no embedding")
		return
	}
	queryVec := embeddings[0]

	// Validate embedding dimension against the column.
	if col.VectorDim > 0 && len(queryVec) != col.VectorDim {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("embedding dimension mismatch: provider returned %d dimensions, column %q expects %d",
				len(queryVec), col.Name, col.VectorDim))
		return
	}

	h.executeNearestQuery(w, r, tbl, col, queryVec, metric, limit, filterSQL, filterArgs)
}

// mapEmbeddingError maps embedding provider errors to appropriate HTTP responses.
func mapEmbeddingError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "embedding provider timed out")
		return
	}
	if errors.Is(err, context.Canceled) {
		writeError(w, http.StatusGatewayTimeout, "embedding request canceled")
		return
	}

	var pe *ai.ProviderError
	if errors.As(err, &pe) {
		switch {
		case pe.StatusCode == 401 || pe.StatusCode == 403:
			writeError(w, http.StatusBadGateway, "embedding provider authentication failed")
		case pe.StatusCode == 429:
			writeError(w, http.StatusTooManyRequests, "embedding provider rate limited, retry later")
		default:
			writeError(w, http.StatusBadGateway, "embedding provider error: "+pe.Message)
		}
		return
	}
	var boe *ai.BreakerOpenError
	if errors.As(err, &boe) {
		writeError(w, http.StatusServiceUnavailable,
			"embedding provider temporarily unavailable due to circuit breaker")
		return
	}

	writeError(w, http.StatusBadGateway, "embedding provider error: "+err.Error())
}

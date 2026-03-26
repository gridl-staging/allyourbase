package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func hybridTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "articles",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", Position: 2, TypeName: "text"},
			{Name: "body", Position: 3, TypeName: "text"},
			{Name: "embedding", Position: 4, TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
		PrimaryKey: []string{"id"},
	}
}

func vectorNoTextTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "metrics",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "score", Position: 2, TypeName: "bigint"},
			{Name: "embedding", Position: 3, TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
		PrimaryKey: []string{"id"},
	}
}

func textOnlyTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "notes",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "content", Position: 2, TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}
}

func hybridSchemaCache() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.articles": hybridTable(),
			"public.metrics":  vectorNoTextTable(),
			"public.notes":    textOnlyTable(),
			"public.multi":    multiVectorTable(),
		},
		HasPgVector: true,
		Schemas:     []string{"public"},
	}
}

func testHandlerForHybrid(sc *schema.SchemaCache, fn EmbedFunc) http.Handler {
	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, nil, nil, nil, nil)
	h.ApplyOptions(WithEmbedder(fn))
	return h.Routes()
}

func TestExecuteFTSQuery_NilPoolBuildsSearchAndFilter(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(hybridSchemaCache()), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/collections/articles", nil)
	_, err := h.executeFTSQuery(req, hybridTable(), "hello", 5, "\"id\" = $1", []any{123})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "database pool is not configured")

	q, args, qerr := buildFTSHybridQuery(hybridTable(), "hello", 5, "\"id\" = $1", []any{123})
	testutil.NoError(t, qerr)
	testutil.Contains(t, q, " AS _fts_rank")
	testutil.Contains(t, q, "ORDER BY _fts_rank DESC")
	testutil.Contains(t, q, "\"id\" = $1")
	testutil.Equal(t, 3, len(args))
	testutil.Equal(t, 123, args[0])
	testutil.Equal(t, "hello", args[1])
	testutil.Equal(t, 5, args[2])
}

func TestExecuteVectorQuery_NilPoolBuildsFilterArgs(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(hybridSchemaCache()), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/collections/articles", nil)
	col, err := findVectorColumn(hybridTable(), "embedding")
	testutil.NoError(t, err)

	_, err = h.executeVectorQuery(req, hybridTable(), col, []float64{0.1, 0.2, 0.3}, "cosine", 5, "\"id\" = $1", []any{123})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "database pool is not configured")

	q, args, qerr := buildVectorHybridQuery(hybridTable(), col, []float64{0.1, 0.2, 0.3}, "cosine", 5, "\"id\" = $1", []any{123})
	testutil.NoError(t, qerr)
	testutil.Contains(t, q, " AS _distance")
	testutil.Contains(t, q, "\"id\" = $1")
	testutil.Equal(t, 3, len(args))
	testutil.Equal(t, 123, args[0])
	testutil.Equal(t, 5, args[2])
}

func TestHybridSearch_NoEmbedder(t *testing.T) {
	t.Parallel()
	h := testHandler(hybridSchemaCache())
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusNotImplemented, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "not configured")
}

func TestHybridSearch_NoPgVector(t *testing.T) {
	t.Parallel()
	sc := hybridSchemaCache()
	sc.HasPgVector = false
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) { return nil, nil }
	h := testHandlerForHybrid(sc, embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "pgvector")
}

func TestHybridSearch_NoTextColumns(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/metrics?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "no text columns")
}

func TestHybridSearch_NoVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/notes?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "no vector columns")
}

func TestHybridSearch_MutualExclusionWithNearest(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true&nearest=[0.1,0.2,0.3]", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "cannot combine")
}

func TestHybridSearch_MutualExclusionWithSemanticQuery(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true&semantic_query=world", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "cannot combine")
}

func TestHybridSearch_SemanticFalseIsRegularFTS(t *testing.T) {
	t.Parallel()
	h := testHandler(hybridSchemaCache())
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=false", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	resp := decodeError(t, w)
	testutil.Equal(t, "internal error", resp.Message)
}

func TestHybridSearch_EmbedTimeout(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) { return nil, context.DeadlineExceeded }
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusGatewayTimeout, w.Code)
}

func TestHybridSearch_EmbedAuthError(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, &ai.ProviderError{StatusCode: 401, Message: "invalid key", Provider: "openai"}
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHybridSearch_DimensionMismatch(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3, 0.4}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "dimension mismatch")
}

func TestHybridSearch_AutoSelectVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		testutil.Equal(t, 1, len(texts))
		testutil.Equal(t, "hello", texts[0])
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHybridSearch_ExplicitVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true&vector_column=embedding", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHybridSearch_AmbiguousVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/multi?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "multiple vector columns")
}

func TestHybridSearch_FullFlowMockProvider(t *testing.T) {
	t.Parallel()
	called := false
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		called = true
		if len(texts) == 0 {
			return nil, errors.New("no text")
		}
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=find+similar&semantic=true&distance=l2", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, true, called)
	resp := decodeError(t, w)
	testutil.Equal(t, "internal error", resp.Message)
}

func TestHybridSearch_EmptyResponseWhenBothSignalsEmpty(t *testing.T) {
	t.Parallel()
	merged := rrfMerge(nil, nil, []string{"id"}, defaultRRFConstant)
	testutil.Equal(t, 0, len(merged))
}

func TestHybridSearch_ResponseEnvelopeDefaults(t *testing.T) {
	t.Parallel()
	resp := ListResponse{Page: 1, PerPage: 20, TotalItems: 0, TotalPages: 1, Items: []map[string]any{}}
	b, err := json.Marshal(resp)
	testutil.NoError(t, err)
	testutil.Contains(t, string(b), "\"page\":1")
	testutil.Contains(t, string(b), "\"perPage\":20")
	testutil.Contains(t, string(b), "\"totalPages\":1")
}

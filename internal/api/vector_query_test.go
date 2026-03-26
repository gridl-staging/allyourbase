package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Test tables ---

func vectorTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "documents",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
			{Name: "embedding", TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
		PrimaryKey: []string{"id"},
	}
}

func multiVectorTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "multi",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "emb_a", TypeName: "vector(3)", IsVector: true, VectorDim: 3},
			{Name: "emb_b", TypeName: "vector(5)", IsVector: true, VectorDim: 5},
		},
		PrimaryKey: []string{"id"},
	}
}

func noVectorTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "plain",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}
}

// --- findVectorColumn tests ---

func TestFindVectorColumn_AutoSelect(t *testing.T) {
	t.Parallel()
	col, err := findVectorColumn(vectorTable(), "")
	testutil.NoError(t, err)
	testutil.Equal(t, "embedding", col.Name)
}

func TestFindVectorColumn_ExplicitValid(t *testing.T) {
	t.Parallel()
	col, err := findVectorColumn(vectorTable(), "embedding")
	testutil.NoError(t, err)
	testutil.Equal(t, "embedding", col.Name)
}

func TestFindVectorColumn_ExplicitNonVector(t *testing.T) {
	t.Parallel()
	_, err := findVectorColumn(vectorTable(), "title")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not a vector column")
}

func TestFindVectorColumn_ExplicitMissing(t *testing.T) {
	t.Parallel()
	_, err := findVectorColumn(vectorTable(), "nonexistent")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not found")
}

func TestFindVectorColumn_NoVectorColumns(t *testing.T) {
	t.Parallel()
	_, err := findVectorColumn(noVectorTable(), "")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "no vector columns")
}

func TestFindVectorColumn_AmbiguousMultiple(t *testing.T) {
	t.Parallel()
	_, err := findVectorColumn(multiVectorTable(), "")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "multiple vector columns")
	testutil.Contains(t, err.Error(), "vector_column")
}

func TestFindVectorColumn_ExplicitFromMultiple(t *testing.T) {
	t.Parallel()
	col, err := findVectorColumn(multiVectorTable(), "emb_b")
	testutil.NoError(t, err)
	testutil.Equal(t, "emb_b", col.Name)
}

// --- parseNearestVector tests ---

func TestParseNearestVector_Valid(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "embedding", IsVector: true, VectorDim: 3}
	vec, err := parseNearestVector("[0.1, 0.2, 0.3]", col)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, len(vec))
}

func TestParseNearestVector_EmptyArray(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "embedding", IsVector: true, VectorDim: 0}
	_, err := parseNearestVector("[]", col)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "must not be empty")
}

func TestParseNearestVector_MalformedJSON(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "embedding", IsVector: true, VectorDim: 3}
	_, err := parseNearestVector("not json", col)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "invalid nearest parameter")
}

func TestParseNearestVector_NonNumeric(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "embedding", IsVector: true, VectorDim: 3}
	_, err := parseNearestVector(`["a", "b", "c"]`, col)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "invalid nearest parameter")
}

func TestParseNearestVector_DimensionMismatch(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "embedding", IsVector: true, VectorDim: 3}
	_, err := parseNearestVector("[0.1, 0.2]", col)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "dimension mismatch")
	testutil.Contains(t, err.Error(), "2 dimensions")
	testutil.Contains(t, err.Error(), "expects 3")
}

func TestParseNearestVector_NoDimConstraint(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "embedding", IsVector: true, VectorDim: 0}
	vec, err := parseNearestVector("[1.0, 2.0, 3.0, 4.0, 5.0]", col)
	testutil.NoError(t, err)
	testutil.Equal(t, 5, len(vec))
}

// --- resolveDistanceMetric tests ---

func TestResolveDistanceMetric_Valid(t *testing.T) {
	t.Parallel()
	for _, m := range []string{"cosine", "l2", "inner_product"} {
		metric, err := resolveDistanceMetric(m)
		testutil.NoError(t, err)
		testutil.Equal(t, m, metric)
	}
}

func TestResolveDistanceMetric_EmptyDefaultsCosine(t *testing.T) {
	t.Parallel()
	metric, err := resolveDistanceMetric("")
	testutil.NoError(t, err)
	testutil.Equal(t, "cosine", metric)
}

func TestResolveDistanceMetric_Invalid(t *testing.T) {
	t.Parallel()
	_, err := resolveDistanceMetric("manhattan")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unsupported distance metric")
}

// --- handleList with nearest param (HTTP-level tests) ---

func vectorSchemaCache() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.documents": vectorTable(),
			"public.multi":     multiVectorTable(),
			"public.plain":     noVectorTable(),
		},
		HasPgVector: true,
		Schemas:     []string{"public"},
	}
}

func TestHandleList_NearestInvalidJSON(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/documents?nearest=notjson", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid nearest parameter")
}

func TestHandleList_NearestDimensionMismatch(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/documents?nearest=[0.1,0.2]", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "dimension mismatch")
}

func TestHandleList_NearestInvalidMetric(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/documents?nearest=[0.1,0.2,0.3]&distance=manhattan", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "unsupported distance metric")
}

func TestHandleList_NearestBadVectorColumn(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/documents?nearest=[0.1,0.2,0.3]&vector_column=title", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "not a vector column")
}

func TestHandleList_NearestNoVectorTable(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/plain?nearest=[0.1,0.2,0.3]", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no vector columns")
}

func TestHandleList_NearestAmbiguousTable(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/multi?nearest=[0.1,0.2,0.3]", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "multiple vector columns")
}

func TestHandleList_NearestEmptyVector(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	w := doRequest(h, "GET", "/collections/documents?nearest=[]", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "must not be empty")
}

// TestHandleList_NearestValidParams verifies that valid nearest params get past
// validation. Without a real DB pool the query will fail, but we verify the
// code path reaches the DB call (returns 500 "internal error", not 400).
func TestHandleList_NearestValidParams(t *testing.T) {
	t.Parallel()
	h := testHandler(vectorSchemaCache())
	nearest, _ := json.Marshal([]float64{0.1, 0.2, 0.3})
	w := doRequest(h, "GET", "/collections/documents?nearest="+string(nearest), "")
	// With nil pool we expect 500, not 400 — validation passed.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

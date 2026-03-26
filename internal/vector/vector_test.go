package vector

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
)

// --- Operator selection ---

func TestDistanceOperator(t *testing.T) {
	tests := []struct {
		metric string
		want   string
	}{
		{"cosine", "<=>"},
		{"l2", "<->"},
		{"inner_product", "<#>"},
	}
	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			op, err := DistanceOperator(tt.metric)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if op != tt.want {
				t.Errorf("DistanceOperator(%q) = %q, want %q", tt.metric, op, tt.want)
			}
		})
	}
}

func TestDistanceOperatorInvalid(t *testing.T) {
	_, err := DistanceOperator("euclidean")
	if err == nil {
		t.Fatal("expected error for invalid metric, got nil")
	}
}

func TestValidMetrics(t *testing.T) {
	metrics := ValidMetrics()
	if len(metrics) != 3 {
		t.Fatalf("expected 3 valid metrics, got %d", len(metrics))
	}
	seen := map[string]bool{}
	for _, m := range metrics {
		seen[m] = true
	}
	for _, want := range []string{"cosine", "l2", "inner_product"} {
		if !seen[want] {
			t.Errorf("missing metric %q in ValidMetrics()", want)
		}
	}
}

// --- SQL generation for nearest query ---

func TestBuildNearestQuery(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "documents",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", JSONType: "string", IsPrimaryKey: true},
			{Name: "title", TypeName: "text", JSONType: "string"},
			{Name: "embedding", TypeName: "vector(3)", JSONType: "array", IsVector: true, VectorDim: 3},
		},
	}

	params := NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
	}

	sql, args, err := BuildNearestQuery(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain the distance operator and ORDER BY.
	if sql == "" {
		t.Fatal("expected non-empty SQL")
	}
	if len(args) < 1 {
		t.Fatal("expected at least 1 arg (the query vector)")
	}

	// Verify the SQL contains expected fragments.
	assertContains(t, sql, `<=>`) // cosine operator
	assertContains(t, sql, `ORDER BY`)
	assertContains(t, sql, `_distance`)
	assertContains(t, sql, `LIMIT`)
}

func TestBuildNearestQueryL2(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "items",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", JSONType: "integer", IsPrimaryKey: true},
			{Name: "vec", TypeName: "vector(2)", JSONType: "array", IsVector: true, VectorDim: 2},
		},
	}

	params := NearestParams{
		Table:        tbl,
		VectorColumn: "vec",
		QueryVector:  []float64{1.0, 2.0},
		Metric:       "l2",
		Limit:        5,
	}

	sql, _, err := BuildNearestQuery(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, sql, `<->`) // l2 operator
}

func TestBuildNearestQueryInvalidMetric(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "embedding", TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "manhattan",
		Limit:        10,
	})
	if err == nil {
		t.Fatal("expected error for invalid metric")
	}
}

func TestBuildNearestQueryDimensionMismatch(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "embedding", TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2},
		Metric:       "cosine",
		Limit:        10,
	})
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}
}

func TestBuildNearestQueryUnknownColumn(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "title", TypeName: "text"},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
	})
	if err == nil {
		t.Fatal("expected error for unknown vector column")
	}
}

func TestBuildNearestQueryNonVectorColumn(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "title", TypeName: "text", JSONType: "string"},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "title",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
	})
	if err == nil {
		t.Fatal("expected error for non-vector column")
	}
}

func TestBuildNearestQueryWithFilter(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "documents",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", JSONType: "string", IsPrimaryKey: true},
			{Name: "status", TypeName: "text", JSONType: "string"},
			{Name: "embedding", TypeName: "vector(3)", JSONType: "array", IsVector: true, VectorDim: 3},
		},
	}

	params := NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
		FilterSQL:    `"status" = $1`,
		FilterArgs:   []any{"active"},
	}

	sql, args, err := BuildNearestQuery(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, sql, "WHERE")
	assertContains(t, sql, `"status"`)
	// Filter arg + vector arg
	if len(args) < 2 {
		t.Fatalf("expected at least 2 args, got %d", len(args))
	}
}

// --- Vector column detection ---

func TestIsVectorColumn(t *testing.T) {
	tests := []struct {
		typeName string
		want     bool
	}{
		{"vector(3)", true},
		{"vector(1536)", true},
		{"vector", true},
		{"text", false},
		{"integer", false},
		{"jsonb", false},
		{"geometry", false},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			got := IsVectorType(tt.typeName)
			if got != tt.want {
				t.Errorf("IsVectorType(%q) = %v, want %v", tt.typeName, got, tt.want)
			}
		})
	}
}

func TestParseVectorDimension(t *testing.T) {
	tests := []struct {
		typeName string
		want     int
	}{
		{"vector(3)", 3},
		{"vector(1536)", 1536},
		{"vector", 0},
		{"text", 0},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			got := ParseVectorDim(tt.typeName)
			if got != tt.want {
				t.Errorf("ParseVectorDim(%q) = %d, want %d", tt.typeName, got, tt.want)
			}
		})
	}
}

// --- Vector index SQL ---

func TestBuildCreateIndexSQL(t *testing.T) {
	tests := []struct {
		name    string
		params  IndexParams
		wantSQL string
	}{
		{
			name: "hnsw cosine",
			params: IndexParams{
				Schema:    "public",
				Table:     "documents",
				Column:    "embedding",
				Method:    "hnsw",
				Metric:    "cosine",
				IndexName: "idx_documents_embedding",
			},
			wantSQL: `CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_documents_embedding" ON "public"."documents" USING hnsw ("embedding" vector_cosine_ops)`,
		},
		{
			name: "ivfflat l2",
			params: IndexParams{
				Schema:    "public",
				Table:     "documents",
				Column:    "embedding",
				Method:    "ivfflat",
				Metric:    "l2",
				IndexName: "idx_documents_embedding_ivf",
				Lists:     100,
			},
			wantSQL: `CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_documents_embedding_ivf" ON "public"."documents" USING ivfflat ("embedding" vector_l2_ops) WITH (lists = 100)`,
		},
		{
			name: "hnsw inner_product",
			params: IndexParams{
				Schema:    "public",
				Table:     "items",
				Column:    "vec",
				Method:    "hnsw",
				Metric:    "inner_product",
				IndexName: "idx_items_vec",
			},
			wantSQL: `CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_items_vec" ON "public"."items" USING hnsw ("vec" vector_ip_ops)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, err := BuildCreateIndexSQL(tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("got:\n  %s\nwant:\n  %s", sql, tt.wantSQL)
			}
		})
	}
}

func TestBuildCreateIndexSQLInvalidMethod(t *testing.T) {
	_, err := BuildCreateIndexSQL(IndexParams{
		Schema: "public", Table: "docs", Column: "vec",
		Method: "btree", Metric: "cosine", IndexName: "idx",
	})
	if err == nil {
		t.Fatal("expected error for invalid method")
	}
}

func TestBuildCreateIndexSQLInvalidMetric(t *testing.T) {
	_, err := BuildCreateIndexSQL(IndexParams{
		Schema: "public", Table: "docs", Column: "vec",
		Method: "hnsw", Metric: "hamming", IndexName: "idx",
	})
	if err == nil {
		t.Fatal("expected error for invalid metric")
	}
}

// --- Helpers ---

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !containsStr(haystack, needle) {
		t.Errorf("expected SQL to contain %q, got:\n%s", needle, haystack)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

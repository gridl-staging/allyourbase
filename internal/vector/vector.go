package vector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// Supported distance metrics and their pgvector operators.
var distanceOps = map[string]string{
	"cosine":        "<=>",
	"l2":            "<->",
	"inner_product": "<#>",
}

// DistanceOperator returns the pgvector operator for the given distance metric.
func DistanceOperator(metric string) (string, error) {
	op, ok := distanceOps[metric]
	if !ok {
		return "", fmt.Errorf("unsupported distance metric %q; valid: %s", metric, strings.Join(ValidMetrics(), ", "))
	}
	return op, nil
}

// ValidMetrics returns all supported distance metric names.
func ValidMetrics() []string {
	out := make([]string, 0, len(distanceOps))
	for k := range distanceOps {
		out = append(out, k)
	}
	return out
}

// IsVectorType returns true if the PostgreSQL type name is a pgvector vector type.
func IsVectorType(typeName string) bool {
	lower := strings.ToLower(typeName)
	return lower == "vector" || strings.HasPrefix(lower, "vector(")
}

// ParseVectorDim extracts the dimension from a vector(N) type name.
// Returns 0 if not parseable or the type has no explicit dimension.
func ParseVectorDim(typeName string) int {
	lower := strings.ToLower(typeName)
	if !strings.HasPrefix(lower, "vector(") {
		return 0
	}
	inner := lower[len("vector("):]
	idx := strings.Index(inner, ")")
	if idx < 0 {
		return 0
	}
	n, err := strconv.Atoi(inner[:idx])
	if err != nil {
		return 0
	}
	return n
}

// NearestParams configures a nearest-neighbor vector search query.
type NearestParams struct {
	Table        *schema.Table
	VectorColumn string
	QueryVector  []float64
	Metric       string // cosine, l2, inner_product
	Limit        int
	FilterSQL    string // optional pre-built WHERE clause fragment
	FilterArgs   []any  // args for FilterSQL
}

// BuildNearestQuery generates a parameterized SQL query for vector similarity search.
// Returns the SQL string, positional args, and any error.
func BuildNearestQuery(p NearestParams) (string, []any, error) {
	// Validate metric.
	op, err := DistanceOperator(p.Metric)
	if err != nil {
		return "", nil, err
	}

	// Find and validate the vector column.
	col := p.Table.ColumnByName(p.VectorColumn)
	if col == nil {
		return "", nil, fmt.Errorf("column %q not found in table %q", p.VectorColumn, p.Table.Name)
	}
	if !col.IsVector {
		return "", nil, fmt.Errorf("column %q is not a vector column (type: %s)", p.VectorColumn, col.TypeName)
	}

	// Validate dimensions if the column has an explicit dimension.
	if col.VectorDim > 0 && len(p.QueryVector) != col.VectorDim {
		return "", nil, fmt.Errorf("dimension mismatch: query vector has %d dimensions, column %q expects %d",
			len(p.QueryVector), p.VectorColumn, col.VectorDim)
	}

	// Build column list (all non-vector columns as-is, vector as array).
	colExprs := make([]string, 0, len(p.Table.Columns)+1)
	for _, c := range p.Table.Columns {
		colExprs = append(colExprs, sqlutil.QuoteIdent(c.Name))
	}

	// Build args: filter args first, then vector arg.
	args := make([]any, 0, len(p.FilterArgs)+2)
	args = append(args, p.FilterArgs...)

	vecParamIdx := len(args) + 1
	args = append(args, formatVector(p.QueryVector))

	// Distance expression as a computed column.
	distExpr := fmt.Sprintf("%s %s $%d AS _distance",
		sqlutil.QuoteIdent(p.VectorColumn), op, vecParamIdx)
	colExprs = append(colExprs, distExpr)

	ref := sqlutil.QuoteQualifiedName(p.Table.Schema, p.Table.Name)

	// WHERE clause.
	where := ""
	if p.FilterSQL != "" {
		where = " WHERE " + p.FilterSQL
	}

	limitParamIdx := len(args) + 1
	args = append(args, p.Limit)

	sql := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY %s %s $%d ASC LIMIT $%d",
		strings.Join(colExprs, ", "),
		ref,
		where,
		sqlutil.QuoteIdent(p.VectorColumn), op, vecParamIdx,
		limitParamIdx,
	)

	return sql, args, nil
}

// IndexParams configures vector index creation.
type IndexParams struct {
	Schema    string
	Table     string
	Column    string
	Method    string // hnsw or ivfflat
	Metric    string // cosine, l2, inner_product
	IndexName string
	Lists     int // only for ivfflat
}

// opsClass maps metric names to pgvector operator class names.
var opsClass = map[string]string{
	"cosine":        "vector_cosine_ops",
	"l2":            "vector_l2_ops",
	"inner_product": "vector_ip_ops",
}

// BuildCreateIndexSQL generates DDL for creating a pgvector index.
func BuildCreateIndexSQL(p IndexParams) (string, error) {
	if p.Method != "hnsw" && p.Method != "ivfflat" {
		return "", fmt.Errorf("unsupported index method %q; valid: hnsw, ivfflat", p.Method)
	}
	opClass, ok := opsClass[p.Metric]
	if !ok {
		return "", fmt.Errorf("unsupported metric %q for index; valid: %s", p.Metric, strings.Join(ValidMetrics(), ", "))
	}

	sql := fmt.Sprintf(`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s USING %s (%s %s)`,
		sqlutil.QuoteIdent(p.IndexName),
		sqlutil.QuoteQualifiedName(p.Schema, p.Table),
		p.Method,
		sqlutil.QuoteIdent(p.Column), opClass,
	)

	if p.Method == "ivfflat" && p.Lists > 0 {
		sql += fmt.Sprintf(" WITH (lists = %d)", p.Lists)
	}

	return sql, nil
}

// formatVector formats a float64 slice as a pgvector literal string "[0.1,0.2,0.3]".
func formatVector(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(f, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

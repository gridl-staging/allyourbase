package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// aggregateTestSchema returns a schema with numeric and text columns for aggregate testing.
func aggregateTestSchema() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.products": {
				Schema: "public",
				Name:   "products",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "integer"},
					{Name: "name", TypeName: "text"},
					{Name: "price", TypeName: "numeric"},
					{Name: "quantity", TypeName: "integer"},
					{Name: "category", TypeName: "text"},
					{Name: "weight", TypeName: "double precision"},
					{Name: "active", TypeName: "boolean"},
					{Name: "location", TypeName: "geometry(Point,4326)", IsGeometry: true, SRID: 4326},
					{Name: "ship_point", TypeName: "geography(Point,4326)", IsGeometry: true, IsGeography: true, SRID: 4326},
				},
				PrimaryKey: []string{"id"},
			},
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid"},
					{Name: "email", TypeName: "text"},
					{Name: "name", TypeName: "text", IsNullable: true},
				},
				PrimaryKey: []string{"id"},
			},
		},
		Schemas: []string{"public"},
	}
}

// --- parseAggregate tests ---

func TestParseAggregateBareCount(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs, err := parseAggregate(tbl, "count")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(exprs))
	testutil.Equal(t, "count", exprs[0].Func)
	testutil.Equal(t, "", exprs[0].Column)
}

func TestParseAggregateMultiFunctions(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs, err := parseAggregate(tbl, "count,sum(price),avg(quantity)")
	testutil.NoError(t, err)
	testutil.Equal(t, 3, len(exprs))
	testutil.Equal(t, "count", exprs[0].Func)
	testutil.Equal(t, "", exprs[0].Column)
	testutil.Equal(t, "sum", exprs[1].Func)
	testutil.Equal(t, "price", exprs[1].Column)
	testutil.Equal(t, "avg", exprs[2].Func)
	testutil.Equal(t, "quantity", exprs[2].Column)
}

func TestParseAggregateAllFunctions(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs, err := parseAggregate(tbl, "count,count_distinct(category),sum(price),avg(price),min(price),max(price)")
	testutil.NoError(t, err)
	testutil.Equal(t, 6, len(exprs))
	testutil.Equal(t, "count_distinct", exprs[1].Func)
	testutil.Equal(t, "category", exprs[1].Column)
	testutil.Equal(t, "min", exprs[4].Func)
	testutil.Equal(t, "max", exprs[5].Func)
}

func TestParseAggregateWhitespace(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs, err := parseAggregate(tbl, " count , sum( price ) ")
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(exprs))
	testutil.Equal(t, "count", exprs[0].Func)
	testutil.Equal(t, "sum", exprs[1].Func)
	testutil.Equal(t, "price", exprs[1].Column)
}

func TestParseAggregateRejectsUnknownFunction(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "foo(price)")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unknown aggregate function")
}

func TestParseAggregateRejectsUnknownColumn(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "sum(nonexistent)")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unknown column")
}

func TestParseAggregateRejectsNonNumericForSum(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "sum(name)")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "numeric")
}

func TestParseAggregateRejectsNonNumericForAvg(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "avg(category)")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "numeric")
}

func TestParseAggregateAllowsSumOnDoubles(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs, err := parseAggregate(tbl, "sum(weight)")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(exprs))
	testutil.Equal(t, "sum", exprs[0].Func)
	testutil.Equal(t, "weight", exprs[0].Column)
}

func TestParseAggregateAllowsMinMaxOnText(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	// min/max should work on any comparable type, not just numeric.
	exprs, err := parseAggregate(tbl, "min(name),max(name)")
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(exprs))
}

func TestParseAggregateRejectsEmptyParens(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "count()")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "empty column")
}

func TestParseAggregateRejectsEmptyInput(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "empty")
}

func TestParseAggregateRejectsMalformedParens(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "sum(price")
	testutil.Error(t, err)
}

func TestParseAggregateRejectsOversizedInput(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	longInput := strings.Repeat("count,", maxAggregateLen/6+1)
	_, err := parseAggregate(tbl, longInput)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "too long")
}

func TestParseAggregateCountDistinct(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs, err := parseAggregate(tbl, "count_distinct(category)")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(exprs))
	testutil.Equal(t, "count_distinct", exprs[0].Func)
	testutil.Equal(t, "category", exprs[0].Column)
}

func TestParseAggregateSpatialFunctions(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs, err := parseAggregate(tbl, "bbox(location),centroid(ship_point)")
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(exprs))
	testutil.Equal(t, "bbox", exprs[0].Func)
	testutil.Equal(t, "location", exprs[0].Column)
	testutil.Equal(t, "centroid", exprs[1].Func)
	testutil.Equal(t, "ship_point", exprs[1].Column)
}

func TestParseAggregateSpatialRejectsNonSpatialColumn(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseAggregate(tbl, "bbox(name)")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "requires a spatial column")
}

// --- parseGroupColumns tests ---

func TestParseGroupColumnsValid(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	cols, err := parseGroupColumns(tbl, "category")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(cols))
	testutil.Equal(t, "category", cols[0])
}

func TestParseGroupColumnsMultiple(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	cols, err := parseGroupColumns(tbl, "category,active")
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(cols))
	testutil.Equal(t, "category", cols[0])
	testutil.Equal(t, "active", cols[1])
}

func TestParseGroupColumnsUnknown(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	_, err := parseGroupColumns(tbl, "nonexistent")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unknown column")
}

func TestParseGroupColumnsEmpty(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	cols, err := parseGroupColumns(tbl, "")
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(cols))
}

func TestParseGroupColumnsWhitespace(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	cols, err := parseGroupColumns(tbl, " category , active ")
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(cols))
	testutil.Equal(t, "category", cols[0])
	testutil.Equal(t, "active", cols[1])
}

// --- buildAggregate SQL tests ---

func TestBuildAggregateBareCount(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{{Func: "count"}}
	q, args, err := buildAggregate(tbl, exprs, listOpts{}, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(args))
	testutil.Contains(t, q, `COUNT(*) AS "count"`)
	testutil.Contains(t, q, `FROM "public"."products"`)
}

func TestBuildAggregateWithFilter(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{{Func: "count"}}
	opts := listOpts{
		filterSQL:  `"category" = $1`,
		filterArgs: []any{"electronics"},
	}
	q, args, err := buildAggregate(tbl, exprs, opts, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, "electronics", args[0])
	testutil.Contains(t, q, "WHERE")
	testutil.Contains(t, q, `"category" = $1`)
}

func TestBuildAggregateSumAvgGroupBy(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{
		{Func: "sum", Column: "price"},
		{Func: "avg", Column: "price"},
	}
	q, args, err := buildAggregate(tbl, exprs, listOpts{}, []string{"category"})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(args))
	testutil.Contains(t, q, `SUM("price") AS "sum_price"`)
	testutil.Contains(t, q, `AVG("price") AS "avg_price"`)
	testutil.Contains(t, q, `GROUP BY "category"`)
	// Group columns must appear in SELECT.
	testutil.Contains(t, q, `"category"`)
}

func TestBuildAggregateCountDistinct(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{{Func: "count_distinct", Column: "category"}}
	q, _, err := buildAggregate(tbl, exprs, listOpts{}, nil)
	testutil.NoError(t, err)
	testutil.Contains(t, q, `COUNT(DISTINCT "category") AS "count_distinct_category"`)
}

func TestBuildAggregateMultipleGroupBy(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{{Func: "count"}}
	q, _, err := buildAggregate(tbl, exprs, listOpts{}, []string{"category", "active"})
	testutil.NoError(t, err)
	testutil.Contains(t, q, `GROUP BY "category", "active"`)
}

func TestBuildAggregateSpatialFunctions(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{
		{Func: "bbox", Column: "location"},
		{Func: "centroid", Column: "ship_point"},
	}
	q, _, err := buildAggregate(tbl, exprs, listOpts{}, nil)
	testutil.NoError(t, err)
	testutil.Contains(t, q, `ST_AsGeoJSON(ST_Extent("location"))::jsonb AS "bbox_location"`)
	testutil.Contains(t, q, `ST_AsGeoJSON(ST_Centroid(ST_Collect("ship_point"::geometry)))::jsonb AS "centroid_ship_point"`)
}

func TestBuildAggregateSpatialWithGroupFilterAndSearchOrdering(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{{Func: "bbox", Column: "location"}}
	opts := listOpts{
		filterSQL:  `"category" = $1`,
		filterArgs: []any{"electronics"},
		searchSQL:  `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $2)`,
		searchArgs: []any{"widget"},
	}

	q, args, err := buildAggregate(tbl, exprs, opts, []string{"active"})
	testutil.NoError(t, err)
	testutil.SliceLen(t, args, 2)
	testutil.Contains(t, q, `"category" = $1`)
	testutil.Contains(t, q, `websearch_to_tsquery('simple', $2)`)
	testutil.Contains(t, q, `GROUP BY "active"`)
}

func TestBuildAggregateWithFilterAndSearch(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{{Func: "count"}}
	opts := listOpts{
		filterSQL:  `"category" = $1`,
		filterArgs: []any{"electronics"},
		searchSQL:  `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $2)`,
		searchArgs: []any{"widget"},
	}
	q, args, err := buildAggregate(tbl, exprs, opts, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(args))
	testutil.Contains(t, q, "WHERE")
	testutil.Contains(t, q, "$1")
	testutil.Contains(t, q, "$2")
	_ = q
}

func TestBuildAggregateWithFilterSpatialAndSearch(t *testing.T) {
	t.Parallel()
	sc := aggregateTestSchema()
	tbl := sc.TableByName("products")
	exprs := []AggregateExpr{{Func: "count"}}
	opts := listOpts{
		filterSQL:   `"category" = $1`,
		filterArgs:  []any{"electronics"},
		spatialSQL:  `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`,
		spatialArgs: []any{-1.0, -2.0, 3.0, 4.0},
		searchSQL:   `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $6)`,
		searchArgs:  []any{"widget"},
	}

	q, args, err := buildAggregate(tbl, exprs, opts, nil)
	testutil.NoError(t, err)
	testutil.SliceLen(t, args, 6)
	testutil.Equal(t, "electronics", args[0])
	testutil.Equal(t, -1.0, args[1].(float64))
	testutil.Equal(t, "widget", args[5])
	testutil.Contains(t, q, `"category" = $1`)
	testutil.Contains(t, q, `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`)
	testutil.Contains(t, q, `websearch_to_tsquery('simple', $6)`)
}

// --- Handler-level aggregate validation tests ---

func TestAggregateWithPageReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?aggregate=count&page=2", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "cannot be combined")
}

func TestAggregateWithPerPageReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?aggregate=count&perPage=10", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "cannot be combined")
}

func TestAggregateWithSortReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?aggregate=count&sort=id", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "cannot be combined")
}

func TestAggregateWithFieldsReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?aggregate=count&fields=id", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "cannot be combined")
}

func TestAggregateWithExpandReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?aggregate=count&expand=foo", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "cannot be combined")
}

func TestGroupWithoutAggregateReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?group=category", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "group")
}

func TestAggregateInvalidExpressionReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?aggregate=bogus(price)", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid aggregate")
}

func TestAggregateInvalidGroupColumnReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	w := doRequest(h, "GET", "/collections/products?aggregate=count&group=nonexistent", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid group")
}

func TestAggregateSchemaNotReady(t *testing.T) {
	t.Parallel()
	h := testHandler(nil)
	w := doRequest(h, "GET", "/collections/products?aggregate=count", "")
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestAggregateNoDBReturnsError verifies that aggregate query attempts
// without a database pool return an internal error.
func TestAggregateNoDBReturnsError(t *testing.T) {
	t.Parallel()
	h := testHandler(aggregateTestSchema())
	// testHandler creates handler without pool (pool=nil).
	// The request should either succeed with validation or fail cleanly on DB access.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/collections/products?aggregate=count", nil)
	h.ServeHTTP(w, r)
	// Should get 500 since pool is nil and withRLS will fail.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAggregateDisabledReturns403(t *testing.T) {
	t.Parallel()
	h := testHandlerWithOptions(
		aggregateTestSchema(),
		WithAPILimits(config.APIConfig{AggregateEnabled: false}),
	)
	w := doRequest(h, "GET", "/collections/products?aggregate=count", "")
	testutil.Equal(t, http.StatusForbidden, w.Code)
	resp := decodeError(t, w)
	testutil.Equal(t, "aggregate queries are disabled", resp.Message)
}

func TestAggregateEnabledWhenOnlyImportLimitOverridden(t *testing.T) {
	t.Parallel()
	h := testHandlerWithOptions(
		aggregateTestSchema(),
		WithAPILimits(config.APIConfig{ImportMaxRows: 1}),
	)
	w := doRequest(h, "GET", "/collections/products?aggregate=count", "")
	// Aggregates stay enabled here; with no DB pool the request should fail at query time.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

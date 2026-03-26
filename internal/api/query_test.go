package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func testTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "users",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", Position: 2, TypeName: "text"},
			{Name: "email", Position: 3, TypeName: "varchar"},
			{Name: "age", Position: 4, TypeName: "integer"},
		},
		PrimaryKey: []string{"id"},
	}
}

func compositePKTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "order_items",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "order_id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "item_id", Position: 2, TypeName: "integer", IsPrimaryKey: true},
			{Name: "quantity", Position: 3, TypeName: "integer"},
		},
		PrimaryKey: []string{"order_id", "item_id"},
	}
}

func geometryTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "routes",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", Position: 2, TypeName: "text"},
			{Name: "path", Position: 3, TypeName: "geometry(LineString,4326)", IsGeometry: true},
		},
		PrimaryKey: []string{"id"},
	}
}

func geographyTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "places_geog",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", Position: 2, TypeName: "text"},
			{Name: "location", Position: 3, TypeName: "geography(Point,4326)", IsGeometry: true, IsGeography: true, SRID: 4326},
		},
		PrimaryKey: []string{"id"},
	}
}

func TestBuildSelectOne(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	q, args := buildSelectOne(tbl, nil, []string{"42"})
	testutil.Contains(t, q, `SELECT * FROM "public"."users"`)
	testutil.Contains(t, q, `"id" = $1`)
	testutil.Contains(t, q, "LIMIT 1")
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, "42", args[0].(string))
}

func TestBuildSelectOneWithFields(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	q, args := buildSelectOne(tbl, []string{"id", "name"}, []string{"1"})
	testutil.Contains(t, q, `"id", "name"`)
	testutil.Contains(t, q, `"id" = $1`)
	testutil.SliceLen(t, args, 1)
}

func TestBuildSelectOneFieldValidation(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	// Unknown fields should be ignored, falls back to *.
	q, _ := buildSelectOne(tbl, []string{"nonexistent"}, []string{"1"})
	testutil.Contains(t, q, "SELECT *")
}

func TestBuildInsert(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	// Use a single key to keep output deterministic.
	data := map[string]any{"name": "Alice"}
	q, args := buildInsert(tbl, data)
	testutil.Contains(t, q, "INSERT INTO")
	testutil.Contains(t, q, `"name"`)
	testutil.Contains(t, q, "$1")
	testutil.Contains(t, q, "RETURNING *")
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, "Alice", args[0].(string))
}

func TestBuildInsertSkipsUnknownColumns(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	data := map[string]any{"name": "Alice", "nonexistent": "val"}
	q, args := buildInsert(tbl, data)
	// Should only have the valid column.
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, "Alice", args[0])
	testutil.Contains(t, q, `"name"`)
	testutil.True(t, !strings.Contains(q, "nonexistent"), "query should not contain unknown column")
}

func TestBuildUpdate(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	data := map[string]any{"name": "Bob"}
	q, args := buildUpdate(tbl, data, []string{"1"})
	testutil.Contains(t, q, "UPDATE")
	testutil.Contains(t, q, "SET")
	testutil.Contains(t, q, `"name" = $1`)
	testutil.Contains(t, q, `"id" = $2`)
	testutil.Contains(t, q, "RETURNING *")
	testutil.SliceLen(t, args, 2)
}

func TestBuildDelete(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	q, args := buildDelete(tbl, []string{"5"})
	testutil.Contains(t, q, "DELETE FROM")
	testutil.Contains(t, q, `"id" = $1`)
	testutil.SliceLen(t, args, 1)
}

func TestBuildPKWhereComposite(t *testing.T) {
	t.Parallel()
	tbl := compositePKTable()

	where, args := buildPKWhere(tbl, []string{"10", "20"})
	testutil.Contains(t, where, `"order_id" = $1`)
	testutil.Contains(t, where, `"item_id" = $2`)
	testutil.SliceLen(t, args, 2)
}

func TestBuildColumnListEmpty(t *testing.T) {
	t.Parallel()
	tbl := testTable()
	testutil.Equal(t, "*", buildColumnList(tbl, nil))
	testutil.Equal(t, "*", buildColumnList(tbl, []string{}))
}

func TestBuildColumnListWithFields(t *testing.T) {
	t.Parallel()
	tbl := testTable()
	result := buildColumnList(tbl, []string{"id", "name"})
	testutil.Contains(t, result, `"id"`)
	testutil.Contains(t, result, `"name"`)
}

func TestBuildColumnListGeometryAllColumns(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	result := buildColumnList(tbl, nil)
	testutil.Contains(t, result, `"id"`)
	testutil.Contains(t, result, `"name"`)
	testutil.Contains(t, result, `ST_AsGeoJSON("path")::jsonb AS "path"`)
	testutil.True(t, !strings.Contains(result, "*"), "geometry table should not use wildcard projection")
}

func TestBuildColumnListGeometryFields(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	result := buildColumnList(tbl, []string{"id", "path"})
	testutil.Contains(t, result, `"id"`)
	testutil.Contains(t, result, `ST_AsGeoJSON("path")::jsonb AS "path"`)
}

func TestBuildColumnListGeometryUnknownFieldsFallback(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	result := buildColumnList(tbl, []string{"does_not_exist"})
	testutil.Contains(t, result, `ST_AsGeoJSON("path")::jsonb AS "path"`)
	testutil.True(t, !strings.Contains(result, "*"), "geometry table fallback should still avoid wildcard projection")
}

func TestBuildList(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	opts := listOpts{
		page:    1,
		perPage: 20,
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)
	testutil.Contains(t, dataQ, "SELECT *")
	testutil.Contains(t, dataQ, "LIMIT $1")
	testutil.Contains(t, dataQ, "OFFSET $2")
	testutil.SliceLen(t, dataArgs, 2)
	testutil.Equal(t, 20, dataArgs[0].(int)) // perPage
	testutil.Equal(t, 0, dataArgs[1].(int))  // offset

	testutil.Contains(t, countQ, "SELECT COUNT(*)")
	testutil.SliceLen(t, countArgs, 0)
}

func TestBuildReturningClause(t *testing.T) {
	t.Parallel()

	testutil.Equal(t, "RETURNING *", buildReturningClause(testTable()))

	geoReturning := buildReturningClause(geometryTable())
	testutil.Contains(t, geoReturning, `RETURNING "id", "name", ST_AsGeoJSON("path")::jsonb AS "path"`)
}

func TestBuildInsertGeometryValue(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	data := map[string]any{
		"path": map[string]any{
			"type":        "LineString",
			"coordinates": []any{[]any{-73.9, 40.7}, []any{-73.8, 40.8}},
		},
	}

	q, args := buildInsert(tbl, data)
	testutil.Contains(t, q, `ST_GeomFromGeoJSON($1)`)
	testutil.Contains(t, q, `ST_AsGeoJSON("path")::jsonb AS "path"`)
	testutil.SliceLen(t, args, 1)

	raw, ok := args[0].(string)
	testutil.True(t, ok, "geometry argument should be serialized JSON string")
	testutil.True(t, json.Valid([]byte(raw)), "geometry argument should be valid JSON")

	var decoded map[string]any
	err := json.Unmarshal([]byte(raw), &decoded)
	testutil.NoError(t, err)
	testutil.Equal(t, "LineString", decoded["type"])
}

func TestBuildInsertGeometryNull(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	q, args := buildInsert(tbl, map[string]any{"path": nil})
	testutil.Contains(t, q, `VALUES (NULL)`)
	testutil.SliceLen(t, args, 0)
}

func TestBuildUpdateGeometryValue(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	q, args := buildUpdate(tbl, map[string]any{"path": map[string]any{"type": "Point", "coordinates": []any{-73.9, 40.7}}}, []string{"1"})
	testutil.Contains(t, q, `"path" = ST_GeomFromGeoJSON($1)`)
	testutil.Contains(t, q, `"id" = $2`)
	testutil.Contains(t, q, `ST_AsGeoJSON("path")::jsonb AS "path"`)
	testutil.SliceLen(t, args, 2)
}

func TestBuildUpdateGeometryNull(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	q, args := buildUpdate(tbl, map[string]any{"path": nil}, []string{"1"})
	testutil.Contains(t, q, `"path" = NULL`)
	testutil.Contains(t, q, `"id" = $1`)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, "1", args[0])
}

func TestBuildInsertGeographyValueUsesGeographyCast(t *testing.T) {
	t.Parallel()
	tbl := geographyTable()

	q, args := buildInsert(tbl, map[string]any{"location": map[string]any{"type": "Point", "coordinates": []any{-73.9, 40.7}}})
	testutil.Contains(t, q, `ST_GeomFromGeoJSON($1)::geography`)
	testutil.Contains(t, q, `ST_AsGeoJSON("location")::jsonb AS "location"`)
	testutil.SliceLen(t, args, 1)
}

func TestBuildUpdateGeographyValueUsesGeographyCast(t *testing.T) {
	t.Parallel()
	tbl := geographyTable()

	q, args := buildUpdate(tbl, map[string]any{"location": map[string]any{"type": "Point", "coordinates": []any{-73.9, 40.7}}}, []string{"1"})
	testutil.Contains(t, q, `"location" = ST_GeomFromGeoJSON($1)::geography`)
	testutil.Contains(t, q, `ST_AsGeoJSON("location")::jsonb AS "location"`)
	testutil.SliceLen(t, args, 2)
}

func TestBuildUpdateWithAuditGeographyValueUsesGeographyCast(t *testing.T) {
	t.Parallel()
	tbl := geographyTable()

	q, args := buildUpdateWithAudit(tbl, map[string]any{"location": map[string]any{"type": "Point", "coordinates": []any{-73.9, 40.7}}}, []string{"1"})
	testutil.Contains(t, q, `"location" = ST_GeomFromGeoJSON($1)::geography`)
	testutil.Contains(t, q, `_audit_old_values`)
	testutil.SliceLen(t, args, 2)
}

func TestBatchCreateGeometrySQL(t *testing.T) {
	// Verifies the batch code path: execBatchOp calls buildInsert with op.Body,
	// which should produce geometry-aware SQL with ST_GeomFromGeoJSON for input
	// and ST_AsGeoJSON in RETURNING clause.
	t.Parallel()
	tbl := geometryTable()

	body := map[string]any{
		"name": "Route A",
		"path": map[string]any{
			"type":        "LineString",
			"coordinates": []any{[]any{-73.9, 40.7}, []any{-73.8, 40.8}},
		},
	}

	q, args := buildInsert(tbl, body)
	testutil.Contains(t, q, "ST_GeomFromGeoJSON(")
	testutil.Contains(t, q, `ST_AsGeoJSON("path")::jsonb AS "path"`)
	testutil.True(t, len(args) >= 1, "should have at least 1 arg")
}

func TestBatchUpdateGeometrySQL(t *testing.T) {
	// Verifies the batch code path: execBatchOp calls buildUpdate with op.Body,
	// which should produce geometry-aware SQL.
	t.Parallel()
	tbl := geometryTable()

	body := map[string]any{
		"path": map[string]any{
			"type":        "Point",
			"coordinates": []any{-73.9, 40.7},
		},
	}

	q, args := buildUpdate(tbl, body, []string{"1"})
	testutil.Contains(t, q, `"path" = ST_GeomFromGeoJSON(`)
	testutil.Contains(t, q, `ST_AsGeoJSON("path")::jsonb AS "path"`)
	testutil.True(t, len(args) >= 2, "should have at least 2 args (geojson + pk)")
}

func TestBuildListWithFilter(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	opts := listOpts{
		page:       2,
		perPage:    10,
		filterSQL:  `"name" = $1`,
		filterArgs: []any{"Alice"},
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)
	testutil.Contains(t, dataQ, "WHERE")
	testutil.Contains(t, dataQ, `"name" = $1`)
	testutil.Contains(t, dataQ, "LIMIT $2")
	testutil.Contains(t, dataQ, "OFFSET $3")
	testutil.SliceLen(t, dataArgs, 3) // filter arg + limit + offset
	testutil.Equal(t, "Alice", dataArgs[0].(string))
	testutil.Equal(t, 10, dataArgs[1].(int)) // perPage
	testutil.Equal(t, 10, dataArgs[2].(int)) // offset (page 2)

	testutil.Contains(t, countQ, "WHERE")
	testutil.SliceLen(t, countArgs, 1)
}

func TestCombineSQLConditionsPreservesClauseAndArgOrder(t *testing.T) {
	t.Parallel()

	sql, args := combineSQLConditions(
		sqlCondition{clause: `"status" = $1`, args: []any{"published"}},
		sqlCondition{},
		sqlCondition{clause: `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`, args: []any{-1.0, -2.0, 3.0, 4.0}},
	)

	testutil.Equal(t, `"status" = $1 AND ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`, sql)
	testutil.SliceLen(t, args, 5)
	testutil.Equal(t, "published", args[0].(string))
	testutil.Equal(t, -1.0, args[1].(float64))
	testutil.Equal(t, 4.0, args[4].(float64))
}

func TestBuildListWithFilterSpatialAndSearch(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	opts := listOpts{
		page:        1,
		perPage:     10,
		filterSQL:   `"status" = $1`,
		filterArgs:  []any{"published"},
		spatialSQL:  `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`,
		spatialArgs: []any{-1.0, -2.0, 3.0, 4.0},
		searchSQL:   `to_tsvector('simple', coalesce("name", '')) @@ websearch_to_tsquery('simple', $6)`,
		searchRank:  `ts_rank(to_tsvector('simple', coalesce("name", '')), websearch_to_tsquery('simple', $6))`,
		searchArgs:  []any{"hello"},
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)

	testutil.Contains(t, dataQ, `"status" = $1`)
	testutil.Contains(t, dataQ, `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`)
	testutil.Contains(t, dataQ, `websearch_to_tsquery('simple', $6)`)
	testutil.Contains(t, dataQ, "LIMIT $7")
	testutil.Contains(t, dataQ, "OFFSET $8")
	testutil.SliceLen(t, dataArgs, 8)
	testutil.Equal(t, "published", dataArgs[0].(string))
	testutil.Equal(t, -1.0, dataArgs[1].(float64))
	testutil.Equal(t, "hello", dataArgs[5].(string))
	testutil.Equal(t, 10, dataArgs[6].(int))
	testutil.Equal(t, 0, dataArgs[7].(int))

	testutil.Contains(t, countQ, `"status" = $1`)
	testutil.Contains(t, countQ, `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`)
	testutil.Contains(t, countQ, `websearch_to_tsquery('simple', $6)`)
	testutil.SliceLen(t, countArgs, 6)
}

func TestBuildListSkipTotal(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	opts := listOpts{
		page:      1,
		perPage:   20,
		skipTotal: true,
	}

	_, _, countQ, countArgs := buildList(tbl, opts)
	testutil.Equal(t, "", countQ)
	testutil.Nil(t, countArgs)
}

func TestBuildListWithSort(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	opts := listOpts{
		page:    1,
		perPage: 20,
		sortSQL: `"name" ASC, "age" DESC`,
	}

	dataQ, _, _, _ := buildList(tbl, opts)
	testutil.Contains(t, dataQ, `ORDER BY "name" ASC, "age" DESC`)
}

func TestBuildListWithDistanceSortSelectsDistanceAndPreservesCountArgs(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	parsedSort, err := parseStructuredSort(tbl, "path.distance(-73.9,40.7),-name", true)
	testutil.NoError(t, err)

	opts := listOpts{
		page:        1,
		perPage:     10,
		filterSQL:   `"name" = $1`,
		filterArgs:  []any{"Route A"},
		spatialSQL:  `ST_Intersects("path", ST_MakeEnvelope($2, $3, $4, $5, 4326))`,
		spatialArgs: []any{-1.0, -2.0, 3.0, 4.0},
		searchSQL:   `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $6)`,
		searchArgs:  []any{"route"},
		sort:        ensureStructuredSortPKTiebreaker(tbl, parsedSort),
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)
	testutil.Contains(t, dataQ, `AS "_distance"`)
	testutil.Contains(t, dataQ, `ORDER BY ST_Distance("path", ST_SetSRID(ST_MakePoint($7, $8), 4326)) ASC, "name" DESC, "id" ASC`)
	testutil.Contains(t, dataQ, `LIMIT $9 OFFSET $10`)
	testutil.SliceLen(t, dataArgs, 10)
	testutil.Equal(t, -73.9, dataArgs[6])
	testutil.Equal(t, 40.7, dataArgs[7])
	testutil.Equal(t, 10, dataArgs[8])
	testutil.Equal(t, 0, dataArgs[9])

	testutil.True(t, !strings.Contains(countQ, "ST_Distance"), "count query must not include distance expression")
	testutil.SliceLen(t, countArgs, 6)
}

func TestBuildDeleteReturning(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	q, args := buildDeleteReturning(tbl, []string{"5"})
	testutil.Contains(t, q, "DELETE FROM")
	testutil.Contains(t, q, `"id" = $1`)
	testutil.Contains(t, q, "RETURNING *")
	testutil.SliceLen(t, args, 1)
}

func TestBuildDeleteReturningCompositeKey(t *testing.T) {
	t.Parallel()
	tbl := compositePKTable()

	q, args := buildDeleteReturning(tbl, []string{"10", "20"})
	testutil.Contains(t, q, "DELETE FROM")
	testutil.Contains(t, q, `"order_id" = $1`)
	testutil.Contains(t, q, `"item_id" = $2`)
	testutil.Contains(t, q, "RETURNING *")
	testutil.SliceLen(t, args, 2)
}

func TestBuildUpdateWithAudit(t *testing.T) {
	t.Parallel()
	tbl := testTable()

	data := map[string]any{"name": "Bob"}
	q, args := buildUpdateWithAudit(tbl, data, []string{"1"})
	// CTE captures old row.
	testutil.Contains(t, q, "WITH _old AS (SELECT * FROM")
	// UPDATE SET clause.
	testutil.Contains(t, q, `"name" = $1`)
	// PK WHERE used in both CTE and UPDATE.
	testutil.Contains(t, q, `"id" = $2`)
	// Audit old values subquery in RETURNING.
	testutil.Contains(t, q, "row_to_json(_old.*)")
	testutil.Contains(t, q, "_audit_old_values")
	testutil.SliceLen(t, args, 2)
}

func TestBuildUpdateWithAuditCompositeKey(t *testing.T) {
	t.Parallel()
	tbl := compositePKTable()

	data := map[string]any{"quantity": 5}
	q, args := buildUpdateWithAudit(tbl, data, []string{"10", "20"})
	testutil.Contains(t, q, "WITH _old AS (SELECT * FROM")
	testutil.Contains(t, q, `"quantity" = $1`)
	testutil.Contains(t, q, `"order_id" = $2`)
	testutil.Contains(t, q, `"item_id" = $3`)
	testutil.Contains(t, q, "_audit_old_values")
	testutil.SliceLen(t, args, 3)
}

func TestBuildUpdateWithAuditGeometry(t *testing.T) {
	t.Parallel()
	tbl := geometryTable()

	data := map[string]any{
		"name": "Route A",
		"path": map[string]any{"type": "LineString", "coordinates": [][]float64{{0, 0}, {1, 1}}},
	}
	q, args := buildUpdateWithAudit(tbl, data, []string{"1"})
	testutil.Contains(t, q, "ST_GeomFromGeoJSON")
	testutil.Contains(t, q, "_audit_old_values")
	testutil.Contains(t, q, "WITH _old AS")
	// name=$1, path=ST_GeomFromGeoJSON($2), pk=$3
	testutil.SliceLen(t, args, 3)
}

func TestParsePKValues(t *testing.T) {
	// Single PK.
	t.Parallel()

	vals := parsePKValues("42", 1)
	testutil.SliceLen(t, vals, 1)
	testutil.Equal(t, "42", vals[0])

	// Composite PK.
	vals = parsePKValues("10,20", 2)
	testutil.SliceLen(t, vals, 2)
	testutil.Equal(t, "10", vals[0])
	testutil.Equal(t, "20", vals[1])
}

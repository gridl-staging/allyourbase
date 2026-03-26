package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func cursorSpatialTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
			{Name: "location", TypeName: "geometry(Point,4326)", IsGeometry: true, SRID: 4326},
		},
		PrimaryKey: []string{"id"},
	}
}

// --- parseSortFields tests ---

func TestParseSortFields_SingleASC(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := parseSortFields(tbl, "name")
	testutil.SliceLen(t, fields, 1)
	testutil.Equal(t, "name", fields[0].Column)
	testutil.Equal(t, false, fields[0].Desc)
}

func TestParseSortFields_SingleDESC(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := parseSortFields(tbl, "-name")
	testutil.SliceLen(t, fields, 1)
	testutil.Equal(t, "name", fields[0].Column)
	testutil.Equal(t, true, fields[0].Desc)
}

func TestParseSortFields_MultiMixed(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := parseSortFields(tbl, "-email,+name")
	testutil.SliceLen(t, fields, 2)
	testutil.Equal(t, "email", fields[0].Column)
	testutil.Equal(t, true, fields[0].Desc)
	testutil.Equal(t, "name", fields[1].Column)
	testutil.Equal(t, false, fields[1].Desc)
}

func TestParseSortFields_UnknownColumnsSkipped(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := parseSortFields(tbl, "-bogus,name")
	testutil.SliceLen(t, fields, 1)
	testutil.Equal(t, "name", fields[0].Column)
}

func TestParseSortFields_Empty(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := parseSortFields(tbl, "")
	testutil.SliceLen(t, fields, 0)
}

func TestParseSortFields_ExplicitPlus(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := parseSortFields(tbl, "+email")
	testutil.SliceLen(t, fields, 1)
	testutil.Equal(t, "email", fields[0].Column)
	testutil.Equal(t, false, fields[0].Desc)
}

// --- ensurePKTiebreaker tests ---

func TestEnsurePKTiebreaker_AddsPK(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := []SortField{{Column: "name", Desc: false}}
	result := ensurePKTiebreaker(tbl, fields)
	testutil.SliceLen(t, result, 2)
	testutil.Equal(t, "name", result[0].Column)
	testutil.Equal(t, "id", result[1].Column)
	testutil.Equal(t, false, result[1].Desc) // PK defaults to ASC
}

func TestEnsurePKTiebreaker_NoDuplicateWhenPKPresent(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	fields := []SortField{{Column: "id", Desc: true}}
	result := ensurePKTiebreaker(tbl, fields)
	testutil.SliceLen(t, result, 1)
	testutil.Equal(t, "id", result[0].Column)
	testutil.Equal(t, true, result[0].Desc) // preserves original direction
}

func TestEnsurePKTiebreaker_CompositePK(t *testing.T) {
	tbl := &schema.Table{
		Name:   "orders",
		Schema: "public",
		Columns: []*schema.Column{
			{Name: "user_id", TypeName: "uuid"},
			{Name: "order_id", TypeName: "uuid"},
			{Name: "amount", TypeName: "numeric"},
		},
		PrimaryKey: []string{"user_id", "order_id"},
	}
	fields := []SortField{{Column: "amount", Desc: true}}
	result := ensurePKTiebreaker(tbl, fields)
	testutil.SliceLen(t, result, 3)
	testutil.Equal(t, "amount", result[0].Column)
	testutil.Equal(t, "user_id", result[1].Column)
	testutil.Equal(t, "order_id", result[2].Column)
}

func TestEnsurePKTiebreaker_CompositePKPartialPresent(t *testing.T) {
	tbl := &schema.Table{
		Name:   "orders",
		Schema: "public",
		Columns: []*schema.Column{
			{Name: "user_id", TypeName: "uuid"},
			{Name: "order_id", TypeName: "uuid"},
			{Name: "amount", TypeName: "numeric"},
		},
		PrimaryKey: []string{"user_id", "order_id"},
	}
	// user_id already in sort — only order_id should be appended.
	fields := []SortField{{Column: "user_id", Desc: false}}
	result := ensurePKTiebreaker(tbl, fields)
	testutil.SliceLen(t, result, 2)
	testutil.Equal(t, "user_id", result[0].Column)
	testutil.Equal(t, "order_id", result[1].Column)
}

func TestEnsurePKTiebreaker_NoPK(t *testing.T) {
	tbl := testSchema().Tables["public.nopk"]
	fields := []SortField{{Column: "data", Desc: false}}
	result := ensurePKTiebreaker(tbl, fields)
	// No PK to add — returns input unchanged.
	testutil.SliceLen(t, result, 1)
}

// --- sortFieldsToSQL tests ---

func TestSortFieldsToSQL_Single(t *testing.T) {
	fields := []SortField{{Column: "name", Desc: false}}
	testutil.Equal(t, `"name" ASC`, sortFieldsToSQL(fields))
}

func TestSortFieldsToSQL_MultiMixed(t *testing.T) {
	fields := []SortField{
		{Column: "email", Desc: true},
		{Column: "name", Desc: false},
	}
	testutil.Equal(t, `"email" DESC, "name" ASC`, sortFieldsToSQL(fields))
}

func TestSortFieldsToSQL_MatchesParseSortSQL(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	// parseSortSQL and sortFieldsToSQL(parseSortFields(...)) should produce the same output.
	inputs := []string{"-email,+name", "id", "-name"}
	for _, input := range inputs {
		expected := parseSortSQL(tbl, input)
		fields := parseSortFields(tbl, input)
		got := sortFieldsToSQL(fields)
		testutil.Equal(t, expected, got)
	}
}

func TestSortFieldsToSQL_Empty(t *testing.T) {
	testutil.Equal(t, "", sortFieldsToSQL(nil))
}

// --- encodeCursor / decodeCursor tests ---

func TestCursorRoundTrip_Strings(t *testing.T) {
	values := []any{"hello", "world"}
	encoded := encodeCursor(values)
	decoded, err := decodeCursor(encoded)
	testutil.NoError(t, err)
	testutil.SliceLen(t, decoded, 2)
	testutil.Equal(t, "hello", decoded[0])
	testutil.Equal(t, "world", decoded[1])
}

func TestCursorRoundTrip_Numeric(t *testing.T) {
	values := []any{float64(42), float64(3.14)}
	encoded := encodeCursor(values)
	decoded, err := decodeCursor(encoded)
	testutil.NoError(t, err)
	testutil.SliceLen(t, decoded, 2)
	// JSON round-trips numbers as float64.
	if decoded[0] != float64(42) {
		t.Fatalf("expected 42, got %v", decoded[0])
	}
	if decoded[1] != float64(3.14) {
		t.Fatalf("expected 3.14, got %v", decoded[1])
	}
}

func TestCursorRoundTrip_NilValue(t *testing.T) {
	values := []any{"abc", nil, float64(1)}
	encoded := encodeCursor(values)
	decoded, err := decodeCursor(encoded)
	testutil.NoError(t, err)
	testutil.SliceLen(t, decoded, 3)
	testutil.Equal(t, "abc", decoded[0])
	if decoded[1] != nil {
		t.Fatalf("expected nil, got %v", decoded[1])
	}
	if decoded[2] != float64(1) {
		t.Fatalf("expected 1.0, got %v", decoded[2])
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, err := decodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecodeCursor_InvalidJSON(t *testing.T) {
	// Valid base64 but not valid JSON.
	_, err := decodeCursor("bm90anNvbg") // "notjson"
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecodeCursor_EmptyValues(t *testing.T) {
	// Encode an empty values array — should be rejected.
	encoded := encodeCursor([]any{})
	_, err := decodeCursor(encoded)
	if err == nil {
		t.Fatal("expected error for empty cursor values")
	}
}

func TestDecodeCursor_TooLong(t *testing.T) {
	// A cursor string longer than the max should be rejected.
	long := make([]byte, maxCursorLen+1)
	for i := range long {
		long[i] = 'A'
	}
	_, err := decodeCursor(string(long))
	if err == nil {
		t.Fatal("expected error for oversized cursor")
	}
}

// --- buildCursorWhere tests ---

func TestBuildCursorWhere_AllASC(t *testing.T) {
	fields := []SortField{
		{Column: "name", Desc: false},
		{Column: "id", Desc: false},
	}
	values := []any{"alice", "uuid-1"}
	sql, args, err := buildCursorWhere(fields, values, 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `("name", "id") > ($1, $2)`, sql)
	testutil.SliceLen(t, args, 2)
	testutil.Equal(t, "alice", args[0])
	testutil.Equal(t, "uuid-1", args[1])
}

func TestBuildCursorWhere_AllDESC(t *testing.T) {
	fields := []SortField{
		{Column: "email", Desc: true},
		{Column: "id", Desc: true},
	}
	values := []any{"z@example.com", "uuid-9"}
	sql, args, err := buildCursorWhere(fields, values, 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `("email", "id") < ($1, $2)`, sql)
	testutil.SliceLen(t, args, 2)
}

func TestBuildCursorWhere_MixedDirections(t *testing.T) {
	fields := []SortField{
		{Column: "created", Desc: true},
		{Column: "id", Desc: false},
	}
	values := []any{"2024-01-01", "uuid-5"}
	sql, args, err := buildCursorWhere(fields, values, 1)
	testutil.NoError(t, err)
	// Mixed directions require expanded OR/AND form.
	expected := `("created" < $1) OR ("created" = $1 AND "id" > $2)`
	testutil.Equal(t, expected, sql)
	testutil.SliceLen(t, args, 2)
}

func TestBuildCursorWhere_MixedThreeFields(t *testing.T) {
	fields := []SortField{
		{Column: "status", Desc: false},
		{Column: "created", Desc: true},
		{Column: "id", Desc: false},
	}
	values := []any{"active", "2024-06-01", "uuid-3"}
	sql, args, err := buildCursorWhere(fields, values, 5)
	testutil.NoError(t, err)
	expected := `("status" > $5) OR ("status" = $5 AND "created" < $6) OR ("status" = $5 AND "created" = $6 AND "id" > $7)`
	testutil.Equal(t, expected, sql)
	testutil.SliceLen(t, args, 3)
}

func TestBuildCursorWhere_SingleField(t *testing.T) {
	fields := []SortField{{Column: "id", Desc: false}}
	values := []any{"uuid-1"}
	sql, args, err := buildCursorWhere(fields, values, 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `"id" > $1`, sql)
	testutil.SliceLen(t, args, 1)
}

func TestBuildCursorWhere_SingleFieldDESC(t *testing.T) {
	fields := []SortField{{Column: "id", Desc: true}}
	values := []any{"uuid-1"}
	sql, args, err := buildCursorWhere(fields, values, 3)
	testutil.NoError(t, err)
	testutil.Equal(t, `"id" < $3`, sql)
	testutil.SliceLen(t, args, 1)
}

func TestBuildCursorWhere_ValueCountMismatch(t *testing.T) {
	fields := []SortField{
		{Column: "name", Desc: false},
		{Column: "id", Desc: false},
	}
	values := []any{"alice"} // only 1 value for 2 fields
	_, _, err := buildCursorWhere(fields, values, 1)
	if err == nil {
		t.Fatal("expected error for value count mismatch")
	}
}

func TestBuildCursorWhere_ArgOffset(t *testing.T) {
	fields := []SortField{{Column: "id", Desc: false}}
	values := []any{"uuid-1"}
	sql, _, err := buildCursorWhere(fields, values, 10)
	testutil.NoError(t, err)
	testutil.Equal(t, `"id" > $10`, sql)
}

// --- extractCursorValues tests ---

func TestExtractCursorValues(t *testing.T) {
	fields := []SortField{
		{Column: "email", Desc: true},
		{Column: "id", Desc: false},
	}
	record := map[string]any{
		"id":    "uuid-1",
		"email": "alice@example.com",
		"name":  "Alice",
	}
	values := extractCursorValues(fields, record)
	testutil.SliceLen(t, values, 2)
	testutil.Equal(t, "alice@example.com", values[0])
	testutil.Equal(t, "uuid-1", values[1])
}

func TestExtractCursorValues_NilField(t *testing.T) {
	fields := []SortField{{Column: "name", Desc: false}, {Column: "id", Desc: false}}
	record := map[string]any{"id": "uuid-1"} // name is missing → nil
	values := extractCursorValues(fields, record)
	testutil.SliceLen(t, values, 2)
	if values[0] != nil {
		t.Fatalf("expected nil for missing field, got %v", values[0])
	}
	testutil.Equal(t, "uuid-1", values[1])
}

func TestPrepareCursorSortProjectionUsesHiddenAliasesForOmittedSortKeys(t *testing.T) {
	fields := []SortField{
		{Column: distanceSortOutputColumn, Expr: `ST_Distance("location", ST_SetSRID(ST_MakePoint($1, $2), 4326))`},
		{Column: "id", Desc: false},
	}

	projection := prepareCursorSortProjection(cursorSpatialTable(), []string{"name"}, fields)
	testutil.SliceLen(t, projection.Selects, 1)
	testutil.Contains(t, projection.Selects[0], `"id" AS "__cursor_sort_1"`)
	testutil.SliceLen(t, projection.HelperColumns, 1)
	testutil.Equal(t, "__cursor_sort_1", projection.HelperColumns[0])
	testutil.Equal(t, distanceSortOutputColumn, sortFieldResultColumn(projection.Fields[0]))
	testutil.Equal(t, "__cursor_sort_1", sortFieldResultColumn(projection.Fields[1]))

	values := extractCursorValues(projection.Fields, map[string]any{
		distanceSortOutputColumn: 123.4,
		"__cursor_sort_1":        float64(5),
	})
	testutil.SliceLen(t, values, 2)
	testutil.Equal(t, 123.4, values[0])
	testutil.Equal(t, float64(5), values[1].(float64))
}

func TestPrepareCursorSortProjectionAvoidsRealColumnAliasCollisions(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
			{Name: "__cursor_sort_1", TypeName: "text"},
			{Name: "location", TypeName: "geometry(Point,4326)", IsGeometry: true, SRID: 4326},
		},
		PrimaryKey: []string{"id"},
	}
	fields := []SortField{
		{Column: distanceSortOutputColumn, Expr: `ST_Distance("location", ST_SetSRID(ST_MakePoint($1, $2), 4326))`},
		{Column: "id", Desc: false},
	}

	projection := prepareCursorSortProjection(tbl, []string{"name", "__cursor_sort_1"}, fields)
	testutil.SliceLen(t, projection.Selects, 1)
	testutil.Contains(t, projection.Selects[0], `"id" AS "__cursor_sort_1_1"`)
	testutil.SliceLen(t, projection.HelperColumns, 1)
	testutil.Equal(t, "__cursor_sort_1_1", projection.HelperColumns[0])
	testutil.Equal(t, "__cursor_sort_1_1", sortFieldResultColumn(projection.Fields[1]))

	values := extractCursorValues(projection.Fields, map[string]any{
		distanceSortOutputColumn: 123.4,
		"__cursor_sort_1":        "real-column-value",
		"__cursor_sort_1_1":      float64(5),
	})
	testutil.SliceLen(t, values, 2)
	testutil.Equal(t, 123.4, values[0])
	testutil.Equal(t, float64(5), values[1].(float64))

	items := []map[string]any{
		{"name": "alice", "__cursor_sort_1": "real-column-value", "__cursor_sort_1_1": "hidden"},
	}
	stripCursorHelperFields(items, projection.HelperColumns)
	testutil.Equal(t, "real-column-value", items[0]["__cursor_sort_1"])
	_, found := items[0]["__cursor_sort_1_1"]
	testutil.False(t, found, "generated cursor helper alias should be removed from response items")
}

func TestStripCursorHelperFieldsRemovesOnlyGeneratedAliases(t *testing.T) {
	items := []map[string]any{
		{"name": "alice", "__cursor_sort_0": "hidden", "__cursor_sort_1": "real-column-value"},
	}

	stripCursorHelperFields(items, []string{"__cursor_sort_0"})

	_, found := items[0]["__cursor_sort_0"]
	testutil.False(t, found, "cursor helper alias should be removed from response items")
	testutil.Equal(t, "real-column-value", items[0]["__cursor_sort_1"])
	testutil.Equal(t, "alice", items[0]["name"])
}

// --- buildListWithCursor tests ---

func TestBuildListWithCursor_NoCursor(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{
		perPage: 10,
	}
	sortFields := []SortField{{Column: "id", Desc: false}}
	q, args := buildListWithCursor(tbl, opts, sortFields, "", nil)
	// Should produce SELECT ... ORDER BY ... LIMIT 11 (perPage+1)
	if !contains(q, "LIMIT $1") {
		t.Fatalf("expected LIMIT clause, got: %s", q)
	}
	if !contains(q, `ORDER BY "id" ASC`) {
		t.Fatalf("expected ORDER BY, got: %s", q)
	}
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, 11, args[0]) // perPage+1
}

func TestBuildListWithCursor_WithCursor(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{
		perPage:    5,
		filterSQL:  `"email" = $1`,
		filterArgs: []any{"test@example.com"},
	}
	sortFields := []SortField{{Column: "id", Desc: false}}
	cursorWhere := `"id" > $2`
	cursorArgs := []any{"uuid-5"}
	q, args := buildListWithCursor(tbl, opts, sortFields, cursorWhere, cursorArgs)
	// WHERE should contain filter AND cursor
	if !contains(q, `"email" = $1`) {
		t.Fatalf("expected filter in WHERE, got: %s", q)
	}
	if !contains(q, `"id" > $2`) {
		t.Fatalf("expected cursor WHERE, got: %s", q)
	}
	// args: filter(1) + cursor(1) + limit(1) = 3
	testutil.SliceLen(t, args, 3)
	testutil.Equal(t, "test@example.com", args[0])
	testutil.Equal(t, "uuid-5", args[1])
	testutil.Equal(t, 6, args[2]) // perPage+1
}

func TestBuildListWithCursor_FilterSpatialSearchAndCursor(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{
		perPage:     5,
		filterSQL:   `"email" = $1`,
		filterArgs:  []any{"test@example.com"},
		spatialSQL:  `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`,
		spatialArgs: []any{-1.0, -2.0, 3.0, 4.0},
		searchSQL:   `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $6)`,
		searchRank:  `ts_rank(to_tsvector('simple', "name"), websearch_to_tsquery('simple', $6))`,
		searchArgs:  []any{"alice"},
	}
	sortFields := []SortField{{Column: "id", Desc: false}}
	cursorWhere := `"id" > $7`
	cursorArgs := []any{"uuid-5"}

	q, args := buildListWithCursor(tbl, opts, sortFields, cursorWhere, cursorArgs)

	testutil.Contains(t, q, `"email" = $1`)
	testutil.Contains(t, q, `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`)
	testutil.Contains(t, q, `websearch_to_tsquery('simple', $6)`)
	testutil.Contains(t, q, `"id" > $7`)
	testutil.Contains(t, q, `ORDER BY "id" ASC`)
	testutil.Contains(t, q, `LIMIT $8`)
	testutil.SliceLen(t, args, 8)
	testutil.Equal(t, "test@example.com", args[0])
	testutil.Equal(t, -1.0, args[1].(float64))
	testutil.Equal(t, "alice", args[5])
	testutil.Equal(t, "uuid-5", args[6])
	testutil.Equal(t, 6, args[7])
}

func TestBuildListWithCursorDistanceSortIncludesProjectionAndOrdering(t *testing.T) {
	tbl := cursorSpatialTable()
	opts := listOpts{
		perPage:     5,
		filterSQL:   `"name" = $1`,
		filterArgs:  []any{"test"},
		spatialSQL:  `ST_Intersects("location", ST_MakeEnvelope($2, $3, $4, $5, 4326))`,
		spatialArgs: []any{-1.0, -2.0, 3.0, 4.0},
		searchSQL:   `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $6)`,
		searchArgs:  []any{"alice"},
	}

	parsedSort, err := parseStructuredSort(tbl, "location.distance(-73.9,40.7),id", true)
	testutil.NoError(t, err)
	opts.sort = ensureStructuredSortPKTiebreaker(tbl, parsedSort)

	resolved, err := resolveStructuredSort(opts.sort, len(opts.filterArgs)+len(opts.spatialArgs)+len(opts.searchArgs)+1)
	testutil.NoError(t, err)
	opts.sortFields = resolved.Fields
	opts.sortArgs = resolved.Args
	opts.distanceSelect = resolved.DistanceSelect

	cursorWhere, cursorArgs, err := buildCursorWhere(opts.sortFields, []any{123.4, float64(5)}, len(opts.filterArgs)+len(opts.spatialArgs)+len(opts.searchArgs)+len(opts.sortArgs)+1)
	testutil.NoError(t, err)

	q, args := buildListWithCursor(tbl, opts, opts.sortFields, cursorWhere, cursorArgs)
	testutil.Contains(t, q, `AS "_distance"`)
	testutil.Contains(t, q, `ORDER BY ST_Distance("location", ST_SetSRID(ST_MakePoint($7, $8), 4326)) ASC, "id" ASC`)
	testutil.Contains(t, q, `(ST_Distance("location", ST_SetSRID(ST_MakePoint($7, $8), 4326)), "id") > ($9, $10)`)
	testutil.Contains(t, q, `LIMIT $11`)
	testutil.SliceLen(t, args, 11)
	testutil.Equal(t, -73.9, args[6])
	testutil.Equal(t, 40.7, args[7])
	testutil.Equal(t, 123.4, args[8])
	testutil.Equal(t, float64(5), args[9].(float64))
	testutil.Equal(t, 6, args[10])
}

func TestBuildListWithCursorDistanceSortFieldsOmitTieBreakerProjectsHiddenColumn(t *testing.T) {
	tbl := cursorSpatialTable()
	opts := listOpts{
		perPage: 5,
		fields:  []string{"name"},
	}

	parsedSort, err := parseStructuredSort(tbl, "location.distance(-73.9,40.7),id", true)
	testutil.NoError(t, err)
	opts.sort = ensureStructuredSortPKTiebreaker(tbl, parsedSort)

	resolved, err := resolveStructuredSort(opts.sort, 1)
	testutil.NoError(t, err)
	cursorProjection := prepareCursorSortProjection(tbl, opts.fields, resolved.Fields)
	opts.sortFields = cursorProjection.Fields
	opts.cursorSelects = cursorProjection.Selects
	opts.sortArgs = resolved.Args
	opts.distanceSelect = resolved.DistanceSelect

	cursorWhere, cursorArgs, err := buildCursorWhere(opts.sortFields, []any{123.4, float64(5)}, len(opts.sortArgs)+1)
	testutil.NoError(t, err)

	q, args := buildListWithCursor(tbl, opts, opts.sortFields, cursorWhere, cursorArgs)
	testutil.Contains(t, q, `"name", ST_Distance("location", ST_SetSRID(ST_MakePoint($1, $2), 4326)) AS "_distance", "id" AS "__cursor_sort_1"`)
	testutil.Contains(t, q, `(ST_Distance("location", ST_SetSRID(ST_MakePoint($1, $2), 4326)), "id") > ($3, $4)`)
	testutil.Contains(t, q, `LIMIT $5`)
	testutil.SliceLen(t, args, 5)
	testutil.Equal(t, -73.9, args[0])
	testutil.Equal(t, 40.7, args[1])
	testutil.Equal(t, 123.4, args[2])
	testutil.Equal(t, float64(5), args[3].(float64))
	testutil.Equal(t, 6, args[4])
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Handler-level tests ---

func TestHandleList_CursorAndPageMutualExclusion(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?cursor=abc&page=2", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	if !searchString(resp.Message, "mutually exclusive") {
		t.Fatalf("expected mutual exclusion error, got: %s", resp.Message)
	}
}

func TestHandleList_CursorInvalidBase64(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?cursor=not-valid!!!", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleList_CursorEmptyValue_FirstPage(t *testing.T) {
	// cursor= (empty value) should be treated as cursor mode, first page.
	// Without a real DB this will fail at query execution, but the handler
	// should not error on parameter parsing.
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?cursor=", "")
	// Will get 500 because no DB — but NOT 400 from param validation.
	// Anything other than 400 is acceptable here.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("empty cursor should not cause 400, got: %d", w.Code)
	}
}

func TestHandleList_CursorModeResponseShape(t *testing.T) {
	// Verify the response type is CursorListResponse, not ListResponse.
	// We can't fully test without a DB, but we can check that the cursor path
	// doesn't return offset pagination fields.
	// This test documents the intent — integration tests cover the full flow.
}

func TestHandleList_CursorWithPerPage(t *testing.T) {
	// cursor mode should respect perPage.
	h := testHandler(testSchema())
	// perPage without page — should work with cursor mode.
	w := doRequest(h, "GET", "/collections/users?cursor=&perPage=10", "")
	// Should not get 400.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("cursor with perPage should not cause 400, got: %d", w.Code)
	}
}

func TestHandleList_CursorPageDefaultNotRejected(t *testing.T) {
	// cursor + page=1 (default) should NOT be rejected since page defaults to 1.
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?cursor=&page=1", "")
	if w.Code == http.StatusBadRequest {
		resp := decodeError(t, w)
		if searchString(resp.Message, "mutually exclusive") {
			t.Fatalf("cursor + page=1 should not be rejected as mutual exclusion")
		}
	}
}

// --- CursorListResponse JSON shape test ---

func TestCursorListResponse_JSON(t *testing.T) {
	resp := CursorListResponse{
		PerPage:    10,
		NextCursor: "abc123",
		Items:      []map[string]any{{"id": "1"}},
	}
	data, err := json.Marshal(resp)
	testutil.NoError(t, err)
	var m map[string]any
	testutil.NoError(t, json.Unmarshal(data, &m))
	if _, ok := m["perPage"]; !ok {
		t.Fatal("expected perPage in JSON")
	}
	if _, ok := m["nextCursor"]; !ok {
		t.Fatal("expected nextCursor in JSON")
	}
	if _, ok := m["items"]; !ok {
		t.Fatal("expected items in JSON")
	}
	// Should NOT have offset pagination fields.
	if _, ok := m["page"]; ok {
		t.Fatal("unexpected page in cursor response")
	}
	if _, ok := m["totalItems"]; ok {
		t.Fatal("unexpected totalItems in cursor response")
	}
}

func TestCursorListResponse_OmitEmptyNextCursor(t *testing.T) {
	resp := CursorListResponse{
		PerPage: 10,
		Items:   []map[string]any{},
	}
	data, err := json.Marshal(resp)
	testutil.NoError(t, err)
	var m map[string]any
	testutil.NoError(t, json.Unmarshal(data, &m))
	if _, ok := m["nextCursor"]; ok {
		t.Fatal("expected nextCursor to be omitted when empty")
	}
}

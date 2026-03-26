package graphql

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func mutationTestTable() *schema.Table {
	tbl := testTable()
	tbl.Columns = append(tbl.Columns, &schema.Column{Name: "meta", TypeName: "jsonb", IsJSON: true, IsNullable: true})
	tbl.Indexes = []*schema.Index{
		{Name: "posts_pkey", IsPrimary: true, IsUnique: true},
		{Name: "posts_title_key", IsUnique: true},
	}
	return tbl
}

func spatialMutationTestTable() *schema.Table {
	tbl := mutationTestTable()
	tbl.Columns = append(tbl.Columns, &schema.Column{
		Name:         "location",
		TypeName:     "geometry",
		IsGeometry:   true,
		GeometryType: "Point",
		SRID:         4326,
	})
	return tbl
}

func expectArgs(t *testing.T, args []any, expected ...any) {
	t.Helper()
	testutil.Equal(t, len(expected), len(args))
	for idx := range expected {
		testutil.Equal(t, expected[idx], args[idx])
	}
}

func buildSingleInsertStatement(tbl *schema.Table, object map[string]interface{}, onConflict map[string]interface{}) (string, []any, error) {
	return buildBatchInsertStatement(tbl, []map[string]interface{}{object}, onConflict)
}

func TestBuildInsertStatementBasic(t *testing.T) {
	t.Parallel()
	sql, args, err := buildSingleInsertStatement(
		mutationTestTable(),
		map[string]interface{}{"title": "Hello", "score": 10},
		nil,
	)
	testutil.NoError(t, err)
	testutil.Equal(t, `INSERT INTO "public"."posts" ("score", "title") VALUES ($1, $2) RETURNING *`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 10, args[0])
	testutil.Equal(t, "Hello", args[1])
}

func TestCollectMutationEventsByOperation(t *testing.T) {
	t.Parallel()

	ctx := ctxWithMutationEventCollector(context.Background())
	tbl := mutationTestTable()
	tbl.PrimaryKey = []string{"id"}

	rows := []map[string]any{
		{"id": 1, "title": "A"},
		{"id": 2, "title": "B"},
	}

	collectMutationEvents(ctx, tbl, "insert", rows)
	collectMutationEvents(ctx, tbl, "update", rows[:1])
	collectMutationEvents(ctx, tbl, "delete", rows[:1])

	events := mutationEventsFromContext(ctx)
	testutil.Equal(t, 4, len(events))
	testutil.Equal(t, "create", events[0].Action)
	testutil.Equal(t, "update", events[2].Action)
	testutil.Equal(t, "delete", events[3].Action)
	testutil.Equal(t, 1, events[3].Record["id"])
	testutil.True(t, reflect.DeepEqual(rows[0], events[3].OldRecord), "old record mismatch")
}

func TestCollectUpdateMutationEventsCarriesOldRecordByPrimaryKey(t *testing.T) {
	t.Parallel()

	ctx := ctxWithMutationEventCollector(context.Background())
	tbl := mutationTestTable()
	tbl.PrimaryKey = []string{"id"}

	oldRows := []map[string]any{
		{"id": 1, "title": "Old title"},
	}
	newRows := []map[string]any{
		{"id": 1, "title": "New title"},
	}

	collectUpdateMutationEvents(ctx, tbl, newRows, oldRows)

	events := mutationEventsFromContext(ctx)
	testutil.Equal(t, 1, len(events))
	testutil.Equal(t, "update", events[0].Action)
	testutil.Equal(t, "New title", events[0].Record["title"])
	testutil.Equal(t, "Old title", events[0].OldRecord["title"])
}

func TestBuildInsertStatementDefaultValues(t *testing.T) {
	t.Parallel()
	sql, args, err := buildSingleInsertStatement(mutationTestTable(), map[string]interface{}{}, nil)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`INSERT INTO "public"."posts" ("body", "created_at", "id", "meta", "score", "title") VALUES (DEFAULT, DEFAULT, DEFAULT, DEFAULT, DEFAULT, DEFAULT) RETURNING *`,
		sql,
	)
	testutil.Equal(t, 0, len(args))
}

func TestBuildInsertStatementOnConflictDoUpdate(t *testing.T) {
	t.Parallel()
	sql, args, err := buildSingleInsertStatement(
		mutationTestTable(),
		map[string]interface{}{"id": 1, "title": "Updated"},
		map[string]interface{}{
			"constraint":     "posts_pkey",
			"update_columns": []interface{}{"title"},
		},
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`INSERT INTO "public"."posts" ("id", "title") VALUES ($1, $2) ON CONFLICT ON CONSTRAINT "posts_pkey" DO UPDATE SET "title" = EXCLUDED."title" RETURNING *`,
		sql,
	)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 1, args[0])
	testutil.Equal(t, "Updated", args[1])
}

func TestBuildInsertStatementOnConflictDedupesUpdateColumns(t *testing.T) {
	t.Parallel()
	sql, _, err := buildSingleInsertStatement(
		mutationTestTable(),
		map[string]interface{}{"id": 1, "title": "Updated"},
		map[string]interface{}{
			"constraint":     "posts_pkey",
			"update_columns": []interface{}{"title", "title"},
		},
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`INSERT INTO "public"."posts" ("id", "title") VALUES ($1, $2) ON CONFLICT ON CONSTRAINT "posts_pkey" DO UPDATE SET "title" = EXCLUDED."title" RETURNING *`,
		sql,
	)
}

func TestBuildInsertStatementRejectsUnknownUpdateColumn(t *testing.T) {
	t.Parallel()
	_, _, err := buildSingleInsertStatement(
		mutationTestTable(),
		map[string]interface{}{"id": 1, "title": "Updated"},
		map[string]interface{}{
			"constraint":     "posts_pkey",
			"update_columns": []interface{}{"not_a_column"},
		},
	)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unknown column")
}

func TestBuildUpdateStatement(t *testing.T) {
	t.Parallel()
	sql, args, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"title": map[string]interface{}{"_eq": "Second Post"}},
		map[string]interface{}{"body": "Updated", "score": 7},
		nil,
		nil,
		nil,
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`UPDATE "public"."posts" SET "body" = $1, "score" = $2 WHERE "title" = $3 RETURNING *`,
		sql,
	)
	testutil.Equal(t, 3, len(args))
	testutil.Equal(t, "Updated", args[0])
	testutil.Equal(t, 7, args[1])
	testutil.Equal(t, "Second Post", args[2])
}

func TestBuildUpdateStatementRejectsUnknownSetColumn(t *testing.T) {
	t.Parallel()
	_, _, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"title": map[string]interface{}{"_eq": "Second Post"}},
		map[string]interface{}{"not_a_column": "x"},
		nil,
		nil,
		nil,
	)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unknown column")
}

func TestBuildDeleteStatement(t *testing.T) {
	t.Parallel()
	sql, args, err := buildDeleteStatement(
		mutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 7}},
	)
	testutil.NoError(t, err)
	testutil.Equal(t, `DELETE FROM "public"."posts" WHERE "id" = $1 RETURNING *`, sql)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, 7, args[0])
}

func TestBuildInsertStatementSpatialUsesGeoJSONReturningProjection(t *testing.T) {
	t.Parallel()

	sql, _, err := buildSingleInsertStatement(
		spatialMutationTestTable(),
		map[string]interface{}{"title": "Hello"},
		nil,
	)
	testutil.NoError(t, err)
	testutil.Contains(t, sql, `RETURNING `)
	testutil.Contains(t, sql, `ST_AsGeoJSON("location")::jsonb AS "location"`)
	testutil.False(t, strings.HasSuffix(sql, "RETURNING *"), "spatial mutations must not return raw geometry values")
}

func TestBuildUpdateStatementSpatialUsesGeoJSONReturningProjection(t *testing.T) {
	t.Parallel()

	sql, _, err := buildUpdateStatement(
		spatialMutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 1}},
		map[string]interface{}{"title": "Updated"},
		nil,
		nil,
		nil,
	)
	testutil.NoError(t, err)
	testutil.Contains(t, sql, `RETURNING `)
	testutil.Contains(t, sql, `ST_AsGeoJSON("location")::jsonb AS "location"`)
}

func TestBuildDeleteStatementSpatialUsesGeoJSONReturningProjection(t *testing.T) {
	t.Parallel()

	sql, _, err := buildDeleteStatement(
		spatialMutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 1}},
	)
	testutil.NoError(t, err)
	testutil.Contains(t, sql, `RETURNING `)
	testutil.Contains(t, sql, `ST_AsGeoJSON("location")::jsonb AS "location"`)
}

func TestBuildBatchInsertStatementSameColumns(t *testing.T) {
	t.Parallel()
	sql, args, err := buildBatchInsertStatement(
		mutationTestTable(),
		[]map[string]interface{}{
			{"id": 1, "title": "A"},
			{"id": 2, "title": "B"},
		},
		nil,
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`INSERT INTO "public"."posts" ("id", "title") VALUES ($1, $2), ($3, $4) RETURNING *`,
		sql,
	)
	expectArgs(t, args, 1, "A", 2, "B")
}

func TestBuildBatchInsertStatementUsesDefaultForMissingColumns(t *testing.T) {
	t.Parallel()
	sql, args, err := buildBatchInsertStatement(
		mutationTestTable(),
		[]map[string]interface{}{
			{"id": 1, "title": "A"},
			{"id": 2},
		},
		nil,
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`INSERT INTO "public"."posts" ("id", "title") VALUES ($1, $2), ($3, DEFAULT) RETURNING *`,
		sql,
	)
	expectArgs(t, args, 1, "A", 2)
}

func TestBuildBatchInsertStatementWithOnConflict(t *testing.T) {
	t.Parallel()
	sql, args, err := buildBatchInsertStatement(
		mutationTestTable(),
		[]map[string]interface{}{
			{"id": 1, "title": "A"},
			{"id": 2, "title": "B"},
		},
		map[string]interface{}{
			"constraint":     "posts_pkey",
			"update_columns": []interface{}{"title"},
		},
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`INSERT INTO "public"."posts" ("id", "title") VALUES ($1, $2), ($3, $4) ON CONFLICT ON CONSTRAINT "posts_pkey" DO UPDATE SET "title" = EXCLUDED."title" RETURNING *`,
		sql,
	)
	expectArgs(t, args, 1, "A", 2, "B")
}

func TestBuildBatchInsertStatementRejectsUnknownColumns(t *testing.T) {
	t.Parallel()
	_, _, err := buildBatchInsertStatement(
		mutationTestTable(),
		[]map[string]interface{}{
			{"id": 1, "not_a_column": "A"},
		},
		nil,
	)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unknown column")
}

func TestBuildUpdateStatementIncNumericColumn(t *testing.T) {
	t.Parallel()
	sql, args, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 7}},
		nil,
		map[string]interface{}{"score": 2},
		nil,
		nil,
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`UPDATE "public"."posts" SET "score" = "score" + $1 WHERE "id" = $2 RETURNING *`,
		sql,
	)
	expectArgs(t, args, 2, 7)
}

func TestBuildUpdateStatementAppendJSONColumn(t *testing.T) {
	t.Parallel()
	sql, args, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 7}},
		nil,
		nil,
		map[string]interface{}{"meta": `{"a":1}`},
		nil,
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`UPDATE "public"."posts" SET "meta" = "meta" || $1::jsonb WHERE "id" = $2 RETURNING *`,
		sql,
	)
	expectArgs(t, args, `{"a":1}`, 7)
}

func TestBuildUpdateStatementPrependJSONColumn(t *testing.T) {
	t.Parallel()
	sql, args, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 7}},
		nil,
		nil,
		nil,
		map[string]interface{}{"meta": `{"a":1}`},
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`UPDATE "public"."posts" SET "meta" = $1::jsonb || "meta" WHERE "id" = $2 RETURNING *`,
		sql,
	)
	expectArgs(t, args, `{"a":1}`, 7)
}

func TestBuildUpdateStatementIncNonNumericColumn(t *testing.T) {
	t.Parallel()
	_, _, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 7}},
		nil,
		map[string]interface{}{"title": 2},
		nil,
		nil,
	)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "is not numeric")
}

func TestBuildUpdateStatementSetAndIncCombined(t *testing.T) {
	t.Parallel()
	sql, args, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 7}},
		map[string]interface{}{"body": "Updated"},
		map[string]interface{}{"score": 2},
		nil,
		nil,
	)
	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`UPDATE "public"."posts" SET "body" = $1, "score" = "score" + $2 WHERE "id" = $3 RETURNING *`,
		sql,
	)
	expectArgs(t, args, "Updated", 2, 7)
}

func TestBuildUpdateStatementRejectsOperatorColumnOverlap(t *testing.T) {
	t.Parallel()
	_, _, err := buildUpdateStatement(
		mutationTestTable(),
		map[string]interface{}{"id": map[string]interface{}{"_eq": 7}},
		map[string]interface{}{"score": 1},
		map[string]interface{}{"score": 2},
		nil,
		nil,
	)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "appears in both _set and _inc")
}

func TestResolveUpdateMutationRequiresAtLeastOneOperator(t *testing.T) {
	t.Parallel()
	_, err := resolveUpdateMutation(context.Background(), mutationTestTable(), nil, map[string]interface{}{
		"where": map[string]interface{}{"id": map[string]interface{}{"_eq": 1}},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "at least one of _set, _inc, _append, _prepend")
}

func TestResolveUpdateMutationRejectsOperatorColumnOverlap(t *testing.T) {
	t.Parallel()
	_, err := resolveUpdateMutation(context.Background(), mutationTestTable(), nil, map[string]interface{}{
		"where": map[string]interface{}{"id": map[string]interface{}{"_eq": 1}},
		"_set":  map[string]interface{}{"score": 1},
		"_inc":  map[string]interface{}{"score": 2},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "appears in both _set and _inc")
}

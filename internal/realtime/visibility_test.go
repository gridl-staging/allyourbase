package realtime

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildVisibilityCheckSinglePK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
	}
	record := map[string]any{"id": 42, "title": "Hello"}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, `SELECT 1 FROM "public"."posts" WHERE "id" = $1`, query)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, 42, args[0])
}

func TestBuildVisibilityCheckCompositePK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "order_items",
		PrimaryKey: []string{"order_id", "item_id"},
	}
	record := map[string]any{"order_id": 1, "item_id": 5, "qty": 3}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, `SELECT 1 FROM "public"."order_items" WHERE "order_id" = $1 AND "item_id" = $2`, query)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 1, args[0])
	testutil.Equal(t, 5, args[1])
}

func TestBuildVisibilityCheckMissingPK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
	}
	record := map[string]any{"title": "Hello"} // no "id"
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, "", query)
	testutil.Nil(t, args)
}

func TestCanSeeRecordNilPool(t *testing.T) {
	// When pool is nil, RLS filtering is disabled — all events pass through.
	t.Parallel()

	h := &Handler{pool: nil}
	event := &Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}}
	testutil.True(t, h.canSeeRecord(context.TODO(), nil, event), "nil pool should allow all events")
}

func TestCanSeeRecordNilPoolAllActions(t *testing.T) {
	// Verify nil pool allows all event types.
	t.Parallel()

	h := &Handler{pool: nil}
	for _, action := range []string{"create", "update", "delete"} {
		event := &Event{Action: action, Table: "posts", Record: map[string]any{"id": 1}}
		testutil.True(t, h.canSeeRecord(context.TODO(), nil, event),
			"nil pool should allow %s events", action)
	}
}

func TestBuildVisibilityCheckQuotesIdentifiers(t *testing.T) {
	// Verify schema, table, and column names are properly double-quoted.
	t.Parallel()

	tbl := &schema.Table{
		Schema:     "my_schema",
		Name:       "my_table",
		PrimaryKey: []string{"my_col"},
	}
	record := map[string]any{"my_col": 1}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Contains(t, query, `"my_schema"`)
	testutil.Contains(t, query, `"my_table"`)
	testutil.Contains(t, query, `"my_col"`)
	testutil.Equal(t, 1, len(args))
}

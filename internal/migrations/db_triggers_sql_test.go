package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDBTriggersMigrationsSQL(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql034 := read(t, "034_ayb_edge_db_triggers.sql")

	// DB triggers metadata table
	testutil.True(t, strings.Contains(sql034, "CREATE TABLE IF NOT EXISTS _ayb_edge_db_triggers"),
		"034 must create _ayb_edge_db_triggers table")
	testutil.True(t, strings.Contains(sql034, "id              UUID PRIMARY KEY DEFAULT gen_random_uuid()"),
		"034 must define UUID primary key with gen_random_uuid default")
	testutil.True(t, strings.Contains(sql034, "function_id     UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE"),
		"034 must enforce function FK with cascade delete")
	testutil.True(t, strings.Contains(sql034, "table_name      TEXT NOT NULL"),
		"034 must define table_name as non-null text")
	testutil.True(t, strings.Contains(sql034, "schema_name     TEXT NOT NULL DEFAULT 'public'"),
		"034 must define schema_name with default 'public'")
	testutil.True(t, strings.Contains(sql034, "events          TEXT[] NOT NULL"),
		"034 must define events as non-null text array")
	testutil.True(t, strings.Contains(sql034, "filter_columns  TEXT[]"),
		"034 must define filter_columns as nullable text array")
	testutil.True(t, strings.Contains(sql034, "enabled         BOOLEAN NOT NULL DEFAULT true"),
		"034 must define enabled with default true")
	testutil.True(t, strings.Contains(sql034, "UNIQUE (function_id, table_name, schema_name)"),
		"034 must enforce unique constraint on function + table + schema")
	testutil.True(t, strings.Contains(sql034, "CREATE INDEX IF NOT EXISTS idx_ayb_edge_db_triggers_function"),
		"034 must create function_id index")
	testutil.True(t, strings.Contains(sql034, "CREATE INDEX IF NOT EXISTS idx_ayb_edge_db_triggers_table"),
		"034 must create table lookup index")
	testutil.True(t, strings.Contains(sql034, "WHERE enabled = true"),
		"034 table index must be partial (enabled only)")

	// Event queue table
	testutil.True(t, strings.Contains(sql034, "CREATE TABLE IF NOT EXISTS _ayb_edge_trigger_events"),
		"034 must create _ayb_edge_trigger_events queue table")
	testutil.True(t, strings.Contains(sql034, "trigger_id      UUID NOT NULL REFERENCES _ayb_edge_db_triggers(id) ON DELETE CASCADE"),
		"034 must enforce trigger FK with cascade delete on events")
	testutil.True(t, strings.Contains(sql034, "operation       TEXT NOT NULL"),
		"034 must define operation as non-null text")
	testutil.True(t, strings.Contains(sql034, "payload         JSONB"),
		"034 must define payload as JSONB")
	testutil.True(t, strings.Contains(sql034, "status          TEXT NOT NULL DEFAULT 'pending'"),
		"034 must define status with default 'pending'")
	testutil.True(t, strings.Contains(sql034, "attempts        INTEGER NOT NULL DEFAULT 0"),
		"034 must define attempts counter")
	testutil.True(t, strings.Contains(sql034, "CREATE INDEX IF NOT EXISTS idx_ayb_edge_trigger_events_pending"),
		"034 must create pending events index for SKIP LOCKED polling")

	// PG trigger function
	testutil.True(t, strings.Contains(sql034, "CREATE OR REPLACE FUNCTION _ayb_edge_notify()"),
		"034 must create _ayb_edge_notify trigger function")
	testutil.True(t, strings.Contains(sql034, "RETURNS trigger"),
		"034 _ayb_edge_notify must return trigger")
	testutil.True(t, strings.Contains(sql034, "ayb.trigger_depth"),
		"034 must check ayb.trigger_depth for recursion guard")
	testutil.True(t, strings.Contains(sql034, "_ayb_edge_trigger_events"),
		"034 trigger function must insert into event queue")
	testutil.True(t, strings.Contains(sql034, "pg_notify('ayb_edge_trigger'"),
		"034 must issue pg_notify for low-latency wakeup")
	testutil.True(t, strings.Contains(sql034, "TG_OP"),
		"034 must use TG_OP for operation detection")
}

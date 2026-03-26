package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestStorageTriggersMigrationsSQL(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql032 := read(t, "032_ayb_edge_storage_triggers.sql")
	testutil.True(t, strings.Contains(sql032, "CREATE TABLE IF NOT EXISTS _ayb_edge_storage_triggers"),
		"032 must create _ayb_edge_storage_triggers table")
	testutil.True(t, strings.Contains(sql032, "id              UUID PRIMARY KEY DEFAULT gen_random_uuid()"),
		"032 must define UUID primary key with gen_random_uuid default")
	testutil.True(t, strings.Contains(sql032, "function_id     UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE"),
		"032 must enforce function FK with cascade delete")
	testutil.True(t, strings.Contains(sql032, "bucket          TEXT NOT NULL"),
		"032 must define bucket as non-null text")
	testutil.True(t, strings.Contains(sql032, "event_types     TEXT[] NOT NULL"),
		"032 must define event_types as non-null text array")
	testutil.True(t, strings.Contains(sql032, "prefix_filter   TEXT"),
		"032 must define prefix_filter")
	testutil.True(t, strings.Contains(sql032, "suffix_filter   TEXT"),
		"032 must define suffix_filter")
	testutil.True(t, strings.Contains(sql032, "enabled         BOOLEAN NOT NULL DEFAULT true"),
		"032 must define enabled with default true")
	testutil.True(t, strings.Contains(sql032, "created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"032 must define created_at timestamp")
	testutil.True(t, strings.Contains(sql032, "updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"032 must define updated_at timestamp")
	testutil.True(t, strings.Contains(sql032, "CREATE INDEX IF NOT EXISTS idx_ayb_edge_storage_triggers_function"),
		"032 must create function_id index")
	testutil.True(t, strings.Contains(sql032, "CREATE INDEX IF NOT EXISTS idx_ayb_edge_storage_triggers_bucket"),
		"032 must create bucket index")
	testutil.True(t, strings.Contains(sql032, "WHERE enabled = true"),
		"032 bucket index must be partial (enabled only)")
}

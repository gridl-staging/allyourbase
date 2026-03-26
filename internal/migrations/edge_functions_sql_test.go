package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestEdgeFunctionMigrationsSQL(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql029 := read(t, "029_ayb_edge_functions.sql")
	testutil.True(t, strings.Contains(sql029, "CREATE TABLE IF NOT EXISTS _ayb_edge_functions"),
		"029 must create _ayb_edge_functions table")
	testutil.True(t, strings.Contains(sql029, "id              UUID PRIMARY KEY DEFAULT gen_random_uuid()"),
		"029 must define UUID primary key with gen_random_uuid default")
	testutil.True(t, strings.Contains(sql029, "name            TEXT NOT NULL UNIQUE"),
		"029 must enforce unique function names")
	testutil.True(t, strings.Contains(sql029, "entry_point     TEXT NOT NULL DEFAULT 'handler'"),
		"029 must default entry_point to handler")
	testutil.True(t, strings.Contains(sql029, "source          TEXT NOT NULL"),
		"029 must persist source")
	testutil.True(t, strings.Contains(sql029, "compiled_js     TEXT NOT NULL"),
		"029 must persist compiled JavaScript")
	testutil.True(t, strings.Contains(sql029, "timeout_ms      INT NOT NULL DEFAULT 5000"),
		"029 must define timeout_ms with default")
	testutil.True(t, strings.Contains(sql029, "env_vars        JSONB NOT NULL DEFAULT '{}'::jsonb"),
		"029 must default env_vars to empty json object")
	testutil.True(t, strings.Contains(sql029, "public          BOOLEAN NOT NULL DEFAULT false"),
		"029 must default public to false")
	testutil.True(t, strings.Contains(sql029, "created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"029 must define created_at timestamp")
	testutil.True(t, strings.Contains(sql029, "updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"029 must define updated_at timestamp")

	sql030 := read(t, "030_ayb_edge_function_logs.sql")
	testutil.True(t, strings.Contains(sql030, "CREATE TABLE IF NOT EXISTS _ayb_edge_function_logs"),
		"030 must create _ayb_edge_function_logs table")
	testutil.True(t, strings.Contains(sql030, "function_id      UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE"),
		"030 must enforce function FK with cascade delete")
	testutil.True(t, strings.Contains(sql030, "invocation_id    UUID NOT NULL"),
		"030 must persist invocation id")
	testutil.True(t, strings.Contains(sql030, "status           TEXT NOT NULL"),
		"030 must persist invocation status")
	testutil.True(t, strings.Contains(sql030, "duration_ms      INT NOT NULL"),
		"030 must persist duration")
	testutil.True(t, strings.Contains(sql030, "stdout           TEXT"),
		"030 must persist stdout")
	testutil.True(t, strings.Contains(sql030, "error            TEXT"),
		"030 must persist error")
	testutil.True(t, strings.Contains(sql030, "request_method   TEXT"),
		"030 must persist request method")
	testutil.True(t, strings.Contains(sql030, "request_path     TEXT"),
		"030 must persist request path")
	testutil.True(t, strings.Contains(sql030, "created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"030 must define created_at timestamp")
	testutil.True(t, strings.Contains(sql030, "CREATE INDEX IF NOT EXISTS idx_ayb_edge_function_logs_function_created"),
		"030 must create function_id/created_at index")
	testutil.True(t, strings.Contains(sql030, "ON _ayb_edge_function_logs (function_id, created_at DESC)"),
		"030 index must optimize function log history lookups")
}

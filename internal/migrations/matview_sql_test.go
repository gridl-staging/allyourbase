package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestMatviewMigrationSQLConstraints(t *testing.T) {
	t.Parallel()

	b, err := fs.ReadFile(embeddedMigrations, "sql/025_ayb_matview_refreshes.sql")
	testutil.NoError(t, err)
	sql025 := string(b)

	testutil.True(t, strings.Contains(sql025, "_ayb_matview_refreshes"),
		"025 must create _ayb_matview_refreshes table")
	testutil.True(t, strings.Contains(sql025, "schema_name"),
		"025 must include schema_name column")
	testutil.True(t, strings.Contains(sql025, "view_name"),
		"025 must include view_name column")
	testutil.True(t, strings.Contains(sql025, "refresh_mode"),
		"025 must include refresh_mode column")
	testutil.True(t, strings.Contains(sql025, "CHECK (refresh_mode IN ('standard', 'concurrent'))"),
		"025 must enforce refresh_mode enum")
	testutil.True(t, strings.Contains(sql025, "CHECK (last_refresh_status IN ('success', 'error'))"),
		"025 must enforce last_refresh_status enum")
	testutil.True(t, strings.Contains(sql025, "UNIQUE (schema_name, view_name)"),
		"025 must enforce uniqueness on (schema_name, view_name)")
	testutil.True(t, strings.Contains(sql025, "CHECK (schema_name ~ '^[A-Za-z_][A-Za-z0-9_]*$')"),
		"025 must enforce schema_name identifier format")
	testutil.True(t, strings.Contains(sql025, "CHECK (view_name ~ '^[A-Za-z_][A-Za-z0-9_]*$')"),
		"025 must enforce view_name identifier format")
}

package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestLogTriggerMetadataMigrationSQL(t *testing.T) {
	t.Parallel()

	b, err := fs.ReadFile(embeddedMigrations, "sql/033_ayb_edge_function_logs_trigger_metadata.sql")
	testutil.NoError(t, err)
	sql033 := string(b)

	testutil.True(t, strings.Contains(sql033, "ALTER TABLE _ayb_edge_function_logs"),
		"033 must alter _ayb_edge_function_logs table")
	testutil.True(t, strings.Contains(sql033, "trigger_type"),
		"033 must add trigger_type column")
	testutil.True(t, strings.Contains(sql033, "trigger_id"),
		"033 must add trigger_id column")
	testutil.True(t, strings.Contains(sql033, "parent_invocation_id"),
		"033 must add parent_invocation_id column")
}

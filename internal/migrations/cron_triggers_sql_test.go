package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestCronTriggersMigrationsSQL(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql031 := read(t, "031_ayb_edge_cron_triggers.sql")
	testutil.True(t, strings.Contains(sql031, "CREATE TABLE IF NOT EXISTS _ayb_edge_cron_triggers"),
		"031 must create _ayb_edge_cron_triggers table")
	testutil.True(t, strings.Contains(sql031, "id              UUID PRIMARY KEY DEFAULT gen_random_uuid()"),
		"031 must define UUID primary key with gen_random_uuid default")
	testutil.True(t, strings.Contains(sql031, "function_id     UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE"),
		"031 must enforce function FK with cascade delete")
	testutil.True(t, strings.Contains(sql031, "schedule_id     UUID NOT NULL REFERENCES _ayb_job_schedules(id) ON DELETE CASCADE"),
		"031 must enforce schedule FK with cascade delete")
	testutil.True(t, strings.Contains(sql031, "payload         JSONB NOT NULL DEFAULT '{}'::jsonb"),
		"031 must default payload to empty json object")
	testutil.True(t, strings.Contains(sql031, "created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"031 must define created_at timestamp")
	testutil.True(t, strings.Contains(sql031, "updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"031 must define updated_at timestamp")
	testutil.True(t, strings.Contains(sql031, "CREATE INDEX IF NOT EXISTS idx_ayb_edge_cron_triggers_function"),
		"031 must create function_id index")
}

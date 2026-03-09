package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestJobsMigrationSQLConstraints(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql023 := read(t, "023_ayb_jobs.sql")
	testutil.True(t, strings.Contains(sql023, "_ayb_jobs"),
		"023 must create _ayb_jobs table")
	testutil.True(t, strings.Contains(sql023, "CHECK (state IN ('queued', 'running', 'completed', 'failed', 'canceled'))"),
		"023 must enforce allowed state values")
	testutil.True(t, strings.Contains(sql023, "CHECK (max_attempts >= 1)"),
		"023 must enforce max_attempts >= 1")
	testutil.True(t, strings.Contains(sql023, "idx_ayb_jobs_claimable"),
		"023 must create claimable partial index")
	testutil.True(t, strings.Contains(sql023, "ON _ayb_jobs (state, run_at)"),
		"023 claimable index must include (state, run_at) key columns")
	testutil.True(t, strings.Contains(sql023, "idx_ayb_jobs_lease"),
		"023 must create lease partial index")
	testutil.True(t, strings.Contains(sql023, "ON _ayb_jobs (state, lease_until)"),
		"023 lease index must include (state, lease_until) key columns")
	testutil.True(t, strings.Contains(sql023, "idx_ayb_jobs_idempotency"),
		"023 must create idempotency unique index")
	testutil.True(t, strings.Contains(sql023, "WHERE state = 'queued'"),
		"023 claimable index must be partial on queued state")
	testutil.True(t, strings.Contains(sql023, "WHERE state = 'running'"),
		"023 lease index must be partial on running state")

	sql024 := read(t, "024_ayb_job_schedules.sql")
	testutil.True(t, strings.Contains(sql024, "_ayb_job_schedules"),
		"024 must create _ayb_job_schedules table")
	testutil.True(t, strings.Contains(sql024, "cron_expr"),
		"024 must have cron_expr column")
	testutil.True(t, strings.Contains(sql024, "timezone"),
		"024 must have timezone column")
	testutil.True(t, strings.Contains(sql024, "UNIQUE"),
		"024 must enforce unique schedule names")
	testutil.True(t, strings.Contains(sql024, "fk_ayb_jobs_schedule"),
		"024 must add FK from jobs to schedules")
	testutil.True(t, strings.Contains(sql024, "table_schema = 'public'"),
		"024 FK idempotency check must be schema-qualified to avoid cross-schema false positives")
	testutil.True(t, strings.Contains(sql024, "ON DELETE SET NULL"),
		"024 FK must clear jobs.schedule_id when a schedule is deleted")
}

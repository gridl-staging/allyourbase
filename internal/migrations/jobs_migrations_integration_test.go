//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestJobsMigrationsConstraintsAndUniqueness(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	var jobsExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_jobs'
		)`).
		Scan(&jobsExists)
	testutil.NoError(t, err)
	testutil.True(t, jobsExists, "_ayb_jobs table should exist")

	var schedulesExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_job_schedules'
		)`).
		Scan(&schedulesExists)
	testutil.NoError(t, err)
	testutil.True(t, schedulesExists, "_ayb_job_schedules table should exist")

	var scheduleID string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_job_schedules (name, job_type, cron_expr)
		 VALUES ('session_cleanup_hourly', 'stale_session_cleanup', '0 * * * *')
		 RETURNING id`,
	).Scan(&scheduleID)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_job_schedules (name, job_type, cron_expr)
		 VALUES ('session_cleanup_hourly', 'stale_session_cleanup', '0 * * * *')`,
	)
	testutil.True(t, err != nil, "duplicate schedule names should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_job_schedules (name, job_type, cron_expr, max_attempts)
		 VALUES ('bad-max-attempts', 'stale_session_cleanup', '0 * * * *', 0)`,
	)
	testutil.True(t, err != nil, "schedule max_attempts < 1 should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, state) VALUES ('stale_session_cleanup', 'invalid')`,
	)
	testutil.True(t, err != nil, "invalid job state should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, max_attempts) VALUES ('stale_session_cleanup', 0)`,
	)
	testutil.True(t, err != nil, "job max_attempts < 1 should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, idempotency_key)
		 VALUES ('stale_session_cleanup', 'dup-key')`,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, idempotency_key)
		 VALUES ('stale_session_cleanup', 'dup-key')`,
	)
	testutil.True(t, err != nil, "duplicate idempotency_key should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, schedule_id)
		 VALUES ('stale_session_cleanup', 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa')`,
	)
	testutil.True(t, err != nil, "unknown schedule_id should violate FK")

	var linkedJobID string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_jobs (type, schedule_id)
		 VALUES ('stale_session_cleanup', $1)
		 RETURNING id`,
		scheduleID,
	).Scan(&linkedJobID)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`DELETE FROM _ayb_job_schedules WHERE id = $1`,
		scheduleID,
	)
	testutil.NoError(t, err)

	var clearedScheduleID *string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT schedule_id FROM _ayb_jobs WHERE id = $1`,
		linkedJobID,
	).Scan(&clearedScheduleID)
	testutil.NoError(t, err)
	testutil.Nil(t, clearedScheduleID)
}

func TestJobsMigrationsIndexes(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	var claimKeys string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT string_agg(a.attname, ',' ORDER BY k.ordinality)
		 FROM pg_class i
		 JOIN pg_index ix ON ix.indexrelid = i.oid
		 JOIN pg_class t ON t.oid = ix.indrelid
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ordinality) ON TRUE
		 JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
		 WHERE n.nspname = 'public'
		   AND t.relname = '_ayb_jobs'
		   AND i.relname = 'idx_ayb_jobs_claimable'`,
	).Scan(&claimKeys)
	testutil.NoError(t, err)
	testutil.Equal(t, "state,run_at", claimKeys)

	var claimPredicate string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT pg_get_expr(ix.indpred, ix.indrelid)
		 FROM pg_class i
		 JOIN pg_index ix ON ix.indexrelid = i.oid
		 WHERE i.relname = 'idx_ayb_jobs_claimable'`,
	).Scan(&claimPredicate)
	testutil.NoError(t, err)
	testutil.Contains(t, claimPredicate, "state")
	testutil.Contains(t, claimPredicate, "queued")

	var leaseKeys string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT string_agg(a.attname, ',' ORDER BY k.ordinality)
		 FROM pg_class i
		 JOIN pg_index ix ON ix.indexrelid = i.oid
		 JOIN pg_class t ON t.oid = ix.indrelid
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ordinality) ON TRUE
		 JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
		 WHERE n.nspname = 'public'
		   AND t.relname = '_ayb_jobs'
		   AND i.relname = 'idx_ayb_jobs_lease'`,
	).Scan(&leaseKeys)
	testutil.NoError(t, err)
	testutil.Equal(t, "state,lease_until", leaseKeys)

	var leasePredicate string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT pg_get_expr(ix.indpred, ix.indrelid)
		 FROM pg_class i
		 JOIN pg_index ix ON ix.indexrelid = i.oid
		 WHERE i.relname = 'idx_ayb_jobs_lease'`,
	).Scan(&leasePredicate)
	testutil.NoError(t, err)
	testutil.Contains(t, leasePredicate, "state")
	testutil.Contains(t, leasePredicate, "running")

	var idempotencyUnique bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT ix.indisunique
		 FROM pg_class i
		 JOIN pg_index ix ON ix.indexrelid = i.oid
		 WHERE i.relname = 'idx_ayb_jobs_idempotency'`,
	).Scan(&idempotencyUnique)
	testutil.NoError(t, err)
	testutil.True(t, idempotencyUnique, "idx_ayb_jobs_idempotency should be unique")

	var idempotencyPredicate string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT pg_get_expr(ix.indpred, ix.indrelid)
		 FROM pg_class i
		 JOIN pg_index ix ON ix.indexrelid = i.oid
		 WHERE i.relname = 'idx_ayb_jobs_idempotency'`,
	).Scan(&idempotencyPredicate)
	testutil.NoError(t, err)
	testutil.Contains(t, idempotencyPredicate, "idempotency_key")
	testutil.Contains(t, idempotencyPredicate, "IS NOT NULL")
}

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestMatviewMigrationConstraintsAndUniqueness(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	var tableExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_matview_refreshes'
		)`,
	).Scan(&tableExists)
	testutil.NoError(t, err)
	testutil.True(t, tableExists, "_ayb_matview_refreshes table should exist")

	var regID string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_matview_refreshes (schema_name, view_name, refresh_mode)
		 VALUES ('public', 'leaderboard', 'standard')
		 RETURNING id`,
	).Scan(&regID)
	testutil.NoError(t, err)
	testutil.True(t, regID != "", "registration id should be returned")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_matview_refreshes (schema_name, view_name, refresh_mode)
		 VALUES ('public', 'leaderboard', 'standard')`,
	)
	testutil.True(t, err != nil, "duplicate (schema_name, view_name) must be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_matview_refreshes (schema_name, view_name, refresh_mode)
		 VALUES ('public', 'leaderboard2', 'invalid')`,
	)
	testutil.True(t, err != nil, "invalid refresh_mode must be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_matview_refreshes (schema_name, view_name, refresh_mode)
		 VALUES ('bad-schema', 'leaderboard3', 'standard')`,
	)
	testutil.True(t, err != nil, "invalid schema_name identifier must be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_matview_refreshes (schema_name, view_name, refresh_mode)
		 VALUES ('public', 'bad-name', 'standard')`,
	)
	testutil.True(t, err != nil, "invalid view_name identifier must be rejected")
}

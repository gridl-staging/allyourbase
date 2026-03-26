//go:build integration

package status_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/status"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func resetDB(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
}

func runMigrations(t *testing.T, ctx context.Context) {
	t.Helper()
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err := r.Run(ctx)
	testutil.NoError(t, err)
}

func TestPgIncidentStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	runMigrations(t, ctx)

	store := status.NewPgIncidentStore(sharedPG.Pool)

	inc := &status.Incident{
		Title:            "Database latency spike",
		Status:           status.IncidentInvestigating,
		AffectedServices: []string{"database"},
	}
	testutil.NoError(t, store.CreateIncident(ctx, inc))
	testutil.True(t, inc.ID != "")

	active, err := store.ListIncidents(ctx, true)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(active))
	testutil.Equal(t, inc.ID, active[0].ID)

	update := &status.IncidentUpdateEntry{
		Message: "Identified query plan regression",
		Status:  status.IncidentIdentified,
	}
	testutil.NoError(t, store.AddIncidentUpdate(ctx, inc.ID, update))
	testutil.True(t, update.ID != "")

	fetched, err := store.GetIncident(ctx, inc.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(fetched.Updates))
	testutil.Equal(t, status.IncidentIdentified, fetched.Updates[0].Status)
	testutil.Equal(t, status.IncidentIdentified, fetched.Status)
	testutil.Nil(t, fetched.ResolvedAt)

	resolvedAt := time.Now().UTC()
	resolvedStatus := status.IncidentResolved
	testutil.NoError(t, store.UpdateIncident(ctx, inc.ID, &status.IncidentUpdate{
		Status:     &resolvedStatus,
		ResolvedAt: &resolvedAt,
	}))

	afterResolve, err := store.GetIncident(ctx, inc.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, status.IncidentResolved, afterResolve.Status)
	testutil.NotNil(t, afterResolve.ResolvedAt)

	monitoringStatus := status.IncidentMonitoring
	testutil.NoError(t, store.UpdateIncident(ctx, inc.ID, &status.IncidentUpdate{
		Status: &monitoringStatus,
	}))
	afterReopen, err := store.GetIncident(ctx, inc.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, status.IncidentMonitoring, afterReopen.Status)
	testutil.Nil(t, afterReopen.ResolvedAt)

	active, err = store.ListIncidents(ctx, true)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(active))

	all, err := store.ListIncidents(ctx, false)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(all))
	testutil.Equal(t, status.IncidentMonitoring, all[0].Status)
}

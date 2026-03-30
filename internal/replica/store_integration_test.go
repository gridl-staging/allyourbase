//go:build integration

package replica_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/replica"
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

// resetAndMigrate drops and recreates the public schema, then runs all
// migrations so that _ayb_replicas table exists.
func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err = r.Run(ctx)
	testutil.NoError(t, err)
}

func TestReplicaStoreIsEmptyOnFreshDB(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	empty, err := store.IsEmpty(ctx)
	testutil.NoError(t, err)
	testutil.True(t, empty, "fresh DB should have no replicas")
}

func TestReplicaStoreBootstrapAndList(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	// Bootstrap with a primary and a replica.
	records := []replica.TopologyNodeRecord{
		{
			Name:     "primary-1",
			Host:     "db-primary.example.com",
			Port:     5432,
			Database: "mydb",
			SSLMode:  "require",
			Role:     replica.TopologyRolePrimary,
			State:    replica.TopologyStateActive,
		},
		{
			Name:     "replica-1",
			Host:     "db-replica.example.com",
			Port:     5432,
			Database: "mydb",
			SSLMode:  "require",
			Role:     replica.TopologyRoleReplica,
			State:    replica.TopologyStateActive,
		},
	}
	err := store.Bootstrap(ctx, records)
	testutil.NoError(t, err)

	// List should return both nodes ordered by name.
	nodes, err := store.List(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(nodes))
	testutil.Equal(t, "primary-1", nodes[0].Name)
	testutil.Equal(t, replica.TopologyRolePrimary, nodes[0].Role)
	testutil.Equal(t, "replica-1", nodes[1].Name)
	testutil.Equal(t, replica.TopologyRoleReplica, nodes[1].Role)

	// No longer empty.
	empty, err := store.IsEmpty(ctx)
	testutil.NoError(t, err)
	testutil.False(t, empty, "should not be empty after bootstrap")
}

func TestReplicaStoreBootstrapIdempotent(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	record := replica.TopologyNodeRecord{
		Name: "primary-1", Host: "db.example.com", Port: 5432,
		Database: "mydb", Role: replica.TopologyRolePrimary, State: replica.TopologyStateActive,
	}

	// Bootstrap twice — second call should be a no-op (ON CONFLICT DO NOTHING).
	err := store.Bootstrap(ctx, []replica.TopologyNodeRecord{record})
	testutil.NoError(t, err)
	err = store.Bootstrap(ctx, []replica.TopologyNodeRecord{record})
	testutil.NoError(t, err)

	// Still only one node.
	nodes, err := store.List(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(nodes))
}

func TestReplicaStoreGetAndGetNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	err := store.Bootstrap(ctx, []replica.TopologyNodeRecord{
		{Name: "primary-1", Host: "db.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRolePrimary, State: replica.TopologyStateActive},
	})
	testutil.NoError(t, err)

	// Get existing node.
	node, err := store.Get(ctx, "primary-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "primary-1", node.Name)
	testutil.Equal(t, "db.example.com", node.Host)

	// Get nonexistent node.
	_, err = store.Get(ctx, "nonexistent")
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "not found")
}

func TestReplicaStoreAddNode(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	// Bootstrap primary.
	err := store.Bootstrap(ctx, []replica.TopologyNodeRecord{
		{Name: "primary-1", Host: "db.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRolePrimary, State: replica.TopologyStateActive},
	})
	testutil.NoError(t, err)

	// Add a replica.
	err = store.Add(ctx, replica.TopologyNodeRecord{
		Name: "replica-2", Host: "db-replica-2.example.com", Port: 5432, Database: "mydb",
		Role: replica.TopologyRoleReplica, State: replica.TopologyStateActive,
	})
	testutil.NoError(t, err)

	nodes, err := store.List(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(nodes))
}

func TestReplicaStoreUpdateState(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	err := store.Bootstrap(ctx, []replica.TopologyNodeRecord{
		{Name: "replica-1", Host: "db.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRoleReplica, State: replica.TopologyStateActive},
	})
	testutil.NoError(t, err)

	// Update state to draining.
	err = store.UpdateState(ctx, "replica-1", replica.TopologyStateDraining)
	testutil.NoError(t, err)

	node, err := store.Get(ctx, "replica-1")
	testutil.NoError(t, err)
	testutil.Equal(t, replica.TopologyStateDraining, node.State)

	// Invalid state should error.
	err = store.UpdateState(ctx, "replica-1", "invalid-state")
	testutil.Error(t, err)

	// Nonexistent node should error.
	err = store.UpdateState(ctx, "nonexistent", replica.TopologyStateActive)
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "not found")
}

func TestReplicaStorePromoteNode(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	// Bootstrap primary + replica.
	err := store.Bootstrap(ctx, []replica.TopologyNodeRecord{
		{Name: "primary-1", Host: "primary.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRolePrimary, State: replica.TopologyStateActive},
		{Name: "replica-1", Host: "replica.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRoleReplica, State: replica.TopologyStateActive},
	})
	testutil.NoError(t, err)

	// Promote replica-1 to primary.
	err = store.PromoteNode(ctx, "replica-1")
	testutil.NoError(t, err)

	// replica-1 should now be primary.
	promoted, err := store.Get(ctx, "replica-1")
	testutil.NoError(t, err)
	testutil.Equal(t, replica.TopologyRolePrimary, promoted.Role)

	// primary-1 should now be removed.
	old, err := store.Get(ctx, "primary-1")
	testutil.NoError(t, err)
	testutil.Equal(t, replica.TopologyStateRemoved, old.State)
}

func TestReplicaStorePromoteAlreadyPrimaryFails(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	err := store.Bootstrap(ctx, []replica.TopologyNodeRecord{
		{Name: "primary-1", Host: "primary.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRolePrimary, State: replica.TopologyStateActive},
	})
	testutil.NoError(t, err)

	// Promoting the current primary should fail.
	err = store.PromoteNode(ctx, "primary-1")
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "already primary")
}

func TestReplicaStorePromoteDrainingFails(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := replica.NewPostgresReplicaStore(sharedPG.Pool)

	err := store.Bootstrap(ctx, []replica.TopologyNodeRecord{
		{Name: "primary-1", Host: "primary.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRolePrimary, State: replica.TopologyStateActive},
		{Name: "replica-1", Host: "replica.example.com", Port: 5432, Database: "mydb",
			Role: replica.TopologyRoleReplica, State: replica.TopologyStateDraining},
	})
	testutil.NoError(t, err)

	// Promoting a draining replica should fail.
	err = store.PromoteNode(ctx, "replica-1")
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "not active")
}

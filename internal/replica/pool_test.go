package replica

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig("postgresql://postgres:postgres@127.0.0.1:1/postgres?sslmode=disable&connect_timeout=1")
	testutil.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	testutil.NoError(t, err)
	return pool
}

func newTestPoolOnPort(t *testing.T, port int) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("postgresql://postgres:postgres@127.0.0.1:%d/postgres?sslmode=disable&connect_timeout=1", port))
	testutil.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	testutil.NoError(t, err)
	return pool
}

func TestNewPoolRouterNilOrEmptyReplicas(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	nilRouter := NewPoolRouter(primary, nil, testutil.DiscardLogger())
	testutil.Equal(t, primary, nilRouter.Primary())
	testutil.Equal(t, primary, nilRouter.ReadPool())
	testutil.False(t, nilRouter.HasReplicas())
	testutil.True(t, nilRouter.passThrough)

	emptyRouter := NewPoolRouter(primary, []ReplicaPool{}, testutil.DiscardLogger())
	testutil.Equal(t, primary, emptyRouter.Primary())
	testutil.Equal(t, primary, emptyRouter.ReadPool())
	testutil.False(t, emptyRouter.HasReplicas())
	testutil.True(t, emptyRouter.passThrough)
}

func TestReadPoolEqualWeightsDistribution(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()
	r2 := newTestPool(t)
	defer r2.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: r2, Config: config.ReplicaConfig{URL: "postgresql://replica-2/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	counts := map[*pgxpool.Pool]int{}
	for i := 0; i < 100; i++ {
		counts[router.ReadPool()]++
	}

	testutil.True(t, counts[r1] >= 40 && counts[r1] <= 60)
	testutil.True(t, counts[r2] >= 40 && counts[r2] <= 60)
	testutil.Equal(t, 100, counts[r1]+counts[r2])
}

func TestReadPoolWeightedDistribution(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()
	r2 := newTestPool(t)
	defer r2.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: r2, Config: config.ReplicaConfig{URL: "postgresql://replica-2/db", Weight: 3, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	counts := map[*pgxpool.Pool]int{}
	for i := 0; i < 100; i++ {
		counts[router.ReadPool()]++
	}

	testutil.True(t, counts[r1] >= 20 && counts[r1] <= 30)
	testutil.True(t, counts[r2] >= 70 && counts[r2] <= 80)
	testutil.Equal(t, 100, counts[r1]+counts[r2])
}

func TestReadPoolUsesRemainingReplicaAfterHealthyRemoval(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()
	r2 := newTestPool(t)
	defer r2.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: r2, Config: config.ReplicaConfig{URL: "postgresql://replica-2/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	router.SetHealthy([]*pgxpool.Pool{r2})
	for i := 0; i < 20; i++ {
		testutil.Equal(t, r2, router.ReadPool())
	}
}

func TestReadPoolFallsBackToPrimaryWhenAllReplicasUnhealthy(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	router.SetHealthy(nil)
	testutil.Equal(t, primary, router.ReadPool())
}

func TestReadPoolReAddReplicaAfterHealthyUpdate(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	router.SetHealthy(nil)
	testutil.Equal(t, primary, router.ReadPool())

	router.SetHealthy([]*pgxpool.Pool{r1})
	for i := 0; i < 20; i++ {
		testutil.Equal(t, r1, router.ReadPool())
	}
}

func TestReadPoolSkipsNilReplicaPools(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: nil, Config: config.ReplicaConfig{URL: "postgresql://replica-nil/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	for i := 0; i < 50; i++ {
		testutil.Equal(t, r1, router.ReadPool())
	}
}

func TestRoutingStatsCountPrimaryFallbackInPassThrough(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	router := NewPoolRouter(primary, nil, testutil.DiscardLogger())

	router.ReadPool()
	router.ReadPool()
	router.ReadPool()

	primaryReads, replicaReads := router.RoutingStats()
	if primaryReads != 3 {
		t.Fatalf("primaryReads = %d, want 3", primaryReads)
	}
	if replicaReads != 0 {
		t.Fatalf("replicaReads = %d, want 0", replicaReads)
	}
}

func TestRoutingStatsCountReplicaReadsAndPrimaryFallback(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	router.ReadPool()
	router.ReadPool()

	router.SetHealthy(nil)
	router.ReadPool()

	primaryReads, replicaReads := router.RoutingStats()
	if primaryReads != 1 {
		t.Fatalf("primaryReads = %d, want 1", primaryReads)
	}
	if replicaReads != 2 {
		t.Fatalf("replicaReads = %d, want 2", replicaReads)
	}
}

func TestNewPoolRouterAppliesReplicaDefaults(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db"}},
	}, testutil.DiscardLogger())

	replicas := router.Replicas()
	testutil.SliceLen(t, replicas, 1)
	testutil.Equal(t, config.DefaultReplicaWeight, replicas[0].config.Weight)
	testutil.Equal(t, config.DefaultReplicaMaxLagBytes, replicas[0].config.MaxLagBytes)
}

func TestReplicasReturnsDetachedCopies(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()
	r2 := newTestPool(t)
	defer r2.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: r2, Config: config.ReplicaConfig{URL: "postgresql://replica-2/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	entries := router.Replicas()
	testutil.SliceLen(t, entries, 2)

	// Mutate returned entry and force selection rebuild; this must not alter router behavior.
	entries[0].config.Weight = 100
	router.SetHealthy([]*pgxpool.Pool{r1, r2})

	counts := map[*pgxpool.Pool]int{}
	for i := 0; i < 100; i++ {
		counts[router.ReadPool()]++
	}

	testutil.True(t, counts[r1] >= 40 && counts[r1] <= 60)
	testutil.True(t, counts[r2] >= 40 && counts[r2] <= 60)
	testutil.Equal(t, 100, counts[r1]+counts[r2])
}

func TestCloseIsIdempotent(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	router.Close()
	router.Close()
}

func TestReadPoolConcurrentWithHealthyMutation(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()
	r2 := newTestPool(t)
	defer r2.Close()
	r3 := newTestPool(t)
	defer r3.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: r2, Config: config.ReplicaConfig{URL: "postgresql://replica-2/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: r3, Config: config.ReplicaConfig{URL: "postgresql://replica-3/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					pool := router.ReadPool()
					if pool != primary && pool != r1 && pool != r2 && pool != r3 {
						t.Errorf("unexpected pool returned")
						return
					}
				}
			}
		}()
	}

	for i := 0; i < 500; i++ {
		switch i % 4 {
		case 0:
			router.SetHealthy([]*pgxpool.Pool{r1, r2, r3})
		case 1:
			router.SetHealthy([]*pgxpool.Pool{r2})
		case 2:
			router.SetHealthy([]*pgxpool.Pool{r1, r3})
		default:
			router.SetHealthy(nil)
		}
	}

	close(stop)
	wg.Wait()
}

func TestAcquireRoutesBasedOnReadOnlyFlag(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	r1 := newTestPool(t)
	defer r1.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: r1, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := router.Acquire(ctx, true)
	testutil.Error(t, err)

	_, err = router.Acquire(ctx, false)
	testutil.Error(t, err)
}

func TestAddReplicaEntryStartsServingReads(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	newReplica := newTestPool(t)
	defer newReplica.Close()

	router := NewPoolRouter(primary, nil, testutil.DiscardLogger())
	testutil.Equal(t, primary, router.ReadPool())

	router.AddReplicaEntry(newReplica, "replica-new", config.ReplicaConfig{
		URL:         "postgresql://replica-new/db",
		Weight:      1,
		MaxLagBytes: 10,
	})

	for i := 0; i < 10; i++ {
		testutil.Equal(t, newReplica, router.ReadPool())
	}
}

func TestRemoveReplicaEntryStopsServingReads(t *testing.T) {
	primary := newTestPool(t)
	defer primary.Close()

	replicaOne := newTestPool(t)
	defer replicaOne.Close()
	replicaTwo := newTestPool(t)
	defer replicaTwo.Close()

	router := NewPoolRouter(primary, []ReplicaPool{
		{Pool: replicaOne, Config: config.ReplicaConfig{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: 1}},
		{Pool: replicaTwo, Config: config.ReplicaConfig{URL: "postgresql://replica-2/db", Weight: 1, MaxLagBytes: 1}},
	}, testutil.DiscardLogger())

	router.RemoveReplicaEntry(replicaOne)
	for i := 0; i < 20; i++ {
		testutil.Equal(t, replicaTwo, router.ReadPool())
	}

	router.RemoveReplicaEntry(replicaTwo)
	testutil.Equal(t, primary, router.ReadPool())
}

func TestSwapPrimaryRedirectsPrimaryFallbackAndWrites(t *testing.T) {
	originalPrimary := newTestPoolOnPort(t, 1)
	defer originalPrimary.Close()
	newPrimary := newTestPoolOnPort(t, 2)
	defer newPrimary.Close()

	router := NewPoolRouter(originalPrimary, nil, testutil.DiscardLogger())
	oldPrimary := router.SwapPrimary(newPrimary)
	testutil.Equal(t, originalPrimary, oldPrimary)
	testutil.Equal(t, newPrimary, router.Primary())
	testutil.Equal(t, newPrimary, router.ReadPool())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := router.Acquire(ctx, false)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "127.0.0.1:2")
}

package replica

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStateTransitionHealthyToUnhealthyAfterTwoFailures(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)

	err := errors.New("ping failed")
	checker.applyCheckResult(status, false, 0, err)
	if status.State != HealthSuspect {
		t.Fatalf("state after first failure = %v, want %v", status.State, HealthSuspect)
	}

	checker.applyCheckResult(status, false, 0, err)
	if status.State != HealthUnhealthy {
		t.Fatalf("state after second failure = %v, want %v", status.State, HealthUnhealthy)
	}
}

func TestStateTransitionUnhealthyToHealthyAfterTwoSuccesses(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	status.State = HealthUnhealthy

	checker.applyCheckResult(status, true, 5, nil)
	if status.State != HealthSuspect {
		t.Fatalf("state after first success = %v, want %v", status.State, HealthSuspect)
	}

	checker.applyCheckResult(status, true, 2, nil)
	if status.State != HealthHealthy {
		t.Fatalf("state after second success = %v, want %v", status.State, HealthHealthy)
	}
}

func TestStateTransitionTransientBlipRecoversToHealthy(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)

	checker.applyCheckResult(status, false, 0, errors.New("temporary"))
	if status.State != HealthSuspect {
		t.Fatalf("state after failure = %v, want %v", status.State, HealthSuspect)
	}

	checker.applyCheckResult(status, true, 0, nil)
	if status.State != HealthHealthy {
		t.Fatalf("state after recovery success = %v, want %v", status.State, HealthHealthy)
	}
}

func TestStateTransitionFalseRecoveryFallsBackToUnhealthy(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	status.State = HealthUnhealthy

	checker.applyCheckResult(status, true, 0, nil)
	if status.State != HealthSuspect {
		t.Fatalf("state after first success = %v, want %v", status.State, HealthSuspect)
	}

	checker.applyCheckResult(status, false, 0, errors.New("fail again"))
	if status.State != HealthUnhealthy {
		t.Fatalf("state after false recovery failure = %v, want %v", status.State, HealthUnhealthy)
	}
}

func TestNewHealthCheckerInitializesStatusesAndDefaultInterval(t *testing.T) {
	router, _, _ := newRouterWithReplicas(t, 2)
	checker := NewHealthChecker(router, 0, testLogger())

	if checker.interval != 10*time.Second {
		t.Fatalf("checker interval = %v, want %v", checker.interval, 10*time.Second)
	}
	if len(checker.statuses) != 2 {
		t.Fatalf("len(checker.statuses) = %d, want 2", len(checker.statuses))
	}
	for i, status := range checker.statuses {
		if status.State != HealthHealthy {
			t.Fatalf("status[%d].State = %v, want %v", i, status.State, HealthHealthy)
		}
	}
}

func TestCheckReplicaPingFailureMovesToSuspect(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error {
		return errors.New("ping failure")
	}

	checker.checkReplica(context.Background(), status)

	if status.State != HealthSuspect {
		t.Fatalf("status.State = %v, want %v", status.State, HealthSuspect)
	}
	if status.LastError == nil {
		t.Fatal("status.LastError is nil, want ping error")
	}
}

func TestCheckReplicaLagFailureMovesToSuspect(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error { return nil }
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) {
		return 0, errors.New("lag failure")
	}

	checker.checkReplica(context.Background(), status)

	if status.State != HealthSuspect {
		t.Fatalf("status.State = %v, want %v", status.State, HealthSuspect)
	}
}

func TestCheckLagUsesThresholdAndReplicaMatching(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	status.Config.MaxLagBytes = 10
	status.Config.URL = "postgres://replica.local/db?application_name=replica-a"

	checker.replicationRowsFn = func(context.Context) ([]replicationLagRow, error) {
		return []replicationLagRow{{ApplicationName: "replica-a", ClientAddr: "10.0.0.2", LagBytes: 25}}, nil
	}

	if _, err := checker.checkLag(context.Background(), status); err == nil {
		t.Fatal("checkLag() error = nil, want lag threshold error")
	}

	checker.replicationRowsFn = func(context.Context) ([]replicationLagRow, error) {
		return []replicationLagRow{{ApplicationName: "replica-a", ClientAddr: "10.0.0.2", LagBytes: 5}}, nil
	}

	lag, err := checker.checkLag(context.Background(), status)
	if err != nil {
		t.Fatalf("checkLag() unexpected error: %v", err)
	}
	if lag != 5 {
		t.Fatalf("checkLag() lag = %d, want 5", lag)
	}
}

func TestCheckLagReplicaNotFoundIsFailure(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	status.Config.URL = "postgres://replica.local/db?application_name=replica-a"

	checker.replicationRowsFn = func(context.Context) ([]replicationLagRow, error) {
		return []replicationLagRow{{ApplicationName: "replica-b", ClientAddr: "10.0.0.3", LagBytes: 0}}, nil
	}

	if _, err := checker.checkLag(context.Background(), status); err == nil {
		t.Fatal("checkLag() error = nil, want not-found failure")
	}
}

func TestCheckLagUsesHostToDisambiguateSharedApplicationName(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	status.Config.MaxLagBytes = 10
	status.Config.URL = "postgres://10.0.0.2:5432/db?application_name=shared-replica"

	checker.replicationRowsFn = func(context.Context) ([]replicationLagRow, error) {
		return []replicationLagRow{
			{ApplicationName: "shared-replica", ClientAddr: "10.0.0.3", LagBytes: 1},
			{ApplicationName: "shared-replica", ClientAddr: "10.0.0.2", LagBytes: 25},
		}, nil
	}

	if _, err := checker.checkLag(context.Background(), status); err == nil {
		t.Fatal("checkLag() error = nil, want lag threshold error for host-matched row")
	}
}

func TestCheckLagConflictingSingleFieldMatchesAreAmbiguous(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	status.Config.MaxLagBytes = 10
	status.Config.URL = "postgres://10.0.0.2:5432/db?application_name=replica-a"

	checker.replicationRowsFn = func(context.Context) ([]replicationLagRow, error) {
		return []replicationLagRow{
			{ApplicationName: "replica-a", ClientAddr: "10.0.0.3", LagBytes: 1}, // app-only match
			{ApplicationName: "other", ClientAddr: "10.0.0.2", LagBytes: 25},    // host-only match
		}, nil
	}

	if _, err := checker.checkLag(context.Background(), status); err == nil {
		t.Fatal("checkLag() error = nil, want ambiguity failure for conflicting single-field matches")
	}
}

func TestSanitizeReplicaURLRemovesCredentialsAndSensitiveQueryParams(t *testing.T) {
	got := SanitizeReplicaURL("postgres://reader:secret@replica.local:5432/appdb?application_name=replica-a&password=leak&sslpassword=leak2&user=leaky")
	want := "postgres://replica.local:5432/appdb?application_name=replica-a"
	if got != want {
		t.Fatalf("SanitizeReplicaURL() = %q, want %q", got, want)
	}
}

func TestSanitizeReplicaURLMasksInvalidURLs(t *testing.T) {
	got := SanitizeReplicaURL("postgres://reader:secret@replica.local:5432/%zz")
	if got != "<invalid-replica-url>" {
		t.Fatalf("SanitizeReplicaURL() = %q, want %q", got, "<invalid-replica-url>")
	}
}

func TestStartStopLifecycleRunsChecksAndStopsCleanly(t *testing.T) {
	checker, _, _, _ := newTestCheckerWithOneReplica(t)
	checker.interval = 10 * time.Millisecond
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error { return nil }
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) { return 0, nil }

	ranCh := make(chan struct{}, 1)
	checker.afterCycleHook = func() {
		select {
		case ranCh <- struct{}{}:
		default:
		}
	}

	checker.Start()
	defer checker.Stop()

	select {
	case <-ranCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("health checker did not run a cycle")
	}

	checker.Stop()
}

func TestStopIsIdempotent(t *testing.T) {
	checker, _, _, _ := newTestCheckerWithOneReplica(t)
	checker.interval = 10 * time.Millisecond
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error { return nil }
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) { return 0, nil }

	checker.Start()
	checker.Stop()
	checker.Stop()
}

func TestStartIsIdempotent(t *testing.T) {
	checker, _, _, _ := newTestCheckerWithOneReplica(t)
	checker.interval = time.Hour
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error { return nil }
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) { return 0, nil }

	var cycles int32
	ranCh := make(chan struct{}, 4)
	checker.afterCycleHook = func() {
		atomic.AddInt32(&cycles, 1)
		select {
		case ranCh <- struct{}{}:
		default:
		}
	}

	checker.Start()
	checker.Start()
	defer checker.Stop()

	select {
	case <-ranCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("health checker did not run a cycle")
	}

	time.Sleep(30 * time.Millisecond)
	if got := atomic.LoadInt32(&cycles); got != 1 {
		t.Fatalf("cycle count after duplicate Start() = %d, want 1", got)
	}
}

func TestRunCheckRunsSingleCycle(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error { return nil }
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) { return 3, nil }

	var cycles int32
	checker.afterCycleHook = func() {
		atomic.AddInt32(&cycles, 1)
	}

	checker.RunCheck(context.Background())

	if got := atomic.LoadInt32(&cycles); got != 1 {
		t.Fatalf("cycles = %d, want 1", got)
	}
	if status.State != HealthHealthy {
		t.Fatalf("status.State = %v, want %v", status.State, HealthHealthy)
	}
	if status.LagBytes != 3 {
		t.Fatalf("status.LagBytes = %d, want 3", status.LagBytes)
	}
	if status.LastCheckedAt.IsZero() {
		t.Fatal("status.LastCheckedAt is zero, want populated")
	}
}

func TestStatusesReturnsSnapshotCopy(t *testing.T) {
	checker, status, _, _ := newTestCheckerWithOneReplica(t)
	status.State = HealthSuspect

	snapshot := checker.Statuses()
	if len(snapshot) != 1 {
		t.Fatalf("len(Statuses()) = %d, want 1", len(snapshot))
	}
	snapshot[0].State = HealthUnhealthy

	if checker.statuses[0].State != HealthSuspect {
		t.Fatalf("internal state mutated via snapshot: got %v want %v", checker.statuses[0].State, HealthSuspect)
	}
}

func TestIntegrationReplicaUnhealthyRemovedFromReadPool(t *testing.T) {
	checker, status, router, primary := newTestCheckerWithOneReplica(t)
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error {
		return errors.New("replica down")
	}

	checker.runCheckCycle(context.Background())
	if status.State != HealthSuspect {
		t.Fatalf("state after first cycle = %v, want %v", status.State, HealthSuspect)
	}
	checker.runCheckCycle(context.Background())
	if status.State != HealthUnhealthy {
		t.Fatalf("state after second cycle = %v, want %v", status.State, HealthUnhealthy)
	}

	if got := router.ReadPool(); got != primary {
		t.Fatalf("router.ReadPool() = %p, want primary %p", got, primary)
	}
}

func TestIntegrationReplicaRecoveryAddedBackToReadPool(t *testing.T) {
	checker, status, router, primary := newTestCheckerWithOneReplica(t)
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error {
		return errors.New("replica down")
	}

	checker.runCheckCycle(context.Background())
	checker.runCheckCycle(context.Background())
	if status.State != HealthUnhealthy {
		t.Fatalf("state before recovery = %v, want %v", status.State, HealthUnhealthy)
	}

	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error { return nil }
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) { return 0, nil }

	checker.runCheckCycle(context.Background())
	if status.State != HealthSuspect {
		t.Fatalf("state after first recovery cycle = %v, want %v", status.State, HealthSuspect)
	}
	if got := router.ReadPool(); got != primary {
		t.Fatalf("router.ReadPool() during suspect recovery = %p, want primary %p", got, primary)
	}

	checker.runCheckCycle(context.Background())
	if status.State != HealthHealthy {
		t.Fatalf("state after second recovery cycle = %v, want %v", status.State, HealthHealthy)
	}
	if got := router.ReadPool(); got != status.Pool {
		t.Fatalf("router.ReadPool() after recovery = %p, want replica %p", got, status.Pool)
	}
}

func TestIntegrationAllReplicasUnhealthyFallsBackToPrimary(t *testing.T) {
	router, primary, replicas := newRouterWithReplicas(t, 2)
	checker := NewHealthChecker(router, time.Second, testLogger())
	checker.pingReplicaFn = func(context.Context, *ReplicaStatus) error {
		return errors.New("down")
	}

	checker.runCheckCycle(context.Background())
	checker.runCheckCycle(context.Background())

	for i, status := range checker.statuses {
		if status.State != HealthUnhealthy {
			t.Fatalf("status[%d].State = %v, want %v", i, status.State, HealthUnhealthy)
		}
	}

	if got := router.ReadPool(); got != primary {
		t.Fatalf("router.ReadPool() = %p, want primary %p", got, primary)
	}

	_ = replicas
}

func TestAddStatusAndRemoveStatusUpdateStatusesSnapshot(t *testing.T) {
	checker, _, _, _ := newTestCheckerWithOneReplica(t)

	var additionalPool pgxpool.Pool
	checker.AddStatus(&additionalPool, "replica-added", ReplicaConfig{
		URL:         "postgres://replica-added.local:5432/appdb?application_name=replica-added",
		Weight:      2,
		MaxLagBytes: 20,
	})

	statuses := checker.Statuses()
	if len(statuses) != 2 {
		t.Fatalf("len(Statuses()) = %d, want 2 after add", len(statuses))
	}

	foundAdded := false
	for _, status := range statuses {
		if status.Pool == &additionalPool {
			foundAdded = true
			if status.Name != "replica-added" {
				t.Fatalf("added status name = %q, want %q", status.Name, "replica-added")
			}
			if status.State != HealthHealthy {
				t.Fatalf("added status state = %v, want %v", status.State, HealthHealthy)
			}
		}
	}
	if !foundAdded {
		t.Fatal("added replica status not found in snapshot")
	}

	checker.RemoveStatus(&additionalPool)
	statuses = checker.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("len(Statuses()) = %d, want 1 after remove", len(statuses))
	}
	if statuses[0].Pool == &additionalPool {
		t.Fatal("removed pool still present in statuses snapshot")
	}
}

func TestRemoveStatusExcludedFromNextCheckCycle(t *testing.T) {
	checker, firstStatus, _, _ := newTestCheckerWithOneReplica(t)

	var removedPool pgxpool.Pool
	checker.AddStatus(&removedPool, "replica-removed", ReplicaConfig{
		URL:         "postgres://replica-removed.local:5432/appdb?application_name=replica-removed",
		Weight:      1,
		MaxLagBytes: 10,
	})

	pingCounts := map[*pgxpool.Pool]int{}
	checker.pingReplicaFn = func(_ context.Context, status *ReplicaStatus) error {
		pingCounts[status.Pool]++
		return nil
	}
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) { return 0, nil }

	checker.RemoveStatus(&removedPool)
	checker.runCheckCycle(context.Background())

	if pingCounts[&removedPool] != 0 {
		t.Fatalf("removed pool ping count = %d, want 0", pingCounts[&removedPool])
	}
	if pingCounts[firstStatus.Pool] == 0 {
		t.Fatal("existing pool was not checked")
	}
}

func TestRunCheckCycleUsesSnapshotDuringConcurrentRemove(t *testing.T) {
	router, _, _ := newRouterWithReplicas(t, 2)
	checker := NewHealthChecker(router, time.Second, testLogger())
	if len(checker.statuses) != 2 {
		t.Fatalf("len(checker.statuses) = %d, want 2", len(checker.statuses))
	}

	firstPool := checker.statuses[0].Pool
	secondPool := checker.statuses[1].Pool
	pingCounts := map[*pgxpool.Pool]int{}

	checker.pingReplicaFn = func(_ context.Context, status *ReplicaStatus) error {
		pingCounts[status.Pool]++
		if status.Pool == firstPool {
			checker.RemoveStatus(secondPool)
		}
		return nil
	}
	checker.lagCheckFn = func(context.Context, *ReplicaStatus) (int64, error) { return 0, nil }

	checker.runCheckCycle(context.Background())

	if pingCounts[firstPool] != 1 {
		t.Fatalf("first pool ping count = %d, want 1", pingCounts[firstPool])
	}
	if pingCounts[secondPool] != 1 {
		t.Fatalf("second pool ping count = %d, want 1", pingCounts[secondPool])
	}
	if len(checker.Statuses()) != 1 {
		t.Fatalf("len(Statuses()) after remove = %d, want 1", len(checker.Statuses()))
	}
}

func newTestCheckerWithOneReplica(t *testing.T) (*HealthChecker, *ReplicaStatus, *PoolRouter, *pgxpool.Pool) {
	t.Helper()
	router, primary, _ := newRouterWithReplicas(t, 1)
	checker := NewHealthChecker(router, time.Second, testLogger())
	if len(checker.statuses) != 1 {
		t.Fatalf("len(checker.statuses) = %d, want 1", len(checker.statuses))
	}
	return checker, checker.statuses[0], router, primary
}

func newRouterWithReplicas(t *testing.T, replicaCount int) (*PoolRouter, *pgxpool.Pool, []*pgxpool.Pool) {
	t.Helper()

	var primaryPool pgxpool.Pool
	replicaPoolValues := make([]pgxpool.Pool, replicaCount)
	replicas := make([]ReplicaPool, 0, replicaCount)
	replicaPtrs := make([]*pgxpool.Pool, 0, replicaCount)

	for i := 0; i < replicaCount; i++ {
		poolPtr := &replicaPoolValues[i]
		replicaPtrs = append(replicaPtrs, poolPtr)
		replicas = append(replicas, ReplicaPool{
			Pool: poolPtr,
			Config: ReplicaConfig{
				URL:         fmt.Sprintf("postgres://replica-%d.local:5432/appdb?application_name=replica-%d", i, i),
				Weight:      1,
				MaxLagBytes: 10,
			},
		})
	}

	router := NewPoolRouter(&primaryPool, replicas, testLogger())
	return router, &primaryPool, replicaPtrs
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

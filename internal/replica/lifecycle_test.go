package replica

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lifecycleStoreMock struct {
	records      map[string]TopologyNodeRecord
	addErr       error
	updateErr    error
	promoteErr   error
	addCalls     []TopologyNodeRecord
	updateCalls  []lifecycleStateUpdate
	promoteCalls []string
}

type lifecycleStateUpdate struct {
	name  string
	state string
}

func newLifecycleStoreMock(records ...TopologyNodeRecord) *lifecycleStoreMock {
	recordMap := make(map[string]TopologyNodeRecord, len(records))
	for _, record := range records {
		recordMap[record.Name] = record
	}
	return &lifecycleStoreMock{records: recordMap}
}

func (m *lifecycleStoreMock) List(context.Context) ([]TopologyNodeRecord, error) {
	out := make([]TopologyNodeRecord, 0, len(m.records))
	for _, record := range m.records {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *lifecycleStoreMock) Get(_ context.Context, name string) (TopologyNodeRecord, error) {
	record, ok := m.records[name]
	if !ok {
		return TopologyNodeRecord{}, ErrTopologyNodeNotFound
	}
	return record, nil
}

func (m *lifecycleStoreMock) IsEmpty(context.Context) (bool, error) {
	return len(m.records) == 0, nil
}

func (m *lifecycleStoreMock) Bootstrap(context.Context, []TopologyNodeRecord) error {
	return nil
}

func (m *lifecycleStoreMock) UpdateState(_ context.Context, name, state string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	record, ok := m.records[name]
	if !ok {
		return ErrTopologyNodeNotFound
	}
	record.State = state
	m.records[name] = record
	m.updateCalls = append(m.updateCalls, lifecycleStateUpdate{name: name, state: state})
	return nil
}

func (m *lifecycleStoreMock) Add(_ context.Context, record TopologyNodeRecord) error {
	if m.addErr != nil {
		return m.addErr
	}
	m.records[record.Name] = record
	m.addCalls = append(m.addCalls, record)
	return nil
}

func (m *lifecycleStoreMock) PromoteNode(_ context.Context, targetName string) error {
	if m.promoteErr != nil {
		return m.promoteErr
	}
	target, ok := m.records[targetName]
	if !ok {
		return fmt.Errorf("target not found")
	}
	for name, record := range m.records {
		if record.Role == TopologyRolePrimary && record.State != TopologyStateRemoved {
			record.State = TopologyStateRemoved
			m.records[name] = record
		}
	}
	target.Role = TopologyRolePrimary
	m.records[targetName] = target
	m.promoteCalls = append(m.promoteCalls, targetName)
	return nil
}

type lifecycleRouterMock struct {
	primary      *pgxpool.Pool
	addCalls     []lifecycleRouterAddCall
	removeCalls  []*pgxpool.Pool
	swapCalls    []*pgxpool.Pool
	lastSwapFrom *pgxpool.Pool
}

type lifecycleRouterAddCall struct {
	name string
	pool *pgxpool.Pool
	cfg  ReplicaConfig
}

func (m *lifecycleRouterMock) AddReplicaEntry(pool *pgxpool.Pool, name string, cfg ReplicaConfig) {
	m.addCalls = append(m.addCalls, lifecycleRouterAddCall{name: name, pool: pool, cfg: cfg})
}

func (m *lifecycleRouterMock) RemoveReplicaEntry(pool *pgxpool.Pool) {
	m.removeCalls = append(m.removeCalls, pool)
}

func (m *lifecycleRouterMock) SwapPrimary(newPrimary *pgxpool.Pool) *pgxpool.Pool {
	old := m.primary
	m.lastSwapFrom = old
	m.primary = newPrimary
	m.swapCalls = append(m.swapCalls, newPrimary)
	return old
}

func (m *lifecycleRouterMock) Primary() *pgxpool.Pool {
	return m.primary
}

type lifecycleCheckerMock struct {
	statuses    []ReplicaStatus
	addCalls    []lifecycleCheckerAddCall
	removeCalls []*pgxpool.Pool
}

type lifecycleCheckerAddCall struct {
	name string
	pool *pgxpool.Pool
	cfg  ReplicaConfig
}

func (m *lifecycleCheckerMock) AddStatus(pool *pgxpool.Pool, name string, cfg ReplicaConfig) {
	m.statuses = append(m.statuses, ReplicaStatus{Name: name, Pool: pool, Config: cfg, State: HealthHealthy})
	m.addCalls = append(m.addCalls, lifecycleCheckerAddCall{name: name, pool: pool, cfg: cfg})
}

func (m *lifecycleCheckerMock) RemoveStatus(pool *pgxpool.Pool) {
	filtered := make([]ReplicaStatus, 0, len(m.statuses))
	for _, status := range m.statuses {
		if status.Pool == pool {
			continue
		}
		filtered = append(filtered, status)
	}
	m.statuses = filtered
	m.removeCalls = append(m.removeCalls, pool)
}

func (m *lifecycleCheckerMock) Statuses() []ReplicaStatus {
	out := make([]ReplicaStatus, len(m.statuses))
	copy(out, m.statuses)
	return out
}

type lifecycleAuditFailingExecer struct{}

func (lifecycleAuditFailingExecer) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("forced audit failure")
}

type lifecycleRow struct {
	values []any
	err    error
}

func (r lifecycleRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan destination/value mismatch: %d != %d", len(dest), len(r.values))
	}
	for i := range dest {
		switch ptr := dest[i].(type) {
		case *int:
			value, ok := r.values[i].(int)
			if !ok {
				return fmt.Errorf("value %d is not int", i)
			}
			*ptr = value
		case *bool:
			value, ok := r.values[i].(bool)
			if !ok {
				return fmt.Errorf("value %d is not bool", i)
			}
			*ptr = value
		default:
			return fmt.Errorf("unsupported scan destination: %T", dest[i])
		}
	}
	return nil
}

func newLifecycleTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newLifecycleServiceForTest(store ReplicaStore, router lifecycleRouter, checker lifecycleChecker, initialPools map[string]*pgxpool.Pool) *LifecycleService {
	return NewLifecycleService(
		store,
		router,
		checker,
		audit.NewAuditLogger(config.AuditConfig{Enabled: false}, nil),
		newLifecycleTestLogger(),
		initialPools,
	)
}

func promotableReplicaRecord(name string) TopologyNodeRecord {
	return TopologyNodeRecord{
		Name:        name,
		Host:        name + ".internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        TopologyRoleReplica,
		State:       TopologyStateActive,
		Weight:      1,
		MaxLagBytes: 1024,
	}
}
func newPromotionQueryRow(controlPool *pgxpool.Pool, recoveryResponses []bool) func(context.Context, *pgxpool.Pool, string) pgx.Row {
	return func(_ context.Context, pool *pgxpool.Pool, sql string) pgx.Row {
		if pool != controlPool {
			return lifecycleRow{err: errors.New("unexpected pool")}
		}
		switch sql {
		case "SELECT pg_promote()":
			return lifecycleRow{values: []any{true}}
		case "SELECT pg_is_in_recovery()":
			value := recoveryResponses[0]
			if len(recoveryResponses) > 1 {
				recoveryResponses = recoveryResponses[1:]
			}
			return lifecycleRow{values: []any{value}}
		default:
			return lifecycleRow{err: fmt.Errorf("unexpected sql: %s", sql)}
		}
	}
}

func TestLifecycleServiceAddReplicaHappyPath(t *testing.T) {
	store := newLifecycleStoreMock()
	primaryPool := newTestPool(t)
	t.Cleanup(primaryPool.Close)
	tempPool := newTestPool(t)
	permPool := newTestPool(t)
	t.Cleanup(func() {
		tempPool.Close()
		permPool.Close()
	})

	router := &lifecycleRouterMock{primary: primaryPool}
	checker := &lifecycleCheckerMock{}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"primary": primaryPool,
	})

	dialCalls := 0
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) {
		dialCalls++
		if dialCalls == 1 {
			return tempPool, nil
		}
		return permPool, nil
	}
	service.queryRow = func(_ context.Context, _ *pgxpool.Pool, sql string) pgx.Row {
		switch sql {
		case "SELECT 1":
			return lifecycleRow{values: []any{1}}
		case "SELECT pg_is_in_recovery()":
			return lifecycleRow{values: []any{true}}
		default:
			return lifecycleRow{err: fmt.Errorf("unexpected sql: %s", sql)}
		}
	}

	createdRecord, err := service.AddReplica(context.Background(), TopologyNodeRecord{
		Name:        "replica-new",
		Host:        "replica-new.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        TopologyRoleReplica,
		State:       TopologyStateActive,
		Weight:      3,
		MaxLagBytes: 8192,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "replica-new", createdRecord.Name)
	testutil.Equal(t, TopologyRoleReplica, createdRecord.Role)
	testutil.Equal(t, TopologyStateActive, createdRecord.State)
	testutil.Equal(t, 2, dialCalls)
	testutil.SliceLen(t, store.addCalls, 1)
	testutil.Equal(t, TopologyRoleReplica, store.addCalls[0].Role)
	testutil.Equal(t, TopologyStateActive, store.addCalls[0].State)
	testutil.SliceLen(t, router.addCalls, 1)
	testutil.Equal(t, "replica-new", router.addCalls[0].name)
	testutil.Equal(t, permPool, router.addCalls[0].pool)
	testutil.SliceLen(t, checker.addCalls, 1)
	testutil.Equal(t, "replica-new", checker.addCalls[0].name)
	testutil.Equal(t, permPool, checker.addCalls[0].pool)

	service.poolsMu.RLock()
	addedPool := service.pools["replica-new"]
	service.poolsMu.RUnlock()
	testutil.Equal(t, permPool, addedPool)
}

func TestLifecycleServiceAddReplicaRejectsDuplicateName(t *testing.T) {
	store := newLifecycleStoreMock(TopologyNodeRecord{
		Name:     "replica-existing",
		Host:     "replica-existing.internal",
		Port:     5432,
		Database: "appdb",
		Role:     TopologyRoleReplica,
		State:    TopologyStateActive,
	})
	router := &lifecycleRouterMock{primary: newTestPool(t)}
	t.Cleanup(router.primary.Close)
	checker := &lifecycleCheckerMock{}
	service := newLifecycleServiceForTest(store, router, checker, nil)

	_, err := service.AddReplica(context.Background(), TopologyNodeRecord{
		Name:     "replica-existing",
		Host:     "replica-existing.internal",
		Port:     5432,
		Database: "appdb",
		Role:     TopologyRoleReplica,
		State:    TopologyStateActive,
	})
	testutil.Error(t, err)
	testutil.Contains(t, strings.ToLower(err.Error()), "already")
	testutil.SliceLen(t, store.addCalls, 0)
	testutil.SliceLen(t, router.addCalls, 0)
}

func TestLifecycleServiceAddReplicaRejectsNonReplicaTarget(t *testing.T) {
	store := newLifecycleStoreMock()
	router := &lifecycleRouterMock{primary: newTestPool(t)}
	t.Cleanup(router.primary.Close)
	checker := &lifecycleCheckerMock{}
	tempPool := newTestPool(t)
	t.Cleanup(tempPool.Close)

	service := newLifecycleServiceForTest(store, router, checker, nil)
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) { return tempPool, nil }
	service.queryRow = func(_ context.Context, _ *pgxpool.Pool, sql string) pgx.Row {
		switch sql {
		case "SELECT 1":
			return lifecycleRow{values: []any{1}}
		case "SELECT pg_is_in_recovery()":
			return lifecycleRow{values: []any{false}}
		default:
			return lifecycleRow{err: fmt.Errorf("unexpected sql: %s", sql)}
		}
	}

	_, err := service.AddReplica(context.Background(), TopologyNodeRecord{
		Name:     "replica-not-standby",
		Host:     "replica-not-standby.internal",
		Port:     5432,
		Database: "appdb",
		Role:     TopologyRoleReplica,
		State:    TopologyStateActive,
	})
	testutil.Error(t, err)
	testutil.Contains(t, strings.ToLower(err.Error()), "standby")
	testutil.SliceLen(t, store.addCalls, 0)
}

func TestLifecycleServiceAddReplicaCompensatesAfterRuntimeFailure(t *testing.T) {
	store := newLifecycleStoreMock()
	primaryPool := newTestPool(t)
	tempPool := newTestPool(t)
	permPool := newTestPool(t)
	t.Cleanup(func() {
		primaryPool.Close()
		tempPool.Close()
		permPool.Close()
	})

	router := &lifecycleRouterMock{primary: primaryPool}
	checker := &lifecycleCheckerMock{}
	service := NewLifecycleService(
		store,
		router,
		checker,
		audit.NewAuditLogger(config.AuditConfig{Enabled: true, AllTables: true}, lifecycleAuditFailingExecer{}),
		newLifecycleTestLogger(),
		map[string]*pgxpool.Pool{"primary": primaryPool},
	)

	dialCalls := 0
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) {
		dialCalls++
		if dialCalls == 1 {
			return tempPool, nil
		}
		return permPool, nil
	}
	service.queryRow = func(_ context.Context, _ *pgxpool.Pool, sql string) pgx.Row {
		switch sql {
		case "SELECT 1":
			return lifecycleRow{values: []any{1}}
		case "SELECT pg_is_in_recovery()":
			return lifecycleRow{values: []any{true}}
		default:
			return lifecycleRow{err: fmt.Errorf("unexpected sql: %s", sql)}
		}
	}

	_, err := service.AddReplica(context.Background(), TopologyNodeRecord{
		Name:     "replica-failing-audit",
		Host:     "replica-failing-audit.internal",
		Port:     5432,
		Database: "appdb",
		Role:     TopologyRoleReplica,
		State:    TopologyStateActive,
	})
	testutil.Error(t, err)
	testutil.SliceLen(t, store.addCalls, 1)
	testutil.True(t, len(store.updateCalls) >= 1)
	testutil.SliceLen(t, router.removeCalls, 1)
	testutil.Equal(t, permPool, router.removeCalls[0])
	testutil.SliceLen(t, checker.removeCalls, 1)
	testutil.Equal(t, permPool, checker.removeCalls[0])

	service.poolsMu.RLock()
	_, exists := service.pools["replica-failing-audit"]
	service.poolsMu.RUnlock()
	testutil.False(t, exists)
}

func TestLifecycleServiceRemoveReplicaHappyPath(t *testing.T) {
	replicaA := newTestPool(t)
	replicaB := newTestPool(t)
	t.Cleanup(func() {
		replicaA.Close()
		replicaB.Close()
	})

	store := newLifecycleStoreMock(
		TopologyNodeRecord{Name: "replica-a", Role: TopologyRoleReplica, State: TopologyStateActive},
		TopologyNodeRecord{Name: "replica-b", Role: TopologyRoleReplica, State: TopologyStateActive},
	)
	router := &lifecycleRouterMock{}
	checker := &lifecycleCheckerMock{}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"replica-a": replicaA,
		"replica-b": replicaB,
	})

	err := service.RemoveReplica(context.Background(), "replica-a", false)
	testutil.NoError(t, err)
	testutil.SliceLen(t, store.updateCalls, 2)
	testutil.Equal(t, TopologyStateDraining, store.updateCalls[0].state)
	testutil.Equal(t, TopologyStateRemoved, store.updateCalls[1].state)
	testutil.SliceLen(t, router.removeCalls, 1)
	testutil.Equal(t, replicaA, router.removeCalls[0])
	testutil.SliceLen(t, checker.removeCalls, 1)
	testutil.Equal(t, replicaA, checker.removeCalls[0])

	service.poolsMu.RLock()
	_, exists := service.pools["replica-a"]
	service.poolsMu.RUnlock()
	testutil.False(t, exists)
}

func TestLifecycleServiceRemoveReplicaRejectsLastActiveReplicaUnlessForced(t *testing.T) {
	replicaA := newTestPool(t)
	t.Cleanup(replicaA.Close)

	store := newLifecycleStoreMock(
		TopologyNodeRecord{Name: "replica-a", Role: TopologyRoleReplica, State: TopologyStateActive},
		TopologyNodeRecord{Name: "replica-removed", Role: TopologyRoleReplica, State: TopologyStateRemoved},
	)
	router := &lifecycleRouterMock{}
	checker := &lifecycleCheckerMock{}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{"replica-a": replicaA})

	err := service.RemoveReplica(context.Background(), "replica-a", false)
	testutil.Error(t, err)
	testutil.Contains(t, strings.ToLower(err.Error()), "last")
	testutil.SliceLen(t, store.updateCalls, 0)

	err = service.RemoveReplica(context.Background(), "replica-a", true)
	testutil.NoError(t, err)
}

func TestLifecycleServiceRemoveReplicaRejectsPrimaryRemoval(t *testing.T) {
	store := newLifecycleStoreMock(TopologyNodeRecord{
		Name:  "primary",
		Role:  TopologyRolePrimary,
		State: TopologyStateActive,
	})
	router := &lifecycleRouterMock{}
	checker := &lifecycleCheckerMock{}
	service := newLifecycleServiceForTest(store, router, checker, nil)

	err := service.RemoveReplica(context.Background(), "primary", false)
	testutil.Error(t, err)
	testutil.Contains(t, strings.ToLower(err.Error()), "replica")
}

func TestLifecycleServicePromoteReplicaHappyPath(t *testing.T) {
	primaryPool := newTestPool(t)
	targetReplicaPool := newTestPool(t)
	promotionControlPool := newTestPool(t)
	newPrimaryPool := newTestPool(t)
	t.Cleanup(func() {
		primaryPool.Close()
		targetReplicaPool.Close()
		promotionControlPool.Close()
		newPrimaryPool.Close()
	})

	store := newLifecycleStoreMock(promotableReplicaRecord("replica-a"))
	router := &lifecycleRouterMock{primary: primaryPool}
	checker := &lifecycleCheckerMock{
		statuses: []ReplicaStatus{
			{Pool: targetReplicaPool, State: HealthHealthy, LagBytes: 12},
		},
	}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"replica-a": targetReplicaPool,
	})

	dialCalls := 0
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) {
		dialCalls++
		if dialCalls == 1 {
			return promotionControlPool, nil
		}
		return newPrimaryPool, nil
	}
	service.queryRow = newPromotionQueryRow(promotionControlPool, []bool{true, false})

	promotedRecord, err := service.PromoteReplica(context.Background(), "replica-a")
	testutil.NoError(t, err)
	testutil.Equal(t, "replica-a", promotedRecord.Name)
	testutil.Equal(t, TopologyRolePrimary, promotedRecord.Role)
	testutil.SliceLen(t, store.promoteCalls, 1)
	testutil.Equal(t, "replica-a", store.promoteCalls[0])
	testutil.SliceLen(t, router.swapCalls, 1)
	testutil.Equal(t, newPrimaryPool, router.swapCalls[0])
	testutil.SliceLen(t, router.removeCalls, 1)
	testutil.Equal(t, targetReplicaPool, router.removeCalls[0])
	testutil.SliceLen(t, checker.removeCalls, 1)
	testutil.Equal(t, targetReplicaPool, checker.removeCalls[0])

	service.poolsMu.RLock()
	updated := service.pools["replica-a"]
	service.poolsMu.RUnlock()
	testutil.Equal(t, newPrimaryPool, updated)
}

func TestLifecycleServicePromoteReplicaRejectsUnhealthyTarget(t *testing.T) {
	replicaPool := newTestPool(t)
	t.Cleanup(replicaPool.Close)

	store := newLifecycleStoreMock(TopologyNodeRecord{
		Name:  "replica-a",
		Role:  TopologyRoleReplica,
		State: TopologyStateActive,
	})
	router := &lifecycleRouterMock{primary: newTestPool(t)}
	t.Cleanup(router.primary.Close)
	checker := &lifecycleCheckerMock{
		statuses: []ReplicaStatus{
			{Pool: replicaPool, State: HealthUnhealthy},
		},
	}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"replica-a": replicaPool,
	})

	_, err := service.PromoteReplica(context.Background(), "replica-a")
	testutil.Error(t, err)
	testutil.Contains(t, strings.ToLower(err.Error()), "healthy")
	testutil.SliceLen(t, store.promoteCalls, 0)
}

func TestLifecycleServicePromoteReplicaFallsBackToRuntimePoolWhenReplacementDialFails(t *testing.T) {
	primaryPool := newTestPool(t)
	targetReplicaPool := newTestPool(t)
	promotionControlPool := newTestPool(t)
	t.Cleanup(func() {
		primaryPool.Close()
		targetReplicaPool.Close()
		promotionControlPool.Close()
	})

	store := newLifecycleStoreMock(promotableReplicaRecord("replica-a"))
	router := &lifecycleRouterMock{primary: primaryPool}
	checker := &lifecycleCheckerMock{
		statuses: []ReplicaStatus{
			{Pool: targetReplicaPool, State: HealthHealthy, LagBytes: 12},
		},
	}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"replica-a": targetReplicaPool,
	})

	dialCalls := 0
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) {
		dialCalls++
		if dialCalls == 1 {
			return promotionControlPool, nil
		}
		return nil, errors.New("replacement primary dial failed")
	}
	service.queryRow = newPromotionQueryRow(promotionControlPool, []bool{true, false})

	promotedRecord, err := service.PromoteReplica(context.Background(), "replica-a")
	testutil.NoError(t, err)
	testutil.Equal(t, TopologyRolePrimary, promotedRecord.Role)
	testutil.Equal(t, 2, dialCalls)
	testutil.SliceLen(t, store.promoteCalls, 1)
	testutil.Equal(t, targetReplicaPool, router.Primary())
	testutil.SliceLen(t, router.swapCalls, 1)
	testutil.Equal(t, targetReplicaPool, router.swapCalls[0])
	testutil.SliceLen(t, router.removeCalls, 1)
	testutil.Equal(t, targetReplicaPool, router.removeCalls[0])
	testutil.SliceLen(t, checker.removeCalls, 1)
	testutil.Equal(t, targetReplicaPool, checker.removeCalls[0])

	service.poolsMu.RLock()
	updated := service.pools["replica-a"]
	service.poolsMu.RUnlock()
	testutil.Equal(t, targetReplicaPool, updated)
}

func TestLifecycleServicePromoteReplicaKeepsRuntimeAlignedWhenStoreSyncFails(t *testing.T) {
	primaryPool := newTestPool(t)
	targetReplicaPool := newTestPool(t)
	promotionControlPool := newTestPool(t)
	newPrimaryPool := newTestPool(t)
	t.Cleanup(func() {
		primaryPool.Close()
		targetReplicaPool.Close()
		promotionControlPool.Close()
		newPrimaryPool.Close()
	})

	store := newLifecycleStoreMock(promotableReplicaRecord("replica-a"))
	store.promoteErr = errors.New("store unavailable")
	router := &lifecycleRouterMock{primary: primaryPool}
	checker := &lifecycleCheckerMock{
		statuses: []ReplicaStatus{
			{Pool: targetReplicaPool, State: HealthHealthy, LagBytes: 12},
		},
	}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"replica-a": targetReplicaPool,
	})

	dialCalls := 0
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) {
		dialCalls++
		if dialCalls == 1 {
			return promotionControlPool, nil
		}
		return newPrimaryPool, nil
	}
	service.queryRow = newPromotionQueryRow(promotionControlPool, []bool{true, false})

	_, err := service.PromoteReplica(context.Background(), "replica-a")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "persist promotion")
	testutil.Equal(t, 2, dialCalls)
	testutil.Equal(t, newPrimaryPool, router.Primary())
	testutil.SliceLen(t, router.swapCalls, 1)
	testutil.Equal(t, newPrimaryPool, router.swapCalls[0])
	testutil.SliceLen(t, router.removeCalls, 1)
	testutil.Equal(t, targetReplicaPool, router.removeCalls[0])
	testutil.SliceLen(t, checker.removeCalls, 1)
	testutil.Equal(t, targetReplicaPool, checker.removeCalls[0])

	service.poolsMu.RLock()
	updated := service.pools["replica-a"]
	service.poolsMu.RUnlock()
	testutil.Equal(t, newPrimaryPool, updated)
}

func TestLifecycleServiceInitiateFailoverAutoSelectsLowestLagReplica(t *testing.T) {
	primaryPool := newTestPool(t)
	replicaA := newTestPool(t)
	replicaB := newTestPool(t)
	promotionControlPool := newTestPool(t)
	newPrimaryPool := newTestPool(t)
	t.Cleanup(func() {
		primaryPool.Close()
		replicaA.Close()
		replicaB.Close()
		promotionControlPool.Close()
		newPrimaryPool.Close()
	})

	store := newLifecycleStoreMock(promotableReplicaRecord("replica-b"))
	router := &lifecycleRouterMock{primary: primaryPool}
	checker := &lifecycleCheckerMock{
		statuses: []ReplicaStatus{
			{Pool: replicaA, State: HealthHealthy, LagBytes: 200},
			{Pool: replicaB, State: HealthHealthy, LagBytes: 25},
		},
	}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"replica-a": replicaA,
		"replica-b": replicaB,
	})

	dialCalls := 0
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) {
		dialCalls++
		if dialCalls == 1 {
			return promotionControlPool, nil
		}
		return newPrimaryPool, nil
	}
	promotionQueryRow := newPromotionQueryRow(promotionControlPool, []bool{true, false})
	service.queryRow = func(_ context.Context, pool *pgxpool.Pool, sql string) pgx.Row {
		if pool == primaryPool && sql == "SELECT 1" {
			return lifecycleRow{err: errors.New("primary unreachable")}
		}
		return promotionQueryRow(context.Background(), pool, sql)
	}

	err := service.InitiateFailover(context.Background(), "", false)
	testutil.NoError(t, err)
	testutil.SliceLen(t, store.promoteCalls, 1)
	testutil.Equal(t, "replica-b", store.promoteCalls[0])
}

func TestLifecycleServiceInitiateFailoverRejectsWhenPrimaryHealthy(t *testing.T) {
	primaryPool := newTestPool(t)
	replicaPool := newTestPool(t)
	t.Cleanup(func() {
		primaryPool.Close()
		replicaPool.Close()
	})

	store := newLifecycleStoreMock(TopologyNodeRecord{
		Name:  "replica-a",
		Role:  TopologyRoleReplica,
		State: TopologyStateActive,
	})
	router := &lifecycleRouterMock{primary: primaryPool}
	checker := &lifecycleCheckerMock{
		statuses: []ReplicaStatus{{Pool: replicaPool, State: HealthHealthy, LagBytes: 5}},
	}
	service := newLifecycleServiceForTest(store, router, checker, map[string]*pgxpool.Pool{
		"replica-a": replicaPool,
	})
	service.queryRow = func(_ context.Context, _ *pgxpool.Pool, sql string) pgx.Row {
		if sql == "SELECT 1" {
			return lifecycleRow{values: []any{1}}
		}
		return lifecycleRow{err: fmt.Errorf("unexpected sql: %s", sql)}
	}

	err := service.InitiateFailover(context.Background(), "replica-a", false)
	testutil.Error(t, err)
	testutil.Contains(t, strings.ToLower(err.Error()), "healthy")
	testutil.SliceLen(t, store.promoteCalls, 0)

	service.queryRow = func(_ context.Context, pool *pgxpool.Pool, sql string) pgx.Row {
		if pool == primaryPool && sql == "SELECT 1" {
			return lifecycleRow{values: []any{1}}
		}
		if sql == "SELECT pg_promote()" {
			return lifecycleRow{values: []any{true}}
		}
		if sql == "SELECT pg_is_in_recovery()" {
			return lifecycleRow{values: []any{false}}
		}
		return lifecycleRow{err: fmt.Errorf("unexpected sql: %s", sql)}
	}
	service.dialPool = func(context.Context, string) (*pgxpool.Pool, error) { return newTestPool(t), nil }

	err = service.InitiateFailover(context.Background(), "replica-a", true)
	testutil.NoError(t, err)
}

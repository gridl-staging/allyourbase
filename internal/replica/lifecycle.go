// Package replica Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/replica/lifecycle.go.
package replica

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	replicaPromotionTimeout  = 30 * time.Second
	replicaPrimaryProbeDelay = 2 * time.Second
	replicaPromotionPollWait = 100 * time.Millisecond
)

type lifecycleRouter interface {
	AddReplicaEntry(pool *pgxpool.Pool, name string, cfg ReplicaConfig)
	RemoveReplicaEntry(pool *pgxpool.Pool)
	SwapPrimary(newPrimary *pgxpool.Pool) *pgxpool.Pool
	Primary() *pgxpool.Pool
}

type lifecycleChecker interface {
	AddStatus(pool *pgxpool.Pool, name string, cfg ReplicaConfig)
	RemoveStatus(pool *pgxpool.Pool)
	Statuses() []ReplicaStatus
}

type LifecycleService struct {
	store   ReplicaStore
	router  lifecycleRouter
	checker lifecycleChecker
	audit   *audit.AuditLogger
	logger  *slog.Logger

	poolsMu sync.RWMutex
	pools   map[string]*pgxpool.Pool

	dialPool func(ctx context.Context, dsn string) (*pgxpool.Pool, error)
	queryRow func(ctx context.Context, pool *pgxpool.Pool, sql string) pgx.Row
}

// TODO: Document NewLifecycleService.
func NewLifecycleService(store ReplicaStore, router lifecycleRouter, checker lifecycleChecker, auditLogger *audit.AuditLogger, logger *slog.Logger, initialPools map[string]*pgxpool.Pool) *LifecycleService {
	if logger == nil {
		logger = slog.Default()
	}

	poolMap := make(map[string]*pgxpool.Pool, len(initialPools))
	for name, pool := range initialPools {
		if pool == nil {
			continue
		}
		poolMap[name] = pool
	}

	service := &LifecycleService{
		store:   store,
		router:  router,
		checker: checker,
		audit:   auditLogger,
		logger:  logger,
		pools:   poolMap,
	}
	service.dialPool = pgxpool.New
	service.queryRow = func(ctx context.Context, pool *pgxpool.Pool, sql string) pgx.Row {
		return pool.QueryRow(ctx, sql)
	}

	return service
}

// TODO: Document LifecycleService.AddReplica.
func (s *LifecycleService) AddReplica(ctx context.Context, record TopologyNodeRecord) (createdRecord TopologyNodeRecord, err error) {
	normalizedRecord, err := s.normalizeReplicaRecordForAdd(record)
	if err != nil {
		return TopologyNodeRecord{}, err
	}
	if err := s.ensureReplicaNameAvailable(ctx, normalizedRecord.Name); err != nil {
		return TopologyNodeRecord{}, err
	}

	connectionURL := normalizedRecord.ConnectionURL()
	dialURL := s.replicaDialURL(connectionURL)
	replicaConfig := NormalizeReplicaConfig(config.ReplicaConfig{
		URL:         connectionURL,
		Weight:      normalizedRecord.Weight,
		MaxLagBytes: normalizedRecord.MaxLagBytes,
	})

	tempPool, err := s.dialPool(ctx, dialURL)
	if err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("add replica %q: dial connectivity pool: %w", normalizedRecord.Name, err)
	}
	if err := s.testReplicaConnectivity(ctx, tempPool); err != nil {
		tempPool.Close()
		return TopologyNodeRecord{}, fmt.Errorf("add replica %q: connectivity check failed: %w", normalizedRecord.Name, err)
	}
	if err := s.verifyIsReplica(ctx, tempPool); err != nil {
		tempPool.Close()
		return TopologyNodeRecord{}, fmt.Errorf("add replica %q: %w", normalizedRecord.Name, err)
	}
	tempPool.Close()

	runtimePool, err := s.dialPool(ctx, dialURL)
	if err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("add replica %q: dial runtime pool: %w", normalizedRecord.Name, err)
	}

	persisted := false
	routerAttached := false
	checkerAttached := false
	mapAttached := false
	defer func() {
		if err == nil {
			return
		}
		if checkerAttached {
			s.checker.RemoveStatus(runtimePool)
		}
		if routerAttached {
			s.router.RemoveReplicaEntry(runtimePool)
		}
		if mapAttached {
			s.deletePool(normalizedRecord.Name)
		}
		runtimePool.Close()
		if persisted {
			_ = s.store.UpdateState(ctx, normalizedRecord.Name, TopologyStateRemoved)
		}
	}()

	if err := s.store.Add(ctx, normalizedRecord); err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("add replica %q: persist topology record: %w", normalizedRecord.Name, err)
	}
	persisted = true

	s.router.AddReplicaEntry(runtimePool, normalizedRecord.Name, replicaConfig)
	routerAttached = true
	s.checker.AddStatus(runtimePool, normalizedRecord.Name, replicaConfig)
	checkerAttached = true
	s.setPool(normalizedRecord.Name, runtimePool)
	mapAttached = true

	if err := s.logLifecycleAudit(ctx, audit.AuditEntry{
		TableName: "_ayb_replicas",
		RecordID:  normalizedRecord.Name,
		Operation: "INSERT",
		NewValues: map[string]any{
			"name":  normalizedRecord.Name,
			"role":  TopologyRoleReplica,
			"state": TopologyStateActive,
		},
	}); err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("add replica %q: write audit event: %w", normalizedRecord.Name, err)
	}

	return normalizedRecord, nil
}

// TODO: Document LifecycleService.RemoveReplica.
func (s *LifecycleService) RemoveReplica(ctx context.Context, name string, force bool) error {
	record, err := s.store.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("remove replica %q: %w", name, err)
	}
	if record.Role != TopologyRoleReplica {
		return fmt.Errorf("remove replica %q: target is not a replica", name)
	}
	if !force {
		activeReplicas, err := s.activeReplicaCount(ctx)
		if err != nil {
			return fmt.Errorf("remove replica %q: count active replicas: %w", name, err)
		}
		if activeReplicas <= 1 {
			return fmt.Errorf("remove replica %q: refusing to remove last active replica without force", name)
		}
	}

	if err := s.store.UpdateState(ctx, name, TopologyStateDraining); err != nil {
		return fmt.Errorf("remove replica %q: transition to draining: %w", name, err)
	}

	replicaPool, _ := s.poolForName(name)
	if replicaPool != nil {
		s.checker.RemoveStatus(replicaPool)
		s.router.RemoveReplicaEntry(replicaPool)
		replicaPool.Close()
	}

	if err := s.store.UpdateState(ctx, name, TopologyStateRemoved); err != nil {
		return fmt.Errorf("remove replica %q: transition to removed: %w", name, err)
	}
	s.deletePool(name)

	if err := s.logLifecycleAudit(ctx, audit.AuditEntry{
		TableName: "_ayb_replicas",
		RecordID:  name,
		Operation: "UPDATE",
		NewValues: map[string]any{"state": TopologyStateRemoved},
	}); err != nil {
		return fmt.Errorf("remove replica %q: write audit event: %w", name, err)
	}

	return nil
}

// TODO: Document LifecycleService.PromoteReplica.
func (s *LifecycleService) PromoteReplica(ctx context.Context, name string) (TopologyNodeRecord, error) {
	record, err := s.store.Get(ctx, name)
	if err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: %w", name, err)
	}
	if record.Role != TopologyRoleReplica {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: target is not a replica", name)
	}
	if record.State != TopologyStateActive {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: target is not active", name)
	}

	targetPool, ok := s.poolForName(name)
	if !ok || targetPool == nil {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: runtime pool not found", name)
	}
	if err := s.ensureReplicaHealthy(targetPool); err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: %w", name, err)
	}

	connectionURL := record.ConnectionURL()
	dialURL := s.replicaDialURL(connectionURL)
	controlPool, err := s.dialPool(ctx, dialURL)
	if err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: dial promotion pool: %w", name, err)
	}
	defer controlPool.Close()

	if err := s.issuePromotionCommand(ctx, controlPool); err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: promote command failed: %w", name, err)
	}
	if err := s.waitForPromotion(ctx, controlPool, replicaPromotionTimeout); err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: promotion wait failed: %w", name, err)
	}

	storePromotionErr := s.store.PromoteNode(ctx, name)
	s.promoteRuntimeTarget(ctx, name, targetPool, dialURL)
	if storePromotionErr != nil {
		s.logger.Warn("promoted replica store sync failed after pg_promote; runtime switched to promoted target",
			slog.String("name", name),
			slog.Any("error", storePromotionErr),
		)
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: persist promotion: %w", name, storePromotionErr)
	}

	if err := s.logLifecycleAudit(ctx, audit.AuditEntry{
		TableName: "_ayb_replicas",
		RecordID:  name,
		Operation: "UPDATE",
		NewValues: map[string]any{"role": TopologyRolePrimary},
	}); err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("promote replica %q: write audit event: %w", name, err)
	}

	record.Role = TopologyRolePrimary
	return record, nil
}

// TODO: Document LifecycleService.InitiateFailover.
func (s *LifecycleService) InitiateFailover(ctx context.Context, target string, force bool) error {
	selectedTarget := target
	reason := "explicit-target"
	if selectedTarget == "" {
		candidate, err := s.selectFailoverCandidate()
		if err != nil {
			return fmt.Errorf("initiate failover: %w", err)
		}
		selectedTarget = candidate
		reason = "auto-lowest-lag"
	}

	if !force {
		if err := s.checkPrimaryReachable(ctx, s.router.Primary()); err == nil {
			return errors.New("initiate failover: primary is healthy; pass force=true to override")
		}
	}

	if _, err := s.PromoteReplica(ctx, selectedTarget); err != nil {
		return err
	}

	if err := s.logLifecycleAudit(ctx, audit.AuditEntry{
		TableName: "_ayb_replicas",
		RecordID:  selectedTarget,
		Operation: "UPDATE",
		NewValues: map[string]any{
			"operation": "failover",
			"reason":    reason,
		},
	}); err != nil {
		return fmt.Errorf("initiate failover: write audit event: %w", err)
	}

	return nil
}

func (s *LifecycleService) normalizeReplicaRecordForAdd(record TopologyNodeRecord) (TopologyNodeRecord, error) {
	record.Role = TopologyRoleReplica
	record.State = TopologyStateActive
	normalizedRecord, err := normalizeTopologyNodeRecord(record)
	if err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("normalize replica topology record %q: %w", record.Name, err)
	}
	return normalizedRecord, nil
}

func (s *LifecycleService) ensureReplicaNameAvailable(ctx context.Context, name string) error {
	_, err := s.store.Get(ctx, name)
	if err == nil {
		return fmt.Errorf("replica topology record %q already exists", name)
	}
	if !errors.Is(err, ErrTopologyNodeNotFound) {
		return fmt.Errorf("check existing replica topology record %q: %w", name, err)
	}
	return nil
}

func (s *LifecycleService) issuePromotionCommand(ctx context.Context, pool *pgxpool.Pool) error {
	var promoted bool
	if err := s.queryRow(ctx, pool, "SELECT pg_promote()").Scan(&promoted); err != nil {
		return err
	}
	if !promoted {
		return errors.New("pg_promote returned false")
	}
	return nil
}

// TODO: Document LifecycleService.promoteRuntimeTarget.
func (s *LifecycleService) promoteRuntimeTarget(ctx context.Context, name string, targetPool *pgxpool.Pool, dialURL string) {
	primaryPool := targetPool
	replacementPool, err := s.dialPool(ctx, dialURL)
	if err != nil {
		s.logger.Warn("promoted replica replacement pool dial failed; reusing runtime replica pool as primary",
			slog.String("name", name),
			slog.Any("error", err),
		)
	} else {
		primaryPool = replacementPool
	}

	s.router.RemoveReplicaEntry(targetPool)
	s.checker.RemoveStatus(targetPool)
	s.router.SwapPrimary(primaryPool)
	s.setPool(name, primaryPool)

	if primaryPool != targetPool {
		targetPool.Close()
	}
}

func (s *LifecycleService) ensureReplicaHealthy(pool *pgxpool.Pool) error {
	statuses := s.checker.Statuses()
	for _, status := range statuses {
		if status.Pool == pool && status.State == HealthHealthy {
			return nil
		}
	}
	return errors.New("target replica is not healthy")
}

func (s *LifecycleService) replicaDialURL(connectionURL string) string {
	return DialURLWithPrimaryCredentials(connectionURL, s.router.Primary())
}

func (s *LifecycleService) activeReplicaCount(ctx context.Context) (int, error) {
	records, err := s.store.List(ctx)
	if err != nil {
		return 0, err
	}

	activeReplicas := 0
	for _, record := range records {
		if record.Role == TopologyRoleReplica && record.State == TopologyStateActive {
			activeReplicas++
		}
	}
	return activeReplicas, nil
}

// TODO: Document LifecycleService.selectFailoverCandidate.
func (s *LifecycleService) selectFailoverCandidate() (string, error) {
	statuses := s.checker.Statuses()
	var (
		selectedName string
		selectedLag  int64
		selectedSet  bool
	)

	for _, status := range statuses {
		if status.State != HealthHealthy || status.Pool == nil {
			continue
		}
		name, ok := s.nameForPool(status.Pool)
		if !ok {
			continue
		}
		if !selectedSet || status.LagBytes < selectedLag {
			selectedName = name
			selectedLag = status.LagBytes
			selectedSet = true
		}
	}

	if !selectedSet {
		return "", errors.New("no healthy replica candidates available")
	}
	return selectedName, nil
}

func (s *LifecycleService) setPool(name string, pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	s.poolsMu.Lock()
	defer s.poolsMu.Unlock()
	s.pools[name] = pool
}

func (s *LifecycleService) deletePool(name string) {
	s.poolsMu.Lock()
	defer s.poolsMu.Unlock()
	delete(s.pools, name)
}

func (s *LifecycleService) poolForName(name string) (*pgxpool.Pool, bool) {
	s.poolsMu.RLock()
	defer s.poolsMu.RUnlock()
	pool, ok := s.pools[name]
	return pool, ok
}

func (s *LifecycleService) nameForPool(pool *pgxpool.Pool) (string, bool) {
	s.poolsMu.RLock()
	defer s.poolsMu.RUnlock()
	for name, candidate := range s.pools {
		if candidate == pool {
			return name, true
		}
	}
	return "", false
}

func (s *LifecycleService) logLifecycleAudit(ctx context.Context, entry audit.AuditEntry) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.LogMutation(ctx, entry)
}

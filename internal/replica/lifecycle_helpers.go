// Package replica Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/replica/lifecycle_helpers.go.
package replica

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func (s *LifecycleService) probePoolOne(ctx context.Context, pool *pgxpool.Pool, nilMessage, probeName string) error {
	if pool == nil {
		return errors.New(nilMessage)
	}

	var one int
	if err := s.queryRow(ctx, pool, "SELECT 1").Scan(&one); err != nil {
		return err
	}
	if one != 1 {
		return fmt.Errorf("unexpected %s probe result: %d", probeName, one)
	}
	return nil
}

func (s *LifecycleService) testReplicaConnectivity(ctx context.Context, pool *pgxpool.Pool) error {
	return s.probePoolOne(ctx, pool, "replica pool is nil", "connectivity")
}

func (s *LifecycleService) verifyIsReplica(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("replica pool is nil")
	}
	var inRecovery bool
	if err := s.queryRow(ctx, pool, "SELECT pg_is_in_recovery()").Scan(&inRecovery); err != nil {
		return err
	}
	if !inRecovery {
		return errors.New("target is not a standby replica")
	}
	return nil
}

// TODO: Document LifecycleService.waitForPromotion.
func (s *LifecycleService) waitForPromotion(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	if pool == nil {
		return errors.New("promotion pool is nil")
	}
	if timeout <= 0 {
		timeout = replicaPromotionTimeout
	}

	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		var inRecovery bool
		if err := s.queryRow(pollCtx, pool, "SELECT pg_is_in_recovery()").Scan(&inRecovery); err != nil {
			return err
		}
		if !inRecovery {
			return nil
		}
		select {
		case <-pollCtx.Done():
			return fmt.Errorf("promotion timed out after %s: %w", timeout, pollCtx.Err())
		case <-time.After(replicaPromotionPollWait):
		}
	}
}

func (s *LifecycleService) checkPrimaryReachable(ctx context.Context, pool *pgxpool.Pool) error {
	probeCtx, cancel := context.WithTimeout(ctx, replicaPrimaryProbeDelay)
	defer cancel()

	return s.probePoolOne(probeCtx, pool, "primary pool is nil", "primary")
}

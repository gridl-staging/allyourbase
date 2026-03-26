// Package replica Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/replica/store_promote.go.
package replica

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type replicaRoleStateRow struct {
	role  string
	state string
}

// TODO: Document PostgresReplicaStore.PromoteNode.
func (s *PostgresReplicaStore) PromoteNode(ctx context.Context, targetName string) error {
	targetName = strings.TrimSpace(targetName)
	if targetName == "" {
		return errors.New("promote replica topology node: target not found")
	}

	beginner, ok := s.db.(replicaTxBeginner)
	if !ok {
		return s.promoteNodeWithExecQuerier(ctx, s.db, targetName)
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("promote replica topology node %q: begin transaction: %w", targetName, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := s.promoteNodeWithExecQuerier(ctx, tx, targetName); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("promote replica topology node %q: commit transaction: %w", targetName, err)
	}
	committed = true
	return nil
}

// TODO: Document PostgresReplicaStore.promoteNodeWithExecQuerier.
func (s *PostgresReplicaStore) promoteNodeWithExecQuerier(ctx context.Context, q replicaExecQuerier, targetName string) error {
	targetRoleState, err := loadReplicaRoleState(ctx, q, targetName)
	if err != nil {
		return fmt.Errorf("promote replica topology node %q: %w", targetName, err)
	}
	if targetRoleState.role == TopologyRolePrimary {
		return fmt.Errorf("promote replica topology node %q: target is already primary", targetName)
	}
	if targetRoleState.state != TopologyStateActive {
		return fmt.Errorf("promote replica topology node %q: target is not active", targetName)
	}

	currentPrimaryName, err := loadCurrentPrimaryName(ctx, q)
	if err != nil {
		return fmt.Errorf("promote replica topology node %q: %w", targetName, err)
	}

	if err := updateReplicaState(ctx, q, currentPrimaryName, TopologyStateRemoved); err != nil {
		return fmt.Errorf("promote replica topology node %q: %w", targetName, err)
	}
	if err := updateReplicaRole(ctx, q, targetName, TopologyRolePrimary); err != nil {
		return fmt.Errorf("promote replica topology node %q: %w", targetName, err)
	}

	return nil
}

func loadReplicaRoleState(ctx context.Context, q replicaRoleStateQuerier, name string) (replicaRoleStateRow, error) {
	row := q.QueryRow(ctx, `SELECT role, state FROM _ayb_replicas WHERE name = $1`, name)
	var result replicaRoleStateRow
	if err := row.Scan(&result.role, &result.state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return replicaRoleStateRow{}, errors.New("target not found")
		}
		return replicaRoleStateRow{}, fmt.Errorf("read target role/state: %w", err)
	}
	return result, nil
}

func loadCurrentPrimaryName(ctx context.Context, q replicaRoleStateQuerier) (string, error) {
	row := q.QueryRow(ctx, `SELECT name FROM _ayb_replicas WHERE role = $1 AND state != $2`, TopologyRolePrimary, TopologyStateRemoved)
	var currentPrimaryName string
	if err := row.Scan(&currentPrimaryName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("no current primary")
		}
		return "", fmt.Errorf("read current primary: %w", err)
	}
	return currentPrimaryName, nil
}

func updateReplicaState(ctx context.Context, q replicaExecQuerier, name, state string) error {
	result, err := q.Exec(ctx, `UPDATE _ayb_replicas SET state = $1, updated_at = NOW() WHERE name = $2`, state, name)
	if err != nil {
		return fmt.Errorf("set state %q for %q: %w", state, name, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("state update target not found: %s", name)
	}
	return nil
}

func updateReplicaRole(ctx context.Context, q replicaExecQuerier, name, role string) error {
	result, err := q.Exec(ctx, `UPDATE _ayb_replicas SET role = $1, updated_at = NOW() WHERE name = $2`, role, name)
	if err != nil {
		return fmt.Errorf("set role %q for %q: %w", role, name, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("role update target not found: %s", name)
	}
	return nil
}

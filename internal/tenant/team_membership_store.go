// Package tenant Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/tenant/team_membership_store.go.
package tenant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const teamMembershipColumns = `id, team_id, user_id, role, created_at`

// TeamMembershipStore defines CRUD operations for team memberships.
type TeamMembershipStore interface {
	AddTeamMembership(ctx context.Context, teamID, userID, role string) (*TeamMembership, error)
	GetTeamMembership(ctx context.Context, teamID, userID string) (*TeamMembership, error)
	ListTeamMemberships(ctx context.Context, teamID string) ([]TeamMembership, error)
	ListUserTeamMemberships(ctx context.Context, userID string) ([]TeamMembership, error)
	RemoveTeamMembership(ctx context.Context, teamID, userID string) error
	UpdateTeamMembershipRole(ctx context.Context, teamID, userID, role string) (*TeamMembership, error)
}

// PostgresTeamMembershipStore persists team memberships in Postgres.
type PostgresTeamMembershipStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewPostgresTeamMembershipStore(pool *pgxpool.Pool, logger *slog.Logger) *PostgresTeamMembershipStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &PostgresTeamMembershipStore{pool: pool, logger: logger}
}

func scanTeamMembership(row pgx.Row) (*TeamMembership, error) {
	var membership TeamMembership
	err := row.Scan(
		&membership.ID,
		&membership.TeamID,
		&membership.UserID,
		&membership.Role,
		&membership.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &membership, nil
}

// TODO: Document scanTeamMemberships.
func scanTeamMemberships(rows pgx.Rows) ([]TeamMembership, error) {
	var memberships []TeamMembership
	for rows.Next() {
		membership, err := scanTeamMembership(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, *membership)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if memberships == nil {
		memberships = []TeamMembership{}
	}
	return memberships, nil
}

// TODO: Document PostgresTeamMembershipStore.AddTeamMembership.
func (s *PostgresTeamMembershipStore) AddTeamMembership(ctx context.Context, teamID, userID, role string) (*TeamMembership, error) {
	if !IsValidTeamRole(role) {
		return nil, ErrInvalidRole
	}

	tx, err := s.beginTeamMembershipTx(ctx, teamID, "add")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	membership, err := scanTeamMembership(tx.QueryRow(ctx,
		`INSERT INTO _ayb_team_memberships (team_id, user_id, role)
		 VALUES ($1, $2, $3)
		 RETURNING `+teamMembershipColumns,
		teamID,
		userID,
		role,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return nil, ErrTeamMembershipExists
			}
			if pgErr.Code == "23503" {
				return nil, fmt.Errorf("invalid team membership reference: %w", err)
			}
		}
		return nil, fmt.Errorf("adding team membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing team membership add transaction: %w", err)
	}

	s.logger.Info("team membership added", "team_id", teamID, "user_id", userID, "role", role)
	return membership, nil
}

// TODO: Document PostgresTeamMembershipStore.GetTeamMembership.
func (s *PostgresTeamMembershipStore) GetTeamMembership(ctx context.Context, teamID, userID string) (*TeamMembership, error) {
	membership, err := scanTeamMembership(s.pool.QueryRow(ctx,
		`SELECT `+teamMembershipColumns+`
		 FROM _ayb_team_memberships
		 WHERE team_id = $1 AND user_id = $2`,
		teamID,
		userID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamMembershipNotFound
		}
		return nil, fmt.Errorf("getting team membership: %w", err)
	}
	return membership, nil
}

// TODO: Document PostgresTeamMembershipStore.ListTeamMemberships.
func (s *PostgresTeamMembershipStore) ListTeamMemberships(ctx context.Context, teamID string) ([]TeamMembership, error) {
	tx, err := s.beginTeamMembershipTx(ctx, teamID, "list")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT `+teamMembershipColumns+`
		 FROM _ayb_team_memberships
		 WHERE team_id = $1
		 ORDER BY created_at ASC`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing team memberships: %w", err)
	}
	memberships, err := scanTeamMemberships(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing team membership list transaction: %w", err)
	}
	return memberships, nil
}

func (s *PostgresTeamMembershipStore) ListUserTeamMemberships(ctx context.Context, userID string) ([]TeamMembership, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+teamMembershipColumns+`
		 FROM _ayb_team_memberships
		 WHERE user_id = $1
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing user team memberships: %w", err)
	}
	defer rows.Close()

	return scanTeamMemberships(rows)
}

// TODO: Document PostgresTeamMembershipStore.RemoveTeamMembership.
func (s *PostgresTeamMembershipStore) RemoveTeamMembership(ctx context.Context, teamID, userID string) error {
	tx, err := s.beginTeamMembershipTx(ctx, teamID, "remove")
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx,
		`DELETE FROM _ayb_team_memberships WHERE team_id = $1 AND user_id = $2`,
		teamID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("removing team membership: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrTeamMembershipNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing team membership remove transaction: %w", err)
	}

	s.logger.Info("team membership removed", "team_id", teamID, "user_id", userID)
	return nil
}

// TODO: Document PostgresTeamMembershipStore.UpdateTeamMembershipRole.
func (s *PostgresTeamMembershipStore) UpdateTeamMembershipRole(ctx context.Context, teamID, userID, role string) (*TeamMembership, error) {
	if !IsValidTeamRole(role) {
		return nil, ErrInvalidRole
	}

	tx, err := s.beginTeamMembershipTx(ctx, teamID, "role update")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	membership, err := scanTeamMembership(tx.QueryRow(ctx,
		`UPDATE _ayb_team_memberships
		 SET role = $3
		 WHERE team_id = $1 AND user_id = $2
		 RETURNING `+teamMembershipColumns,
		teamID,
		userID,
		role,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamMembershipNotFound
		}
		return nil, fmt.Errorf("updating team membership role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing team membership role update transaction: %w", err)
	}

	s.logger.Info("team membership role updated", "team_id", teamID, "user_id", userID, "role", role)
	return membership, nil
}

func (s *PostgresTeamMembershipStore) beginTeamMembershipTx(ctx context.Context, teamID, operation string) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting team membership %s transaction: %w", operation, err)
	}
	if err := lockTeamMembershipTeam(ctx, tx, teamID); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

// TODO: Document lockTeamMembershipTeam.
func lockTeamMembershipTeam(ctx context.Context, tx pgx.Tx, teamID string) error {
	var lockedTeamID string
	err := tx.QueryRow(ctx,
		`SELECT id
		 FROM _ayb_teams
		 WHERE id = $1
		 FOR KEY SHARE`,
		teamID,
	).Scan(&lockedTeamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTeamNotFound
		}
		return fmt.Errorf("locking team membership team: %w", err)
	}
	return nil
}

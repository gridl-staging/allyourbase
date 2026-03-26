// Package tenant Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/tenant/team_store.go.
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

const teamColumns = `id, org_id, name, slug, created_at, updated_at`

// TeamStore defines CRUD operations for teams.
type TeamStore interface {
	CreateTeam(ctx context.Context, orgID, name, slug string) (*Team, error)
	GetTeam(ctx context.Context, id string) (*Team, error)
	ListTeams(ctx context.Context, orgID string) ([]Team, error)
	UpdateTeam(ctx context.Context, id string, update TeamUpdate) (*Team, error)
	DeleteTeam(ctx context.Context, id string) error
}

// PostgresTeamStore persists teams in Postgres.
type PostgresTeamStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewPostgresTeamStore(pool *pgxpool.Pool, logger *slog.Logger) *PostgresTeamStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &PostgresTeamStore{pool: pool, logger: logger}
}

func scanTeam(row pgx.Row) (*Team, error) {
	var team Team
	err := row.Scan(&team.ID, &team.OrgID, &team.Name, &team.Slug, &team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

// TODO: Document scanTeams.
func scanTeams(rows pgx.Rows) ([]Team, error) {
	var items []Team
	for rows.Next() {
		team, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *team)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []Team{}
	}
	return items, nil
}

// TODO: Document PostgresTeamStore.CreateTeam.
func (s *PostgresTeamStore) CreateTeam(ctx context.Context, orgID, name, slug string) (*Team, error) {
	if !IsValidSlug(slug) {
		return nil, ErrInvalidSlug
	}

	tx, err := s.beginOrgScopedTeamTx(ctx, orgID, "create")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	team, err := scanTeam(tx.QueryRow(ctx,
		`INSERT INTO _ayb_teams (org_id, name, slug)
		 VALUES ($1, $2, $3)
		 RETURNING `+teamColumns,
		orgID,
		name,
		slug,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return nil, ErrTeamSlugTaken
			}
			if pgErr.Code == "23503" {
				return nil, ErrOrgNotFound
			}
		}
		return nil, fmt.Errorf("creating team: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing team create transaction: %w", err)
	}

	s.logger.Info("team created", "team_id", team.ID, "org_id", team.OrgID)
	return team, nil
}

func (s *PostgresTeamStore) GetTeam(ctx context.Context, id string) (*Team, error) {
	team, err := scanTeam(s.pool.QueryRow(ctx,
		`SELECT `+teamColumns+` FROM _ayb_teams WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("getting team: %w", err)
	}
	return team, nil
}

// TODO: Document PostgresTeamStore.ListTeams.
func (s *PostgresTeamStore) ListTeams(ctx context.Context, orgID string) ([]Team, error) {
	tx, err := s.beginOrgScopedTeamTx(ctx, orgID, "list")
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT `+teamColumns+` FROM _ayb_teams WHERE org_id = $1 ORDER BY name ASC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing teams: %w", err)
	}
	teams, err := scanTeams(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing team list transaction: %w", err)
	}
	return teams, nil
}

// TODO: Document PostgresTeamStore.UpdateTeam.
func (s *PostgresTeamStore) UpdateTeam(ctx context.Context, id string, update TeamUpdate) (*Team, error) {
	if update.Slug != nil && !IsValidSlug(*update.Slug) {
		return nil, ErrInvalidSlug
	}

	team, err := scanTeam(s.pool.QueryRow(ctx,
		`UPDATE _ayb_teams
		 SET name = COALESCE($2, name),
		     slug = COALESCE($3, slug),
		     updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+teamColumns,
		id,
		update.Name,
		update.Slug,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrTeamSlugTaken
		}
		return nil, fmt.Errorf("updating team: %w", err)
	}

	s.logger.Info("team updated", "team_id", id)
	return team, nil
}

func (s *PostgresTeamStore) DeleteTeam(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM _ayb_teams WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting team: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrTeamNotFound
	}

	s.logger.Info("team deleted", "team_id", id)
	return nil
}

func (s *PostgresTeamStore) beginOrgScopedTeamTx(ctx context.Context, orgID, operation string) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting team %s transaction: %w", operation, err)
	}
	if err := lockTeamOrg(ctx, tx, orgID); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

// TODO: Document lockTeamOrg.
func lockTeamOrg(ctx context.Context, tx pgx.Tx, orgID string) error {
	var lockedOrgID string
	err := tx.QueryRow(ctx,
		`SELECT id
		 FROM _ayb_organizations
		 WHERE id = $1
		 FOR KEY SHARE`,
		orgID,
	).Scan(&lockedOrgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrgNotFound
		}
		return fmt.Errorf("locking team org: %w", err)
	}
	return nil
}

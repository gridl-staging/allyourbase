// Package tenant Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/tenant/org_store.go.
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

const orgColumns = `id, name, slug, parent_org_id, plan_tier, created_at, updated_at`

func orgColumnsWithAlias(alias string) string {
	return alias + `.id, ` +
		alias + `.name, ` +
		alias + `.slug, ` +
		alias + `.parent_org_id, ` +
		alias + `.plan_tier, ` +
		alias + `.created_at, ` +
		alias + `.updated_at`
}

// OrgStore defines CRUD operations for organizations.
type OrgStore interface {
	CreateOrg(ctx context.Context, name, slug string, parentOrgID *string, planTier string) (*Organization, error)
	GetOrg(ctx context.Context, id string) (*Organization, error)
	GetOrgBySlug(ctx context.Context, slug string) (*Organization, error)
	ListOrgs(ctx context.Context, userID string) ([]Organization, error)
	ListChildOrgs(ctx context.Context, parentOrgID string) ([]Organization, error)
	UpdateOrg(ctx context.Context, id string, update OrgUpdate) (*Organization, error)
	DeleteOrg(ctx context.Context, id string) error
}

// PostgresOrgStore persists organizations in Postgres.
type PostgresOrgStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewPostgresOrgStore(pool *pgxpool.Pool, logger *slog.Logger) *PostgresOrgStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &PostgresOrgStore{pool: pool, logger: logger}
}

// TODO: Document scanOrg.
func scanOrg(row pgx.Row) (*Organization, error) {
	var org Organization
	err := row.Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.ParentOrgID,
		&org.PlanTier,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// TODO: Document scanOrgs.
func scanOrgs(rows pgx.Rows) ([]Organization, error) {
	var items []Organization
	for rows.Next() {
		org, err := scanOrg(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *org)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []Organization{}
	}
	return items, nil
}

// TODO: Document PostgresOrgStore.CreateOrg.
func (s *PostgresOrgStore) CreateOrg(ctx context.Context, name, slug string, parentOrgID *string, planTier string) (*Organization, error) {
	if !IsValidSlug(slug) {
		return nil, ErrInvalidSlug
	}

	if err := s.validateParentOrgCycle(ctx, "", parentOrgID); err != nil {
		return nil, err
	}

	if planTier == "" {
		planTier = "free"
	}

	org, err := scanOrg(s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_organizations (name, slug, parent_org_id, plan_tier)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+orgColumns,
		name, slug, parentOrgID, planTier,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return nil, ErrOrgSlugTaken
			}
			if pgErr.Code == "23503" {
				return nil, ErrParentOrgNotFound
			}
		}
		return nil, fmt.Errorf("creating org: %w", err)
	}

	s.logger.Info("org created", "org_id", org.ID, "slug", org.Slug)
	return org, nil
}

func (s *PostgresOrgStore) GetOrg(ctx context.Context, id string) (*Organization, error) {
	org, err := scanOrg(s.pool.QueryRow(ctx,
		`SELECT `+orgColumns+` FROM _ayb_organizations WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("getting org: %w", err)
	}
	return org, nil
}

func (s *PostgresOrgStore) GetOrgBySlug(ctx context.Context, slug string) (*Organization, error) {
	org, err := scanOrg(s.pool.QueryRow(ctx,
		`SELECT `+orgColumns+` FROM _ayb_organizations WHERE slug = $1`,
		slug,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("getting org by slug: %w", err)
	}
	return org, nil
}

// TODO: Document PostgresOrgStore.ListOrgs.
func (s *PostgresOrgStore) ListOrgs(ctx context.Context, userID string) ([]Organization, error) {
	var rows pgx.Rows
	var err error

	if userID == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT `+orgColumns+` FROM _ayb_organizations ORDER BY created_at DESC`,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT `+orgColumnsWithAlias("o")+` FROM _ayb_organizations o
			 JOIN _ayb_org_memberships m ON m.org_id = o.id
			 WHERE m.user_id = $1
			 ORDER BY o.created_at DESC`,
			userID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("listing orgs: %w", err)
	}
	defer rows.Close()

	return scanOrgs(rows)
}

func (s *PostgresOrgStore) ListChildOrgs(ctx context.Context, parentOrgID string) ([]Organization, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+orgColumns+` FROM _ayb_organizations WHERE parent_org_id = $1 ORDER BY created_at DESC`,
		parentOrgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing child orgs: %w", err)
	}
	defer rows.Close()

	return scanOrgs(rows)
}

// TODO: Document PostgresOrgStore.UpdateOrg.
func (s *PostgresOrgStore) UpdateOrg(ctx context.Context, id string, update OrgUpdate) (*Organization, error) {
	if err := s.validateParentOrgCycle(ctx, id, update.ParentOrgID); err != nil {
		return nil, err
	}

	if update.Slug != nil && !IsValidSlug(*update.Slug) {
		return nil, ErrInvalidSlug
	}

	org, err := scanOrg(s.pool.QueryRow(ctx,
		`UPDATE _ayb_organizations
		 SET name = COALESCE($2, name),
		     slug = COALESCE($3, slug),
		     parent_org_id = CASE
				 WHEN $4::text IS NULL THEN parent_org_id
				 WHEN $4::text = '' THEN NULL
				 ELSE $4::uuid
			   END,
		     updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+orgColumns,
		id,
		update.Name,
		update.Slug,
		update.ParentOrgID,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return nil, ErrOrgSlugTaken
			}
			if pgErr.Code == "23503" {
				return nil, ErrParentOrgNotFound
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("updating org: %w", err)
	}

	s.logger.Info("org updated", "org_id", id)
	return org, nil
}

func (s *PostgresOrgStore) DeleteOrg(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM _ayb_organizations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting org: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrgNotFound
	}

	s.logger.Info("org deleted", "org_id", id)
	return nil
}

// TODO: Document PostgresOrgStore.validateParentOrgCycle.
func (s *PostgresOrgStore) validateParentOrgCycle(ctx context.Context, orgID string, parentOrgID *string) error {
	if parentOrgID == nil {
		return nil
	}
	if *parentOrgID == "" {
		return nil
	}
	if orgID != "" && *parentOrgID == orgID {
		return ErrCircularParentOrg
	}
	if orgID == "" {
		return nil
	}
	var isCycle bool
	err := s.pool.QueryRow(ctx, `WITH RECURSIVE org_chain AS (
		SELECT id, parent_org_id, ARRAY[id] AS visited_org_ids
		  FROM _ayb_organizations
		 WHERE id = $1
		UNION ALL
		SELECT o.id, o.parent_org_id, chain.visited_org_ids || o.id
		  FROM _ayb_organizations o
		  JOIN org_chain chain ON o.id = chain.parent_org_id
		 WHERE chain.parent_org_id IS NOT NULL
		   AND NOT o.id = ANY(chain.visited_org_ids)
	)
			SELECT EXISTS (
				SELECT 1 FROM org_chain WHERE id = $2
			)`,
		*parentOrgID,
		orgID,
	).Scan(&isCycle)
	if err != nil {
		return fmt.Errorf("checking org hierarchy: %w", err)
	}
	if isCycle {
		return ErrCircularParentOrg
	}
	return nil
}

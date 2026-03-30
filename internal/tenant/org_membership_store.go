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

const orgMembershipColumns = `id, org_id, user_id, role, created_at`

// OrgMembershipStore defines CRUD operations for organization memberships.
type OrgMembershipStore interface {
	AddOrgMembership(ctx context.Context, orgID, userID, role string) (*OrgMembership, error)
	GetOrgMembership(ctx context.Context, orgID, userID string) (*OrgMembership, error)
	ListOrgMemberships(ctx context.Context, orgID string) ([]OrgMembership, error)
	ListUserOrgMemberships(ctx context.Context, userID string) ([]OrgMembership, error)
	RemoveOrgMembership(ctx context.Context, orgID, userID string) error
	UpdateOrgMembershipRole(ctx context.Context, orgID, userID, role string) (*OrgMembership, error)
}

// PostgresOrgMembershipStore persists org memberships in Postgres.
type PostgresOrgMembershipStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewPostgresOrgMembershipStore(pool *pgxpool.Pool, logger *slog.Logger) *PostgresOrgMembershipStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &PostgresOrgMembershipStore{pool: pool, logger: logger}
}

func scanOrgMembership(row pgx.Row) (*OrgMembership, error) {
	var membership OrgMembership
	err := row.Scan(
		&membership.ID,
		&membership.OrgID,
		&membership.UserID,
		&membership.Role,
		&membership.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &membership, nil
}

// scanOrgMemberships collects all rows into a slice, returning an empty (non-nil) slice when no rows exist.
func scanOrgMemberships(rows pgx.Rows) ([]OrgMembership, error) {
	var memberships []OrgMembership
	for rows.Next() {
		membership, err := scanOrgMembership(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, *membership)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if memberships == nil {
		memberships = []OrgMembership{}
	}
	return memberships, nil
}

// AddOrgMembership inserts a new membership within a transaction that locks the org row first to prevent concurrent duplicate inserts.
func (s *PostgresOrgMembershipStore) AddOrgMembership(ctx context.Context, orgID, userID, role string) (*OrgMembership, error) {
	if !IsValidRole(role) {
		return nil, ErrInvalidRole
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting org membership add transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockOrgMembershipOrg(ctx, tx, orgID); err != nil {
		return nil, err
	}

	membership, err := scanOrgMembership(tx.QueryRow(ctx,
		`INSERT INTO _ayb_org_memberships (org_id, user_id, role)
		 VALUES ($1, $2, $3)
		 RETURNING `+orgMembershipColumns,
		orgID,
		userID,
		role,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return nil, ErrOrgMembershipExists
			}
			if pgErr.Code == "23503" {
				return nil, fmt.Errorf("invalid org membership reference: %w", err)
			}
		}
		return nil, fmt.Errorf("adding org membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing org membership add transaction: %w", err)
	}

	s.logger.Info("org membership added", "org_id", orgID, "user_id", userID, "role", role)
	return membership, nil
}

// GetOrgMembership returns a single membership by org and user ID, or ErrOrgMembershipNotFound if it does not exist.
func (s *PostgresOrgMembershipStore) GetOrgMembership(ctx context.Context, orgID, userID string) (*OrgMembership, error) {
	membership, err := scanOrgMembership(s.pool.QueryRow(ctx,
		`SELECT `+orgMembershipColumns+`
		 FROM _ayb_org_memberships
		 WHERE org_id = $1 AND user_id = $2`,
		orgID,
		userID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgMembershipNotFound
		}
		return nil, fmt.Errorf("getting org membership: %w", err)
	}
	return membership, nil
}

// ListOrgMemberships returns all memberships for an org, ordered by creation time, within a transaction that verifies the org exists.
func (s *PostgresOrgMembershipStore) ListOrgMemberships(ctx context.Context, orgID string) ([]OrgMembership, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting org membership list transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockOrgMembershipOrg(ctx, tx, orgID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT `+orgMembershipColumns+`
		 FROM _ayb_org_memberships
		 WHERE org_id = $1
		 ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing org memberships: %w", err)
	}

	memberships, err := scanOrgMemberships(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing org membership list transaction: %w", err)
	}
	return memberships, nil
}

func (s *PostgresOrgMembershipStore) ListUserOrgMemberships(ctx context.Context, userID string) ([]OrgMembership, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+orgMembershipColumns+`
		 FROM _ayb_org_memberships
		 WHERE user_id = $1
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing user org memberships: %w", err)
	}
	defer rows.Close()

	return scanOrgMemberships(rows)
}

// RemoveOrgMembership deletes a membership, refusing to remove the last owner to prevent orphaned orgs.
func (s *PostgresOrgMembershipStore) RemoveOrgMembership(ctx context.Context, orgID, userID string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("starting org membership remove transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockOrgMembershipOrg(ctx, tx, orgID); err != nil {
		return err
	}

	membership, ownerCount, err := lockOrgMembershipChangeRows(ctx, tx, orgID, userID)
	if err != nil {
		return err
	}
	if membership.Role == OrgRoleOwner && ownerCount == 1 {
		return ErrLastOwner
	}

	result, err := tx.Exec(ctx,
		`DELETE FROM _ayb_org_memberships
		 WHERE org_id = $1 AND user_id = $2`,
		orgID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("removing org membership: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrgMembershipNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing org membership remove transaction: %w", err)
	}

	s.logger.Info("org membership removed", "org_id", orgID, "user_id", userID)
	return nil
}

// UpdateOrgMembershipRole changes a member's role, refusing to demote the last owner to prevent orphaned orgs.
func (s *PostgresOrgMembershipStore) UpdateOrgMembershipRole(ctx context.Context, orgID, userID, role string) (*OrgMembership, error) {
	if !IsValidRole(role) {
		return nil, ErrInvalidRole
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting org membership role update transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockOrgMembershipOrg(ctx, tx, orgID); err != nil {
		return nil, err
	}

	membership, ownerCount, err := lockOrgMembershipChangeRows(ctx, tx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if membership.Role == OrgRoleOwner && role != OrgRoleOwner && ownerCount == 1 {
		return nil, ErrLastOwner
	}

	updatedMembership, err := scanOrgMembership(tx.QueryRow(ctx,
		`UPDATE _ayb_org_memberships
		 SET role = $3
		 WHERE org_id = $1 AND user_id = $2
		 RETURNING `+orgMembershipColumns,
		orgID,
		userID,
		role,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgMembershipNotFound
		}
		return nil, fmt.Errorf("updating org membership role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing org membership role update transaction: %w", err)
	}

	s.logger.Info("org membership role updated", "org_id", orgID, "user_id", userID, "role", role)
	return updatedMembership, nil
}

// lockOrgMembershipOrg acquires a FOR KEY SHARE lock on the org row to ensure it is not deleted mid-transaction, returning ErrOrgNotFound if absent.
func lockOrgMembershipOrg(ctx context.Context, tx pgx.Tx, orgID string) error {
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
		return fmt.Errorf("locking org membership org: %w", err)
	}
	return nil
}

// lockOrgMembershipChangeRows locks the target membership row together with all
// current owner rows in a single ordered query so concurrent owner changes do
// not deadlock on mismatched lock acquisition order.
func lockOrgMembershipChangeRows(ctx context.Context, tx pgx.Tx, orgID, userID string) (*OrgMembership, int, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+orgMembershipColumns+`
		 FROM _ayb_org_memberships
		 WHERE org_id = $1 AND (role = $2 OR user_id = $3)
		 ORDER BY user_id
		 FOR UPDATE`,
		orgID,
		OrgRoleOwner,
		userID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("locking org membership change rows: %w", err)
	}
	defer rows.Close()

	var targetMembership *OrgMembership
	ownerCount := 0
	for rows.Next() {
		membership, err := scanOrgMembership(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning locked org membership rows: %w", err)
		}
		if membership.Role == OrgRoleOwner {
			ownerCount++
		}
		if membership.UserID == userID {
			targetMembership = membership
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating locked org membership rows: %w", err)
	}
	if targetMembership == nil {
		return nil, 0, ErrOrgMembershipNotFound
	}

	return targetMembership, ownerCount, nil
}

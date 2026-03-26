package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// scanMembership scans a single membership row.
func scanMembership(row pgx.Row) (*TenantMembership, error) {
	var m TenantMembership
	err := row.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// scanMemberships scans multiple membership rows, reusing scanMembership for
// each row to keep the column list in a single place.
func scanMemberships(rows pgx.Rows) ([]TenantMembership, error) {
	var items []TenantMembership
	for rows.Next() {
		m, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []TenantMembership{}
	}
	return items, nil
}

// AddMembership adds a user to a tenant with the given role.
// Returns ErrMembershipExists if the user is already a member.
func (s *Service) AddMembership(ctx context.Context, tenantID, userID, role string) (*TenantMembership, error) {
	if !IsValidRole(role) {
		return nil, ErrInvalidRole
	}

	m, err := scanMembership(s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_tenant_memberships (tenant_id, user_id, role)
		 VALUES ($1, $2, $3)
		 RETURNING id, tenant_id, user_id, role, created_at`,
		tenantID, userID, role,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrMembershipExists
		}
		return nil, fmt.Errorf("adding membership: %w", err)
	}

	s.logger.Info("membership added", "tenant_id", tenantID, "user_id", userID, "role", role)
	return m, nil
}

// GetMembership retrieves a single membership by tenant and user.
// Returns ErrMembershipNotFound if not found.
func (s *Service) GetMembership(ctx context.Context, tenantID, userID string) (*TenantMembership, error) {
	m, err := scanMembership(s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, user_id, role, created_at
		 FROM _ayb_tenant_memberships
		 WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMembershipNotFound
		}
		return nil, fmt.Errorf("getting membership: %w", err)
	}
	return m, nil
}

// RemoveMembership removes a user from a tenant.
// Returns ErrMembershipNotFound if not found.
func (s *Service) RemoveMembership(ctx context.Context, tenantID, userID string) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_tenant_memberships WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	)
	if err != nil {
		return fmt.Errorf("removing membership: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrMembershipNotFound
	}

	s.logger.Info("membership removed", "tenant_id", tenantID, "user_id", userID)
	return nil
}

// ListMemberships returns all memberships for a tenant.
func (s *Service) ListMemberships(ctx context.Context, tenantID string) ([]TenantMembership, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, user_id, role, created_at
		 FROM _ayb_tenant_memberships
		 WHERE tenant_id = $1
		 ORDER BY created_at ASC`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	defer rows.Close()

	return scanMemberships(rows)
}

// UpdateMembershipRole updates the role of a membership.
// Returns ErrMembershipNotFound if not found.
func (s *Service) UpdateMembershipRole(ctx context.Context, tenantID, userID, role string) (*TenantMembership, error) {
	if !IsValidRole(role) {
		return nil, ErrInvalidRole
	}

	m, err := scanMembership(s.pool.QueryRow(ctx,
		`UPDATE _ayb_tenant_memberships
		 SET role = $3
		 WHERE tenant_id = $1 AND user_id = $2
		 RETURNING id, tenant_id, user_id, role, created_at`,
		tenantID, userID, role,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMembershipNotFound
		}
		return nil, fmt.Errorf("updating membership role: %w", err)
	}

	s.logger.Info("membership role updated", "tenant_id", tenantID, "user_id", userID, "role", role)
	return m, nil
}

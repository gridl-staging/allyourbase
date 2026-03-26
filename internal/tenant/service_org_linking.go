package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// AssignTenantToOrg links a tenant to an organization.
func (s *Service) AssignTenantToOrg(ctx context.Context, tenantID, orgID string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_tenants
		 SET org_id = $2,
		     updated_at = NOW()
		 WHERE id = $1`,
		tenantID,
		orgID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrOrgNotFound
		}
		return fmt.Errorf("assigning tenant to org: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrTenantNotFound
	}

	s.logger.Info("tenant assigned to org", "tenant_id", tenantID, "org_id", orgID)
	return nil
}

// UnassignTenantFromOrg removes an organization link from a tenant only when it
// is still attached to the requested org.
func (s *Service) UnassignTenantFromOrg(ctx context.Context, tenantID, orgID string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_tenants
		 SET org_id = NULL,
		     updated_at = NOW()
		 WHERE id = $1 AND org_id = $2`,
		tenantID,
		orgID,
	)
	if err != nil {
		return fmt.Errorf("unassigning tenant from org: %w", err)
	}
	if result.RowsAffected() == 0 {
		currentTenant, lookupErr := s.GetTenant(ctx, tenantID)
		if lookupErr != nil {
			if errors.Is(lookupErr, ErrTenantNotFound) {
				return ErrTenantNotFound
			}
			return fmt.Errorf("disambiguating tenant org unassign: %w", lookupErr)
		}
		if currentTenant.OrgID == nil || *currentTenant.OrgID != orgID {
			return ErrTenantNotInOrg
		}
		return ErrTenantNotFound
	}

	s.logger.Info("tenant unassigned from org", "tenant_id", tenantID, "org_id", orgID)
	return nil
}

// ListOrgTenants lists tenants linked to an organization.
func (s *Service) ListOrgTenants(ctx context.Context, orgID string) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+tenantColumns+`
		 FROM _ayb_tenants
		 WHERE org_id = $1
		 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing org tenants: %w", err)
	}
	defer rows.Close()

	items := []Tenant{}
	for rows.Next() {
		tenantRecord, scanErr := scanTenant(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scanning org tenant: %w", scanErr)
		}
		items = append(items, *tenantRecord)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating org tenants: %w", err)
	}

	return items, nil
}

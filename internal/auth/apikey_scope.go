package auth

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrOrgScopeUnauthorized indicates an org-scoped key is not permitted for
	// the requested tenant context.
	ErrOrgScopeUnauthorized = errors.New("org-scoped key not authorized for this tenant")
	// ErrOrgScopeLookupFailed indicates tenant->org lookup failed due to an
	// infrastructure/store error.
	ErrOrgScopeLookupFailed = errors.New("org-scoped key tenant lookup failed")
	// ErrOrgScopeMisconfigured indicates middleware called org-scope resolution
	// without a tenant checker dependency.
	ErrOrgScopeMisconfigured = errors.New("org-scoped key tenant checker not configured")
)

// OrgTenantChecker looks up a tenant's org_id to determine whether an
// org-scoped API key is authorized to operate on that tenant.
type OrgTenantChecker interface {
	TenantOrgID(ctx context.Context, tenantID string) (*string, error)
}

// ResolveAPIKeyTenantAccess decides whether the given claims (from an API key)
// permit access to the specified tenant. It handles all three scope types:
//   - Legacy user-scoped keys (no AppID, no OrgID): always allowed
//   - App-scoped keys (AppID set): always allowed (app owns rate-limit, not tenant)
//   - Org-scoped keys (OrgID set): allowed only if the tenant's org_id matches
//
// Returns nil if access is permitted, or an error describing the denial.
func ResolveAPIKeyTenantAccess(ctx context.Context, claims *Claims, tenantID string, checker OrgTenantChecker) error {
	if claims == nil || claims.OrgID == "" {
		return nil // legacy or app-scoped keys pass through
	}

	if tenantID == "" {
		return fmt.Errorf("%w: tenant context required", ErrOrgScopeUnauthorized)
	}
	if checker == nil {
		return ErrOrgScopeMisconfigured
	}

	tenantOrgID, err := checker.TenantOrgID(ctx, tenantID)
	if err != nil {
		return errors.Join(ErrOrgScopeLookupFailed, fmt.Errorf("resolving tenant org: %w", err))
	}

	if tenantOrgID == nil || *tenantOrgID != claims.OrgID {
		return ErrOrgScopeUnauthorized
	}

	return nil
}

// Package server defines tenant middleware composition entry points and tenant administration contracts.
package server

import (
	"context"
	"encoding/json"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

// tenantAdmin is an interface for tenant administration including CRUD operations, membership management, audit event recording, and maintenance state control.
type tenantAdmin interface {
	GetTenant(ctx context.Context, id string) (*tenant.Tenant, error)
	CreateTenant(ctx context.Context, name, slug, isolationMode, planTier, region string, orgMetadata json.RawMessage, idempotencyKey string) (*tenant.Tenant, error)
	DeleteTenantSchema(ctx context.Context, slug string) error
	ListTenants(ctx context.Context, page, perPage int) (*tenant.TenantListResult, error)
	TransitionState(ctx context.Context, id string, fromState, newState tenant.TenantState) (*tenant.Tenant, error)
	UpdateTenant(ctx context.Context, id string, name string, orgMetadata json.RawMessage) (*tenant.Tenant, error)
	AddMembership(ctx context.Context, tenantID, userID, role string) (*tenant.TenantMembership, error)
	RemoveMembership(ctx context.Context, tenantID, userID string) error
	ListMemberships(ctx context.Context, tenantID string) ([]tenant.TenantMembership, error)
	GetMembership(ctx context.Context, tenantID, userID string) (*tenant.TenantMembership, error)
	UpdateMembershipRole(ctx context.Context, tenantID, userID, role string) (*tenant.TenantMembership, error)
	InsertAuditEvent(ctx context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string) error
	IsUnderMaintenance(ctx context.Context, tenantID string) (bool, error)
	EnableMaintenance(ctx context.Context, tenantID, reason, actorID string) (*tenant.TenantMaintenanceState, error)
	DisableMaintenance(ctx context.Context, tenantID, actorID string) (*tenant.TenantMaintenanceState, error)
	GetMaintenanceState(ctx context.Context, tenantID string) (*tenant.TenantMaintenanceState, error)
	AssignTenantToOrg(ctx context.Context, tenantID, orgID string) error
	UnassignTenantFromOrg(ctx context.Context, tenantID, orgID string) error
	ListOrgTenants(ctx context.Context, orgID string) ([]tenant.Tenant, error)
}

// useTenantScopedAccessGuards applies the tenant-scoped authorization chain in
// the required order after request authentication has already succeeded.
func (s *Server) useTenantScopedAccessGuards(r chi.Router) {
	r.Use(s.enforceTenantContext)
	r.Use(s.enforceTenantMatch)
	if s != nil && s.tenantSvc != nil {
		r.Use(s.enforceOrgScopeAccess)
	}
	if s != nil && s.permResolver != nil {
		r.Use(s.requireTenantPermission)
	}
}

// withTenantScopedAdminOrUserAuth mounts routes behind the standard
// admin-or-user authentication and tenant-scoped authorization chain.
func (s *Server) withTenantScopedAdminOrUserAuth(r chi.Router, register func(chi.Router)) {
	r.Group(func(r chi.Router) {
		// Accept either a valid admin HMAC token or a user JWT/API-key.
		r.Use(s.requireAdminOrUserAuth(s.authSvc))
		// Apply tenant-scoped auth guards whenever any tenant wiring is enabled.
		// Baseline guards (tenant context + tenant match) should still enforce
		// tenant isolation for partially wired servers, while full guards are
		// enabled conditionally in useTenantScopedAccessGuards.
		if s != nil && (s.tenantSvc != nil || s.permResolver != nil) {
			s.useTenantScopedAccessGuards(r)
		}
		register(r)
	})
}

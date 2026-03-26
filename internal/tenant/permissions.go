// Package tenant Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/tenant/permissions.go.
package tenant

import (
	"context"
	"errors"
	"fmt"
)

// ResolvedPermission describes effective access to a tenant.
type ResolvedPermission struct {
	EffectiveRole string
	Source        string
	SourceID      string
}

type resolvedPermissionCtxKey struct{}

// ContextWithPermission returns a copy of ctx with resolved tenant permissions.
func ContextWithPermission(ctx context.Context, permission *ResolvedPermission) context.Context {
	return context.WithValue(ctx, resolvedPermissionCtxKey{}, permission)
}

// PermissionFromContext extracts resolved tenant permissions from context.
func PermissionFromContext(ctx context.Context) *ResolvedPermission {
	permission, _ := ctx.Value(resolvedPermissionCtxKey{}).(*ResolvedPermission)
	return permission
}

type tenantPermissionSource interface {
	GetMembership(ctx context.Context, tenantID, userID string) (*TenantMembership, error)
	GetTenant(ctx context.Context, id string) (*Tenant, error)
}

// PermissionResolver resolves direct and inherited tenant permissions.
type PermissionResolver struct {
	tenantService       tenantPermissionSource
	orgMembershipStore  OrgMembershipStore
	teamMembershipStore TeamMembershipStore
	teamStore           TeamStore
}

func NewPermissionResolver(
	tenantService tenantPermissionSource,
	orgMembershipStore OrgMembershipStore,
	teamMembershipStore TeamMembershipStore,
	teamStore TeamStore,
) *PermissionResolver {
	return &PermissionResolver{
		tenantService:       tenantService,
		orgMembershipStore:  orgMembershipStore,
		teamMembershipStore: teamMembershipStore,
		teamStore:           teamStore,
	}
}

// ResolvePermissions determines the user's effective tenant role.
func (r *PermissionResolver) ResolvePermissions(ctx context.Context, userID, tenantID string) (*ResolvedPermission, error) {
	directMembership, err := r.tenantService.GetMembership(ctx, tenantID, userID)
	if err == nil {
		return &ResolvedPermission{
			EffectiveRole: directMembership.Role,
			Source:        "direct",
			SourceID:      tenantID,
		}, nil
	}
	if !errors.Is(err, ErrMembershipNotFound) {
		return nil, fmt.Errorf("resolving direct tenant membership: %w", err)
	}

	tenantRecord, err := r.tenantService.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("resolving tenant for permissions: %w", err)
	}
	if tenantRecord.OrgID == nil {
		return nil, nil
	}
	orgID := *tenantRecord.OrgID

	bestPermission, err := r.resolveOrgPermission(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}

	bestPermission, err = r.resolveTeamPermission(ctx, orgID, userID, bestPermission)
	if err != nil {
		return nil, err
	}

	return bestPermission, nil
}

// TODO: Document PermissionResolver.resolveOrgPermission.
func (r *PermissionResolver) resolveOrgPermission(ctx context.Context, orgID, userID string) (*ResolvedPermission, error) {
	orgMembership, err := r.orgMembershipStore.GetOrgMembership(ctx, orgID, userID)
	if errors.Is(err, ErrOrgMembershipNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolving org membership: %w", err)
	}

	effectiveRole, ok := mapOrgRoleToEffectiveRole(orgMembership.Role)
	if !ok {
		return nil, fmt.Errorf("resolving org membership: unsupported role %q", orgMembership.Role)
	}

	return &ResolvedPermission{
		EffectiveRole: effectiveRole,
		Source:        "org",
		SourceID:      orgID,
	}, nil
}

// TODO: Document PermissionResolver.resolveTeamPermission.
func (r *PermissionResolver) resolveTeamPermission(
	ctx context.Context,
	orgID string,
	userID string,
	currentBest *ResolvedPermission,
) (*ResolvedPermission, error) {
	teams, err := r.teamStore.ListTeams(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing org teams for permission resolution: %w", err)
	}

	best := currentBest
	for _, team := range teams {
		teamMembership, membershipErr := r.teamMembershipStore.GetTeamMembership(ctx, team.ID, userID)
		if errors.Is(membershipErr, ErrTeamMembershipNotFound) {
			continue
		}
		if membershipErr != nil {
			return nil, fmt.Errorf("resolving team membership for team %s: %w", team.ID, membershipErr)
		}

		effectiveRole, ok := mapTeamRoleToEffectiveRole(teamMembership.Role)
		if !ok {
			return nil, fmt.Errorf("resolving team membership: unsupported role %q", teamMembership.Role)
		}

		candidate := &ResolvedPermission{
			EffectiveRole: effectiveRole,
			Source:        "team",
			SourceID:      team.ID,
		}
		if best == nil || roleRank(candidate.EffectiveRole) > roleRank(best.EffectiveRole) {
			best = candidate
		}
	}

	return best, nil
}

func mapOrgRoleToEffectiveRole(role string) (string, bool) {
	switch role {
	case OrgRoleOwner, OrgRoleAdmin:
		return MemberRoleAdmin, true
	case OrgRoleMember, OrgRoleViewer:
		return MemberRoleViewer, true
	default:
		return "", false
	}
}

func mapTeamRoleToEffectiveRole(role string) (string, bool) {
	switch role {
	case TeamRoleLead:
		return MemberRoleMember, true
	case TeamRoleMember:
		return MemberRoleViewer, true
	default:
		return "", false
	}
}

func roleRank(role string) int {
	switch role {
	case MemberRoleOwner:
		return 4
	case MemberRoleAdmin:
		return 3
	case MemberRoleMember:
		return 2
	case MemberRoleViewer:
		return 1
	default:
		return 0
	}
}

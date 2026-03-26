//go:build integration

package tenant

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestPermissionResolver() *PermissionResolver {
	tenantService := newTestService()
	orgMembershipStore := newTestOrgMembershipStore()
	teamMembershipStore := newTestTeamMembershipStore()
	teamStore := newTestTeamStore()
	return NewPermissionResolver(tenantService, orgMembershipStore, teamMembershipStore, teamStore)
}

func createPermissionTenantInOrg(t *testing.T, orgSlug, tenantSlug string) (*Organization, *Tenant) {
	t.Helper()
	ctx := context.Background()
	orgStore := newTestOrgStore()
	tenantService := newTestService()

	org, err := orgStore.CreateOrg(ctx, "Permission Org "+orgSlug, orgSlug, nil, "free")
	testutil.NoError(t, err)
	tenantRecord, err := tenantService.CreateTenant(ctx, "Tenant "+tenantSlug, tenantSlug, "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	testutil.NoError(t, tenantService.AssignTenantToOrg(ctx, tenantRecord.ID, org.ID))
	return org, tenantRecord
}

func TestResolvePermissions_DirectMembershipWins(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	resolver := newTestPermissionResolver()
	tenantService := newTestService()
	orgMembershipStore := newTestOrgMembershipStore()
	teamMembershipStore := newTestTeamMembershipStore()
	teamStore := newTestTeamStore()

	org, tenantRecord := createPermissionTenantInOrg(t, "direct-wins-org", "direct-wins-tenant")
	team, err := teamStore.CreateTeam(ctx, org.ID, "Direct Wins Team", "direct-wins-team")
	testutil.NoError(t, err)

	userID := insertTestUser(t, "direct-wins@example.com")
	_, err = orgMembershipStore.AddOrgMembership(ctx, org.ID, userID, OrgRoleOwner)
	testutil.NoError(t, err)
	_, err = teamMembershipStore.AddTeamMembership(ctx, team.ID, userID, TeamRoleLead)
	testutil.NoError(t, err)
	_, err = tenantService.AddMembership(ctx, tenantRecord.ID, userID, MemberRoleViewer)
	testutil.NoError(t, err)

	resolved, err := resolver.ResolvePermissions(ctx, userID, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, resolved)
	testutil.Equal(t, MemberRoleViewer, resolved.EffectiveRole)
	testutil.Equal(t, "direct", resolved.Source)
	testutil.Equal(t, tenantRecord.ID, resolved.SourceID)
}

func TestResolvePermissions_OrgInheritanceMappings(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	resolver := newTestPermissionResolver()
	orgMembershipStore := newTestOrgMembershipStore()

	org, tenantRecord := createPermissionTenantInOrg(t, "org-map-org", "org-map-tenant")
	tests := []struct {
		name          string
		email         string
		orgRole       string
		effectiveRole string
	}{
		{name: "owner maps to admin", email: "org-owner@example.com", orgRole: OrgRoleOwner, effectiveRole: MemberRoleAdmin},
		{name: "admin maps to admin", email: "org-admin@example.com", orgRole: OrgRoleAdmin, effectiveRole: MemberRoleAdmin},
		{name: "member maps to viewer", email: "org-member@example.com", orgRole: OrgRoleMember, effectiveRole: MemberRoleViewer},
		{name: "viewer maps to viewer", email: "org-viewer@example.com", orgRole: OrgRoleViewer, effectiveRole: MemberRoleViewer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := insertTestUser(t, tt.email)
			_, err := orgMembershipStore.AddOrgMembership(ctx, org.ID, userID, tt.orgRole)
			testutil.NoError(t, err)

			resolved, err := resolver.ResolvePermissions(ctx, userID, tenantRecord.ID)
			testutil.NoError(t, err)
			testutil.NotNil(t, resolved)
			testutil.Equal(t, tt.effectiveRole, resolved.EffectiveRole)
			testutil.Equal(t, "org", resolved.Source)
			testutil.Equal(t, org.ID, resolved.SourceID)
		})
	}
}

func TestResolvePermissions_TeamInheritanceMappings(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	resolver := newTestPermissionResolver()
	teamMembershipStore := newTestTeamMembershipStore()
	teamStore := newTestTeamStore()

	org, tenantRecord := createPermissionTenantInOrg(t, "team-map-org", "team-map-tenant")
	team, err := teamStore.CreateTeam(ctx, org.ID, "Team Mapping", "team-mapping")
	testutil.NoError(t, err)

	leadUser := insertTestUser(t, "team-lead-map@example.com")
	_, err = teamMembershipStore.AddTeamMembership(ctx, team.ID, leadUser, TeamRoleLead)
	testutil.NoError(t, err)
	leadResolved, err := resolver.ResolvePermissions(ctx, leadUser, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, leadResolved)
	testutil.Equal(t, MemberRoleMember, leadResolved.EffectiveRole)
	testutil.Equal(t, "team", leadResolved.Source)
	testutil.Equal(t, team.ID, leadResolved.SourceID)

	memberUser := insertTestUser(t, "team-member-map@example.com")
	_, err = teamMembershipStore.AddTeamMembership(ctx, team.ID, memberUser, TeamRoleMember)
	testutil.NoError(t, err)
	memberResolved, err := resolver.ResolvePermissions(ctx, memberUser, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, memberResolved)
	testutil.Equal(t, MemberRoleViewer, memberResolved.EffectiveRole)
	testutil.Equal(t, "team", memberResolved.Source)
	testutil.Equal(t, team.ID, memberResolved.SourceID)
}

func TestResolvePermissions_HighestPrivilegeWinsAcrossOrgAndTeam(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	resolver := newTestPermissionResolver()
	orgMembershipStore := newTestOrgMembershipStore()
	teamMembershipStore := newTestTeamMembershipStore()
	teamStore := newTestTeamStore()

	org, tenantRecord := createPermissionTenantInOrg(t, "highest-org", "highest-tenant")
	team, err := teamStore.CreateTeam(ctx, org.ID, "Highest Team", "highest-team")
	testutil.NoError(t, err)

	teamHigherUser := insertTestUser(t, "team-higher@example.com")
	_, err = orgMembershipStore.AddOrgMembership(ctx, org.ID, teamHigherUser, OrgRoleViewer)
	testutil.NoError(t, err)
	_, err = teamMembershipStore.AddTeamMembership(ctx, team.ID, teamHigherUser, TeamRoleLead)
	testutil.NoError(t, err)

	teamHigherResolved, err := resolver.ResolvePermissions(ctx, teamHigherUser, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, teamHigherResolved)
	testutil.Equal(t, MemberRoleMember, teamHigherResolved.EffectiveRole)
	testutil.Equal(t, "team", teamHigherResolved.Source)

	orgHigherUser := insertTestUser(t, "org-higher@example.com")
	_, err = orgMembershipStore.AddOrgMembership(ctx, org.ID, orgHigherUser, OrgRoleAdmin)
	testutil.NoError(t, err)
	_, err = teamMembershipStore.AddTeamMembership(ctx, team.ID, orgHigherUser, TeamRoleMember)
	testutil.NoError(t, err)

	orgHigherResolved, err := resolver.ResolvePermissions(ctx, orgHigherUser, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, orgHigherResolved)
	testutil.Equal(t, MemberRoleAdmin, orgHigherResolved.EffectiveRole)
	testutil.Equal(t, "org", orgHigherResolved.Source)
}

func TestResolvePermissions_NoAccessCasesReturnNil(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	resolver := newTestPermissionResolver()
	tenantService := newTestService()

	org, tenantInOrg := createPermissionTenantInOrg(t, "no-access-org", "no-access-tenant")
	_ = org
	userID := insertTestUser(t, "no-access-user@example.com")

	resolved, err := resolver.ResolvePermissions(ctx, userID, tenantInOrg.ID)
	testutil.NoError(t, err)
	testutil.Nil(t, resolved)

	tenantWithoutOrg, err := tenantService.CreateTenant(ctx, "No Org Tenant", "no-org-tenant", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	resolved, err = resolver.ResolvePermissions(ctx, userID, tenantWithoutOrg.ID)
	testutil.NoError(t, err)
	testutil.Nil(t, resolved)
}

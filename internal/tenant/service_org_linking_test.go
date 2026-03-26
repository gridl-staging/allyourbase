//go:build integration

package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAssignTenantToOrgAndUnassignTenantFromOrg(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	tenantService := newTestService()
	orgStore := newTestOrgStore()

	org, err := orgStore.CreateOrg(ctx, "Linked Org", "linked-org", nil, "free")
	testutil.NoError(t, err)
	tenantRecord, err := tenantService.CreateTenant(ctx, "Linked Tenant", "linked-tenant", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	testutil.Nil(t, tenantRecord.OrgID)

	err = tenantService.AssignTenantToOrg(ctx, tenantRecord.ID, org.ID)
	testutil.NoError(t, err)

	updated, err := tenantService.GetTenant(ctx, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, updated.OrgID)
	testutil.Equal(t, org.ID, *updated.OrgID)

	err = tenantService.UnassignTenantFromOrg(ctx, tenantRecord.ID, org.ID)
	testutil.NoError(t, err)

	unassigned, err := tenantService.GetTenant(ctx, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.Nil(t, unassigned.OrgID)
}

func TestAssignTenantToOrgRejectsUnknownOrg(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	tenantService := newTestService()

	tenantRecord, err := tenantService.CreateTenant(ctx, "Unknown Org Tenant", "unknown-org-tenant", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	err = tenantService.AssignTenantToOrg(ctx, tenantRecord.ID, "00000000-0000-0000-0000-000000000000")
	testutil.True(t, errors.Is(err, ErrOrgNotFound))
}

func TestAssignTenantToOrgAndUnassignTenantFromOrgNotFound(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	tenantService := newTestService()

	err := tenantService.AssignTenantToOrg(ctx, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000")
	testutil.True(t, errors.Is(err, ErrTenantNotFound))

	err = tenantService.UnassignTenantFromOrg(ctx, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000")
	testutil.True(t, errors.Is(err, ErrTenantNotFound))
}

func TestUnassignTenantFromOrgRejectsOrgMismatch(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	tenantService := newTestService()
	orgStore := newTestOrgStore()

	orgOne, err := orgStore.CreateOrg(ctx, "Org One", "unassign-org-one", nil, "free")
	testutil.NoError(t, err)
	orgTwo, err := orgStore.CreateOrg(ctx, "Org Two", "unassign-org-two", nil, "free")
	testutil.NoError(t, err)
	tenantRecord, err := tenantService.CreateTenant(ctx, "Linked Tenant", "linked-tenant-mismatch", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	testutil.NoError(t, tenantService.AssignTenantToOrg(ctx, tenantRecord.ID, orgOne.ID))

	err = tenantService.UnassignTenantFromOrg(ctx, tenantRecord.ID, orgTwo.ID)
	testutil.True(t, errors.Is(err, ErrTenantNotInOrg))

	linkedTenant, err := tenantService.GetTenant(ctx, tenantRecord.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, linkedTenant.OrgID)
	testutil.Equal(t, orgOne.ID, *linkedTenant.OrgID)
}

func TestListOrgTenants(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	tenantService := newTestService()
	orgStore := newTestOrgStore()

	orgOne, err := orgStore.CreateOrg(ctx, "List Org One", "list-org-one", nil, "free")
	testutil.NoError(t, err)
	orgTwo, err := orgStore.CreateOrg(ctx, "List Org Two", "list-org-two", nil, "free")
	testutil.NoError(t, err)

	tenantOne, err := tenantService.CreateTenant(ctx, "Tenant One", "tenant-list-one", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	tenantTwo, err := tenantService.CreateTenant(ctx, "Tenant Two", "tenant-list-two", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	tenantThree, err := tenantService.CreateTenant(ctx, "Tenant Three", "tenant-list-three", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	testutil.NoError(t, tenantService.AssignTenantToOrg(ctx, tenantOne.ID, orgOne.ID))
	testutil.NoError(t, tenantService.AssignTenantToOrg(ctx, tenantTwo.ID, orgOne.ID))
	testutil.NoError(t, tenantService.AssignTenantToOrg(ctx, tenantThree.ID, orgTwo.ID))

	orgOneTenants, err := tenantService.ListOrgTenants(ctx, orgOne.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, orgOneTenants, 2)
	orgOneIDs := map[string]bool{orgOneTenants[0].ID: true, orgOneTenants[1].ID: true}
	testutil.True(t, orgOneIDs[tenantOne.ID])
	testutil.True(t, orgOneIDs[tenantTwo.ID])

	orgTwoTenants, err := tenantService.ListOrgTenants(ctx, orgTwo.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, orgTwoTenants, 1)
	testutil.Equal(t, tenantThree.ID, orgTwoTenants[0].ID)
}

//go:build integration

package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestOrgStore() *PostgresOrgStore {
	return NewPostgresOrgStore(sharedPG.Pool, testutil.DiscardLogger())
}

func newTestTeamStore() *PostgresTeamStore {
	return NewPostgresTeamStore(sharedPG.Pool, testutil.DiscardLogger())
}

func ptrToString(value string) *string {
	return &value
}

func TestOrgStoreCRUD(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgStore()

	created, err := store.CreateOrg(ctx, "Acme", "acme", nil, "free")
	testutil.NoError(t, err)
	testutil.NotNil(t, created)
	testutil.Equal(t, "acme", created.Slug)
	testutil.Equal(t, "Acme", created.Name)
	testutil.Equal(t, "free", created.PlanTier)

	loaded, err := store.GetOrg(ctx, created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, loaded.ID)
	testutil.Equal(t, "Acme", loaded.Name)

	bySlug, err := store.GetOrgBySlug(ctx, "acme")
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, bySlug.ID)

	list, err := store.ListOrgs(ctx, "")
	testutil.NoError(t, err)
	testutil.SliceLen(t, list, 1)
	testutil.Equal(t, created.ID, list[0].ID)

	updated, err := store.UpdateOrg(ctx, created.ID, OrgUpdate{Name: ptrToString("Acme Corp")})
	testutil.NoError(t, err)
	testutil.Equal(t, "Acme Corp", updated.Name)
	testutil.Equal(t, created.ID, updated.ID)

	// Delete and confirm it's gone.
	err = store.DeleteOrg(ctx, created.ID)
	testutil.NoError(t, err)

	_, err = store.GetOrg(ctx, created.ID)
	testutil.True(t, errors.Is(err, ErrOrgNotFound))
}

func TestOrgStoreListByUserAndChildren(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgStore()

	parent, err := store.CreateOrg(ctx, "Parent Org", "parent-org", nil, "free")
	testutil.NoError(t, err)

	child, err := store.CreateOrg(ctx, "Child Org", "child-org", ptrToString(parent.ID), "free")
	testutil.NoError(t, err)

	memberUser := insertTestUser(t, "member@org.test")
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_org_memberships (org_id, user_id, role) VALUES ($1, $2, $3)`,
		parent.ID,
		memberUser,
		OrgRoleAdmin,
	)
	testutil.NoError(t, err)

	listByUser, err := store.ListOrgs(ctx, memberUser)
	testutil.NoError(t, err)
	testutil.SliceLen(t, listByUser, 1)
	testutil.Equal(t, parent.ID, listByUser[0].ID)

	childOrgs, err := store.ListChildOrgs(ctx, parent.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, childOrgs, 1)
	testutil.Equal(t, child.ID, childOrgs[0].ID)
}

func TestOrgStoreRejectsInvalidSlug(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgStore()

	testLongSlug := strings.Repeat("a", 64)
	invalidSlugs := []string{
		"A",
		"ab$",
		"-bad",
		"bad-",
		"ab cd",
		"a",
		"a-",
		"-a",
		testLongSlug,
	}
	for _, slug := range invalidSlugs {
		_, err := store.CreateOrg(ctx, "Test Org", slug, nil, "free")
		testutil.True(t, errors.Is(err, ErrInvalidSlug))
	}
}

func TestOrgStorePreventsCircularParent(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgStore()

	parent, err := store.CreateOrg(ctx, "Parent", "parent", nil, "free")
	testutil.NoError(t, err)
	child, err := store.CreateOrg(ctx, "Child", "child", ptrToString(parent.ID), "free")
	testutil.NoError(t, err)

	_, err = store.UpdateOrg(ctx, parent.ID, OrgUpdate{ParentOrgID: ptrToString(child.ID)})
	testutil.True(t, errors.Is(err, ErrCircularParentOrg))
}

func TestOrgStorePreventsCircularParentAcrossDeepHierarchy(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgStore()

	root, err := store.CreateOrg(ctx, "Root", "root", nil, "free")
	testutil.NoError(t, err)

	parentID := root.ID
	var deepestID string
	for depth := 1; depth <= 11; depth++ {
		org, createErr := store.CreateOrg(
			ctx,
			fmt.Sprintf("Level %d", depth),
			fmt.Sprintf("level-%02d", depth),
			ptrToString(parentID),
			"free",
		)
		testutil.NoError(t, createErr)
		parentID = org.ID
		deepestID = org.ID
	}

	_, err = store.UpdateOrg(ctx, root.ID, OrgUpdate{ParentOrgID: ptrToString(deepestID)})
	testutil.True(t, errors.Is(err, ErrCircularParentOrg))
}

func TestOrgStoreRejectsUnknownParentOnCreate(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgStore()

	_, err := store.CreateOrg(ctx, "Child", "child-missing-parent", ptrToString("00000000-0000-0000-0000-000000000000"), "free")
	testutil.True(t, errors.Is(err, ErrParentOrgNotFound))
}

func TestOrgStoreRejectsUnknownParentOnUpdate(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgStore()

	org, err := store.CreateOrg(ctx, "Standalone", "standalone-org", nil, "free")
	testutil.NoError(t, err)

	_, err = store.UpdateOrg(ctx, org.ID, OrgUpdate{ParentOrgID: ptrToString("00000000-0000-0000-0000-000000000000")})
	testutil.True(t, errors.Is(err, ErrParentOrgNotFound))
}

func TestTeamStoreCRUD(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	teamStore := newTestTeamStore()

	org, err := orgStore.CreateOrg(ctx, "Engineering", "engineering", nil, "free")
	testutil.NoError(t, err)

	team, err := teamStore.CreateTeam(ctx, org.ID, "Core Team", "core")
	testutil.NoError(t, err)
	testutil.Equal(t, "Core Team", team.Name)
	testutil.Equal(t, "core", team.Slug)

	got, err := teamStore.GetTeam(ctx, team.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, team.ID, got.ID)

	list, err := teamStore.ListTeams(ctx, org.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, list, 1)

	updated, err := teamStore.UpdateTeam(ctx, team.ID, TeamUpdate{Name: ptrToString("Infra Team")})
	testutil.NoError(t, err)
	testutil.Equal(t, "Infra Team", updated.Name)

	err = teamStore.DeleteTeam(ctx, team.ID)
	testutil.NoError(t, err)

	_, err = teamStore.GetTeam(ctx, team.ID)
	testutil.True(t, errors.Is(err, ErrTeamNotFound))
}

func TestTeamStoreSlugUniquePerOrg(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	teamStore := newTestTeamStore()

	orgOne, err := orgStore.CreateOrg(ctx, "Org One", "team-org-one", nil, "free")
	testutil.NoError(t, err)
	orgTwo, err := orgStore.CreateOrg(ctx, "Org Two", "team-org-two", nil, "free")
	testutil.NoError(t, err)

	_, err = teamStore.CreateTeam(ctx, orgOne.ID, "Operations", "ops")
	testutil.NoError(t, err)
	_, err = teamStore.CreateTeam(ctx, orgOne.ID, "DevOps", "ops")
	testutil.True(t, errors.Is(err, ErrTeamSlugTaken))

	otherOrgTeam, err := teamStore.CreateTeam(ctx, orgTwo.ID, "Operations", "ops")
	testutil.NoError(t, err)
	testutil.NotNil(t, otherOrgTeam)

	ordered, err := teamStore.ListTeams(ctx, orgOne.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, ordered, 1)
	testutil.Equal(t, "Operations", ordered[0].Name)

	// Confirm sorted order in a multi-row case.
	_, err = teamStore.CreateTeam(ctx, orgOne.ID, "Beta", "beta")
	testutil.NoError(t, err)
	_, err = teamStore.CreateTeam(ctx, orgOne.ID, "Alpha", "alpha")
	testutil.NoError(t, err)

	ordered, err = teamStore.ListTeams(ctx, orgOne.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, ordered, 3)
	testutil.Equal(t, "Alpha", ordered[0].Name)
	testutil.Equal(t, "Beta", ordered[1].Name)
	testutil.Equal(t, "Operations", ordered[2].Name)
}

func TestTeamStoreRejectsUnknownOrgID(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	teamStore := newTestTeamStore()

	_, err := teamStore.CreateTeam(ctx, "00000000-0000-0000-0000-000000000000", "Ghost Team", "ghost-team")
	testutil.True(t, errors.Is(err, ErrOrgNotFound))
	testutil.True(t, !errors.Is(err, ErrTeamSlugTaken))
}

func TestTeamStoreListTeamsMissingOrgReturnsOrgNotFound(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	teamStore := newTestTeamStore()

	_, err := teamStore.ListTeams(ctx, "00000000-0000-0000-0000-000000000000")
	testutil.True(t, errors.Is(err, ErrOrgNotFound))
}

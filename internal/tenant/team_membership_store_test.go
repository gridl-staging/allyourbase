//go:build integration

package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestTeamMembershipStore() *PostgresTeamMembershipStore {
	return NewPostgresTeamMembershipStore(sharedPG.Pool, testutil.DiscardLogger())
}

func TestTeamMembershipStoreCRUDAndLists(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	teamStore := newTestTeamStore()
	store := newTestTeamMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Team Org", "team-membership-org", nil, "free")
	testutil.NoError(t, err)
	team, err := teamStore.CreateTeam(ctx, org.ID, "Platform", "platform")
	testutil.NoError(t, err)

	userID := insertTestUser(t, "team-member@example.com")
	membership, err := store.AddTeamMembership(ctx, team.ID, userID, TeamRoleMember)
	testutil.NoError(t, err)
	testutil.Equal(t, team.ID, membership.TeamID)
	testutil.Equal(t, userID, membership.UserID)
	testutil.Equal(t, TeamRoleMember, membership.Role)

	fetched, err := store.GetTeamMembership(ctx, team.ID, userID)
	testutil.NoError(t, err)
	testutil.Equal(t, membership.ID, fetched.ID)

	byTeam, err := store.ListTeamMemberships(ctx, team.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, byTeam, 1)
	testutil.Equal(t, membership.ID, byTeam[0].ID)

	byUser, err := store.ListUserTeamMemberships(ctx, userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, byUser, 1)
	testutil.Equal(t, membership.ID, byUser[0].ID)

	updated, err := store.UpdateTeamMembershipRole(ctx, team.ID, userID, TeamRoleLead)
	testutil.NoError(t, err)
	testutil.Equal(t, TeamRoleLead, updated.Role)

	err = store.RemoveTeamMembership(ctx, team.ID, userID)
	testutil.NoError(t, err)

	_, err = store.GetTeamMembership(ctx, team.ID, userID)
	testutil.True(t, errors.Is(err, ErrTeamMembershipNotFound))
}

func TestTeamMembershipStorePreventsDuplicateMemberships(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	teamStore := newTestTeamStore()
	store := newTestTeamMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Team Dup Org", "team-dup-org", nil, "free")
	testutil.NoError(t, err)
	team, err := teamStore.CreateTeam(ctx, org.ID, "Operations", "operations")
	testutil.NoError(t, err)
	userID := insertTestUser(t, "team-dup@example.com")

	_, err = store.AddTeamMembership(ctx, team.ID, userID, TeamRoleMember)
	testutil.NoError(t, err)

	_, err = store.AddTeamMembership(ctx, team.ID, userID, TeamRoleMember)
	testutil.True(t, errors.Is(err, ErrTeamMembershipExists))
}

func TestTeamMembershipStoreRejectsUnknownTeamID(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestTeamMembershipStore()
	userID := insertTestUser(t, "team-fk@example.com")

	_, err := store.AddTeamMembership(ctx, "00000000-0000-0000-0000-000000000000", userID, TeamRoleMember)
	testutil.True(t, errors.Is(err, ErrTeamNotFound))
}

func TestTeamMembershipStoreMissingTeamReturnsTeamNotFound(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestTeamMembershipStore()
	missingTeamID := "00000000-0000-0000-0000-000000000000"
	userID := insertTestUser(t, "team-missing@example.com")

	_, err := store.ListTeamMemberships(ctx, missingTeamID)
	testutil.True(t, errors.Is(err, ErrTeamNotFound))

	_, err = store.UpdateTeamMembershipRole(ctx, missingTeamID, userID, TeamRoleLead)
	testutil.True(t, errors.Is(err, ErrTeamNotFound))

	err = store.RemoveTeamMembership(ctx, missingTeamID, userID)
	testutil.True(t, errors.Is(err, ErrTeamNotFound))
}

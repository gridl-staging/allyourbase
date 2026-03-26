//go:build integration

package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestOrgMembershipStore() *PostgresOrgMembershipStore {
	return NewPostgresOrgMembershipStore(sharedPG.Pool, testutil.DiscardLogger())
}

func TestOrgMembershipStoreCRUDAndLists(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	store := newTestOrgMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Acme Org", "acme-org", nil, "free")
	testutil.NoError(t, err)

	userID := insertTestUser(t, "member-org-store@example.com")
	membership, err := store.AddOrgMembership(ctx, org.ID, userID, OrgRoleMember)
	testutil.NoError(t, err)
	testutil.Equal(t, org.ID, membership.OrgID)
	testutil.Equal(t, userID, membership.UserID)
	testutil.Equal(t, OrgRoleMember, membership.Role)

	fetched, err := store.GetOrgMembership(ctx, org.ID, userID)
	testutil.NoError(t, err)
	testutil.Equal(t, membership.ID, fetched.ID)

	byOrg, err := store.ListOrgMemberships(ctx, org.ID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, byOrg, 1)
	testutil.Equal(t, membership.ID, byOrg[0].ID)

	byUser, err := store.ListUserOrgMemberships(ctx, userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, byUser, 1)
	testutil.Equal(t, membership.ID, byUser[0].ID)

	updated, err := store.UpdateOrgMembershipRole(ctx, org.ID, userID, OrgRoleAdmin)
	testutil.NoError(t, err)
	testutil.Equal(t, OrgRoleAdmin, updated.Role)

	err = store.RemoveOrgMembership(ctx, org.ID, userID)
	testutil.NoError(t, err)

	_, err = store.GetOrgMembership(ctx, org.ID, userID)
	testutil.True(t, errors.Is(err, ErrOrgMembershipNotFound))
}

func TestOrgMembershipStorePreventsDuplicateMemberships(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	store := newTestOrgMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Acme Duplicate", "acme-org-dup", nil, "free")
	testutil.NoError(t, err)
	userID := insertTestUser(t, "duplicate-org-membership@example.com")

	_, err = store.AddOrgMembership(ctx, org.ID, userID, OrgRoleViewer)
	testutil.NoError(t, err)

	_, err = store.AddOrgMembership(ctx, org.ID, userID, OrgRoleViewer)
	testutil.True(t, errors.Is(err, ErrOrgMembershipExists))
}

func TestOrgMembershipStoreRejectsUnknownReferences(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgMembershipStore()
	orgStore := newTestOrgStore()

	userID := insertTestUser(t, "fk-org-membership@example.com")
	_, err := store.AddOrgMembership(ctx, "00000000-0000-0000-0000-000000000000", userID, OrgRoleViewer)
	testutil.True(t, errors.Is(err, ErrOrgNotFound))

	org, err := orgStore.CreateOrg(ctx, "Valid Org", "valid-org-membership-ref", nil, "free")
	testutil.NoError(t, err)

	_, err = store.AddOrgMembership(ctx, org.ID, "00000000-0000-0000-0000-000000000000", OrgRoleViewer)
	testutil.ErrorContains(t, err, "invalid org membership reference")
	testutil.True(t, !errors.Is(err, ErrOrgMembershipExists))
}

func TestOrgMembershipStoreListRejectsUnknownOrg(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgMembershipStore()

	_, err := store.ListOrgMemberships(ctx, "00000000-0000-0000-0000-000000000000")
	testutil.True(t, errors.Is(err, ErrOrgNotFound))
}

func TestOrgMembershipStoreUpdateRejectsUnknownOrg(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgMembershipStore()

	_, err := store.UpdateOrgMembershipRole(ctx, "00000000-0000-0000-0000-000000000000", insertTestUser(t, "missing-org-update@example.com"), OrgRoleAdmin)
	testutil.True(t, errors.Is(err, ErrOrgNotFound))
}

func TestOrgMembershipStoreRemoveRejectsUnknownOrg(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	store := newTestOrgMembershipStore()

	err := store.RemoveOrgMembership(ctx, "00000000-0000-0000-0000-000000000000", insertTestUser(t, "missing-org-remove@example.com"))
	testutil.True(t, errors.Is(err, ErrOrgNotFound))
}

func TestOrgMembershipStoreBlocksRemovingLastOwner(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	store := newTestOrgMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Owner Guard", "owner-guard", nil, "free")
	testutil.NoError(t, err)
	ownerOne := insertTestUser(t, "owner-guard-1@example.com")
	ownerTwo := insertTestUser(t, "owner-guard-2@example.com")

	_, err = store.AddOrgMembership(ctx, org.ID, ownerOne, OrgRoleOwner)
	testutil.NoError(t, err)

	err = store.RemoveOrgMembership(ctx, org.ID, ownerOne)
	testutil.True(t, errors.Is(err, ErrLastOwner))

	_, err = store.AddOrgMembership(ctx, org.ID, ownerTwo, OrgRoleOwner)
	testutil.NoError(t, err)

	err = store.RemoveOrgMembership(ctx, org.ID, ownerOne)
	testutil.NoError(t, err)
}

func TestOrgMembershipStoreBlocksDemotingLastOwner(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	orgStore := newTestOrgStore()
	store := newTestOrgMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Demotion Guard", "demotion-guard", nil, "free")
	testutil.NoError(t, err)
	ownerOne := insertTestUser(t, "owner-demote-1@example.com")
	ownerTwo := insertTestUser(t, "owner-demote-2@example.com")

	_, err = store.AddOrgMembership(ctx, org.ID, ownerOne, OrgRoleOwner)
	testutil.NoError(t, err)

	_, err = store.UpdateOrgMembershipRole(ctx, org.ID, ownerOne, OrgRoleAdmin)
	testutil.True(t, errors.Is(err, ErrLastOwner))

	_, err = store.AddOrgMembership(ctx, org.ID, ownerTwo, OrgRoleOwner)
	testutil.NoError(t, err)

	updated, err := store.UpdateOrgMembershipRole(ctx, org.ID, ownerOne, OrgRoleAdmin)
	testutil.NoError(t, err)
	testutil.Equal(t, OrgRoleAdmin, updated.Role)
}

func TestOrgMembershipStoreConcurrentOwnerRemovalReturnsLastOwner(t *testing.T) {
	setupTenantTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orgStore := newTestOrgStore()
	store := newTestOrgMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Concurrent Owners", "concurrent-owners", nil, "free")
	testutil.NoError(t, err)
	ownerOne := insertTestUser(t, "concurrent-owner-1@example.com")
	ownerTwo := insertTestUser(t, "concurrent-owner-2@example.com")

	_, err = store.AddOrgMembership(ctx, org.ID, ownerOne, OrgRoleOwner)
	testutil.NoError(t, err)
	_, err = store.AddOrgMembership(ctx, org.ID, ownerTwo, OrgRoleOwner)
	testutil.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)

	go func() {
		<-start
		results <- store.RemoveOrgMembership(ctx, org.ID, ownerOne)
	}()
	go func() {
		<-start
		results <- store.RemoveOrgMembership(ctx, org.ID, ownerTwo)
	}()

	close(start)

	var nilCount int
	var lastOwnerCount int
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			nilCount++
		case errors.Is(err, ErrLastOwner):
			lastOwnerCount++
		default:
			t.Fatalf("unexpected concurrent removal error: %v", err)
		}
	}

	testutil.Equal(t, 1, nilCount)
	testutil.Equal(t, 1, lastOwnerCount)
}

func TestOrgMembershipStoreConcurrentOwnerDemotionReturnsLastOwner(t *testing.T) {
	setupTenantTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orgStore := newTestOrgStore()
	store := newTestOrgMembershipStore()

	org, err := orgStore.CreateOrg(ctx, "Concurrent Demotions", "concurrent-demotions", nil, "free")
	testutil.NoError(t, err)
	ownerOne := insertTestUser(t, "concurrent-demotion-1@example.com")
	ownerTwo := insertTestUser(t, "concurrent-demotion-2@example.com")

	_, err = store.AddOrgMembership(ctx, org.ID, ownerOne, OrgRoleOwner)
	testutil.NoError(t, err)
	_, err = store.AddOrgMembership(ctx, org.ID, ownerTwo, OrgRoleOwner)
	testutil.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)

	go func() {
		<-start
		_, updateErr := store.UpdateOrgMembershipRole(ctx, org.ID, ownerOne, OrgRoleAdmin)
		results <- updateErr
	}()
	go func() {
		<-start
		_, updateErr := store.UpdateOrgMembershipRole(ctx, org.ID, ownerTwo, OrgRoleAdmin)
		results <- updateErr
	}()

	close(start)

	var nilCount int
	var lastOwnerCount int
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			nilCount++
		case errors.Is(err, ErrLastOwner):
			lastOwnerCount++
		default:
			t.Fatalf("unexpected concurrent demotion error: %v", err)
		}
	}

	testutil.Equal(t, 1, nilCount)
	testutil.Equal(t, 1, lastOwnerCount)
}

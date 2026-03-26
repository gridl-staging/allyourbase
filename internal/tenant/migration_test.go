package tenant

import (
	"testing"
)

// --- groupAppsByOwner ---

func TestGroupAppsByOwner_Empty(t *testing.T) {
	t.Parallel()
	groups := groupAppsByOwner(nil)
	if len(groups) != 0 {
		t.Fatalf("expected empty groups, got %d", len(groups))
	}
}

func TestGroupAppsByOwner_SingleOwner(t *testing.T) {
	t.Parallel()
	rows := []legacyAppRow{
		{AppID: "app-1", OwnerUserID: "user-1", OwnerEmail: "alice@example.com"},
		{AppID: "app-2", OwnerUserID: "user-1", OwnerEmail: "alice@example.com"},
	}
	groups := groupAppsByOwner(rows)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.OwnerUserID != "user-1" {
		t.Errorf("expected owner user-1, got %s", g.OwnerUserID)
	}
	if len(g.AppIDs) != 2 {
		t.Errorf("expected 2 apps, got %d", len(g.AppIDs))
	}
}

func TestGroupAppsByOwner_MultipleOwners(t *testing.T) {
	t.Parallel()
	rows := []legacyAppRow{
		{AppID: "app-1", OwnerUserID: "user-1", OwnerEmail: "alice@example.com"},
		{AppID: "app-2", OwnerUserID: "user-2", OwnerEmail: "bob@example.com"},
		{AppID: "app-3", OwnerUserID: "user-1", OwnerEmail: "alice@example.com"},
	}
	groups := groupAppsByOwner(rows)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// Groups should be deterministically ordered by owner_user_id.
	if groups[0].OwnerUserID != "user-1" {
		t.Errorf("expected first group owner user-1, got %s", groups[0].OwnerUserID)
	}
	if len(groups[0].AppIDs) != 2 {
		t.Errorf("expected 2 apps for user-1, got %d", len(groups[0].AppIDs))
	}
	if groups[1].OwnerUserID != "user-2" {
		t.Errorf("expected second group owner user-2, got %s", groups[1].OwnerUserID)
	}
	if len(groups[1].AppIDs) != 1 {
		t.Errorf("expected 1 app for user-2, got %d", len(groups[1].AppIDs))
	}
}

func TestGroupAppsByOwner_DeterministicOrdering(t *testing.T) {
	t.Parallel()
	// Input in non-sorted order; groups should come out sorted by owner_user_id.
	rows := []legacyAppRow{
		{AppID: "app-z", OwnerUserID: "user-z", OwnerEmail: "z@example.com"},
		{AppID: "app-a", OwnerUserID: "user-a", OwnerEmail: "a@example.com"},
		{AppID: "app-m", OwnerUserID: "user-m", OwnerEmail: "m@example.com"},
	}
	groups := groupAppsByOwner(rows)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if groups[0].OwnerUserID != "user-a" || groups[1].OwnerUserID != "user-m" || groups[2].OwnerUserID != "user-z" {
		t.Errorf("groups not sorted: %v, %v, %v", groups[0].OwnerUserID, groups[1].OwnerUserID, groups[2].OwnerUserID)
	}
}

func TestGroupAppsByOwner_MissingOwner(t *testing.T) {
	t.Parallel()
	// Apps whose owner is not in _ayb_users will have empty OwnerEmail.
	rows := []legacyAppRow{
		{AppID: "app-1", OwnerUserID: "ghost-user", OwnerEmail: ""},
	}
	groups := groupAppsByOwner(rows)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].OwnerEmail != "" {
		t.Errorf("expected empty email for missing owner, got %s", groups[0].OwnerEmail)
	}
}

// --- deriveSlug ---

func TestDeriveSlug_FromEmail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		email string
		want  string
	}{
		{"alice@example.com", "alice"},
		{"bob.smith@example.com", "bob-smith"},
		{"user+tag@example.com", "user-tag"},
		{"UPPER@example.com", "upper"},
		{"multi..dots@example.com", "multi-dots"},
		{"@example.com", "tenant"},   // empty local part → fallback
		{"", "tenant"},               // empty email → fallback
		{"no-at-sign", "no-at-sign"}, // no @ → treat whole string as local
	}
	for _, tc := range cases {
		got := deriveSlug(tc.email)
		if got != tc.want {
			t.Errorf("deriveSlug(%q) = %q, want %q", tc.email, got, tc.want)
		}
	}
}

func TestDeriveSlug_StripLeadingTrailingDashes(t *testing.T) {
	t.Parallel()
	// Emails starting/ending with special chars should produce clean slugs.
	got := deriveSlug("-bad-@example.com")
	// Leading/trailing dashes stripped.
	if got == "" {
		t.Error("expected non-empty slug")
	}
	if got[0] == '-' || got[len(got)-1] == '-' {
		t.Errorf("slug has leading/trailing dash: %q", got)
	}
}

// --- resolveSlugWithCollisions ---

func TestResolveSlugWithCollisions_NoneExist(t *testing.T) {
	t.Parallel()
	existing := map[string]bool{}
	got := resolveSlugWithCollisions("alice", existing)
	if got != "alice" {
		t.Errorf("expected alice, got %s", got)
	}
}

func TestResolveSlugWithCollisions_FirstCollision(t *testing.T) {
	t.Parallel()
	existing := map[string]bool{"alice": true}
	got := resolveSlugWithCollisions("alice", existing)
	if got != "alice-1" {
		t.Errorf("expected alice-1, got %s", got)
	}
}

func TestResolveSlugWithCollisions_MultipleCollisions(t *testing.T) {
	t.Parallel()
	existing := map[string]bool{"alice": true, "alice-1": true, "alice-2": true}
	got := resolveSlugWithCollisions("alice", existing)
	if got != "alice-3" {
		t.Errorf("expected alice-3, got %s", got)
	}
}

func TestProposeDryRunSlug_SkipDoesNotClaim(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	g := ownerGroup{
		OwnerUserID: "user-1",
		OwnerEmail:  "alice@example.com",
		AppIDs:      []string{"app-1"},
	}
	got := proposeDryRunSlug(g, dryRunActionSkip, seen)
	if got != "" {
		t.Errorf("expected empty slug for skip, got %q", got)
	}
	if seen["alice"] {
		t.Error("skip action must not reserve slug")
	}
}

func TestProposeDryRunSlug_CreateClaims(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{"alice": true}
	g := ownerGroup{
		OwnerUserID: "user-1",
		OwnerEmail:  "alice@example.com",
		AppIDs:      []string{"app-1"},
	}
	got := proposeDryRunSlug(g, dryRunActionCreate, seen)
	if got != "alice-1" {
		t.Errorf("expected alice-1, got %q", got)
	}
	if !seen["alice-1"] {
		t.Error("create action must reserve selected slug")
	}
}

func TestProposeDryRunSlug_ReuseDoesNotClaim(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	g := ownerGroup{
		OwnerUserID:      "user-1",
		OwnerEmail:       "alice@example.com",
		AppIDs:           []string{"app-1"},
		ExistingTenantID: "tenant-1",
	}
	got := proposeDryRunSlug(g, dryRunActionReuse, seen)
	if got != "alice" {
		t.Errorf("expected derived reuse slug alice, got %q", got)
	}
	if seen["alice"] {
		t.Error("reuse action must not reserve slug")
	}
}

// --- dry-run conflict classification ---

func TestClassifyGroup_MissingOwner(t *testing.T) {
	t.Parallel()
	g := ownerGroup{
		OwnerUserID: "ghost",
		OwnerEmail:  "", // missing owner
		AppIDs:      []string{"app-1"},
	}
	action, conflict := classifyGroup(g, map[string]bool{})
	if action != dryRunActionSkip {
		t.Errorf("expected skip, got %s", action)
	}
	if conflict != dryRunConflictMissingOwner {
		t.Errorf("expected missing-owner conflict, got %s", conflict)
	}
}

func TestClassifyGroup_NoApps(t *testing.T) {
	t.Parallel()
	g := ownerGroup{
		OwnerUserID: "user-1",
		OwnerEmail:  "alice@example.com",
		AppIDs:      []string{},
	}
	action, conflict := classifyGroup(g, map[string]bool{})
	if action != dryRunActionSkip {
		t.Errorf("expected skip for owner with no apps, got %s", action)
	}
	if conflict != dryRunConflictNoApps {
		t.Errorf("expected no-apps conflict, got %s", conflict)
	}
}

func TestClassifyGroup_SlugCollisionExistingTenant(t *testing.T) {
	t.Parallel()
	g := ownerGroup{
		OwnerUserID: "user-1",
		OwnerEmail:  "alice@example.com",
		AppIDs:      []string{"app-1"},
	}
	// Existing slug forces collision handling (alice -> alice-1).
	existing := map[string]bool{"alice": true}
	action, conflict := classifyGroup(g, existing)
	if action != dryRunActionCreate {
		t.Errorf("expected create, got %s (conflict: %s)", action, conflict)
	}
	if conflict != dryRunConflictSlugCollision {
		t.Errorf("expected slug-collision conflict, got %s", conflict)
	}
}

func TestClassifyGroup_CreateNoConflict(t *testing.T) {
	t.Parallel()
	g := ownerGroup{
		OwnerUserID: "user-1",
		OwnerEmail:  "alice@example.com",
		AppIDs:      []string{"app-1"},
	}
	action, conflict := classifyGroup(g, map[string]bool{})
	if action != dryRunActionCreate {
		t.Errorf("expected create, got %s", action)
	}
	if conflict != "" {
		t.Errorf("expected no conflict, got %s", conflict)
	}
}

func TestClassifyGroup_ReuseExistingTenant(t *testing.T) {
	t.Parallel()
	// When idempotency key already exists, action is reuse.
	g := ownerGroup{
		OwnerUserID:      "user-1",
		OwnerEmail:       "alice@example.com",
		AppIDs:           []string{"app-1"},
		ExistingTenantID: "tenant-uuid",
	}
	action, conflict := classifyGroup(g, map[string]bool{})
	if action != dryRunActionReuse {
		t.Errorf("expected reuse, got %s (conflict: %s)", action, conflict)
	}
	if conflict != "" {
		t.Errorf("expected no conflict for reuse, got %s", conflict)
	}
}

// --- MigrationResult stats fields ---

func TestMigrationResult_ZeroValue(t *testing.T) {
	t.Parallel()
	var r MigrationResult
	if r.ExaminedGroups != 0 || r.CreatedTenants != 0 || r.AssignedApps != 0 {
		t.Error("expected zero values in MigrationResult")
	}
}

// --- ConsistencyReport ---

func TestConsistencyReport_CleanState(t *testing.T) {
	t.Parallel()
	r := ConsistencyReport{
		NullTenantIDApps:     0,
		DanglingTenantIDApps: 0,
		OrphanTenants:        0,
	}
	r.computeClean()
	if !r.Clean {
		t.Error("expected Clean=true when all counts are zero")
	}
}

func TestConsistencyReport_DirtyState(t *testing.T) {
	t.Parallel()
	r := ConsistencyReport{
		NullTenantIDApps:     3,
		DanglingTenantIDApps: 0,
		OrphanTenants:        0,
	}
	r.computeClean()
	if r.Clean {
		t.Error("expected Clean=false when NullTenantIDApps > 0")
	}
}

package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockOrgTenantChecker struct {
	orgID *string
	err   error
}

func (m *mockOrgTenantChecker) TenantOrgID(_ context.Context, _ string) (*string, error) {
	return m.orgID, m.err
}

func TestResolveAPIKeyTenantAccess_LegacyKeyPassesThrough(t *testing.T) {
	t.Parallel()
	claims := &Claims{}
	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "tenant-1", nil)
	testutil.NoError(t, err)
}

func TestResolveAPIKeyTenantAccess_NilClaimsPassesThrough(t *testing.T) {
	t.Parallel()
	err := ResolveAPIKeyTenantAccess(context.Background(), nil, "tenant-1", nil)
	testutil.NoError(t, err)
}

func TestResolveAPIKeyTenantAccess_AppScopedPassesThrough(t *testing.T) {
	t.Parallel()
	claims := &Claims{AppID: "some-app"}
	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "tenant-1", nil)
	testutil.NoError(t, err)
}

func TestResolveAPIKeyTenantAccess_OrgScopedMatchingOrg(t *testing.T) {
	t.Parallel()
	orgID := "org-123"
	claims := &Claims{OrgID: orgID}
	checker := &mockOrgTenantChecker{orgID: &orgID}

	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "tenant-1", checker)
	testutil.NoError(t, err)
}

func TestResolveAPIKeyTenantAccess_OrgScopedMismatch(t *testing.T) {
	t.Parallel()
	claimOrg := "org-123"
	tenantOrg := "org-456"
	claims := &Claims{OrgID: claimOrg}
	checker := &mockOrgTenantChecker{orgID: &tenantOrg}

	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "tenant-1", checker)
	testutil.True(t, err != nil, "expected error for org mismatch")
	testutil.True(t, errors.Is(err, ErrOrgScopeUnauthorized), "expected unauthorized sentinel")
}

func TestResolveAPIKeyTenantAccess_OrgScopedTenantHasNoOrg(t *testing.T) {
	t.Parallel()
	claims := &Claims{OrgID: "org-123"}
	checker := &mockOrgTenantChecker{orgID: nil}

	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "tenant-1", checker)
	testutil.True(t, err != nil, "expected error for tenant with no org")
	testutil.True(t, errors.Is(err, ErrOrgScopeUnauthorized), "expected unauthorized sentinel")
}

func TestResolveAPIKeyTenantAccess_OrgScopedNoTenantID(t *testing.T) {
	t.Parallel()
	claims := &Claims{OrgID: "org-123"}

	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "", nil)
	testutil.True(t, err != nil, "expected error for empty tenant ID")
	testutil.True(t, errors.Is(err, ErrOrgScopeUnauthorized), "expected unauthorized sentinel")
	testutil.Contains(t, err.Error(), "tenant context required")
}

func TestResolveAPIKeyTenantAccess_OrgScopedNoChecker(t *testing.T) {
	t.Parallel()
	claims := &Claims{OrgID: "org-123"}

	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "tenant-1", nil)
	testutil.True(t, err != nil, "expected error for missing checker")
	testutil.True(t, errors.Is(err, ErrOrgScopeMisconfigured), "expected misconfigured sentinel")
}

func TestResolveAPIKeyTenantAccess_CheckerError(t *testing.T) {
	t.Parallel()
	claims := &Claims{OrgID: "org-123"}
	checker := &mockOrgTenantChecker{err: fmt.Errorf("db error")}

	err := ResolveAPIKeyTenantAccess(context.Background(), claims, "tenant-1", checker)
	testutil.True(t, err != nil, "expected error from checker")
	testutil.True(t, errors.Is(err, ErrOrgScopeLookupFailed), "expected lookup sentinel")
	testutil.Contains(t, err.Error(), "db error")
}

func TestMapCreateAPIKeyInsertErrorOrgFK(t *testing.T) {
	t.Parallel()

	err := mapCreateAPIKeyInsertError(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "_ayb_api_keys_org_id_fkey",
	})
	testutil.Equal(t, ErrInvalidOrgID, err)
}

func TestMapCreateAPIKeyInsertErrorScopeExclusivity(t *testing.T) {
	t.Parallel()

	err := mapCreateAPIKeyInsertError(&pgconn.PgError{
		Code:           "23514",
		ConstraintName: "_ayb_api_keys_scope_exclusivity",
	})
	testutil.Equal(t, ErrAppOrgScopeConflict, err)
}

func TestAppOrgScopeConflictError(t *testing.T) {
	t.Parallel()
	testutil.Contains(t, ErrAppOrgScopeConflict.Error(), "mutually exclusive")
}

func TestCreateAPIKeyInvalidOrgUUID(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	orgID := "not-a-uuid"

	_, _, err := svc.CreateAPIKey(context.Background(), "user-1", "bad org", CreateAPIKeyOptions{OrgID: &orgID})
	testutil.Equal(t, ErrInvalidOrgID, err)
}

func TestErrInvalidOrgIDDistinct(t *testing.T) {
	t.Parallel()
	testutil.True(t, ErrInvalidOrgID != ErrInvalidAppID, "org error should be distinct from app error")
	testutil.Contains(t, ErrInvalidOrgID.Error(), "org not found")
}

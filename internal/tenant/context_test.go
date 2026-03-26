package tenant

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTenantFromContextReturnsEmptyForMissingTenant(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "", TenantFromContext(context.Background()))
}

func TestContextWithTenantIDRoundTrip(t *testing.T) {
	t.Parallel()
	base := context.Background()
	ctx := ContextWithTenantID(base, "tenant-1")
	testutil.Equal(t, "tenant-1", TenantFromContext(ctx))
}

func TestContextWithTenantIDEmptyString(t *testing.T) {
	t.Parallel()
	base := context.Background()
	ctx := ContextWithTenantID(base, "")
	testutil.Equal(t, "", TenantFromContext(ctx))
}

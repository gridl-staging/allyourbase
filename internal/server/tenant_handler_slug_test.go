package server

import (
	"testing"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTenantHandlerSlugValidationMatchesTenant(t *testing.T) {
	t.Parallel()

	slugs := []string{
		"ab",
		"tenant-slug",
		"-bad",
		"bad-",
		"Aa",
		"a",
		"with space",
	}

	for _, slug := range slugs {
		testutil.Equal(t, tenant.IsValidSlug(slug), isValidSlug(slug))
	}
}

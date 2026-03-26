package tenant

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestIsValidSlug(t *testing.T) {
	t.Parallel()

	validSlugs := []string{
		"ab",
		"team-1",
		strings.Repeat("a", 63),
	}
	for _, slug := range validSlugs {
		testutil.True(t, IsValidSlug(slug))
	}

	invalidSlugs := []string{
		"a",
		"-team",
		"team-",
		"Team",
		"team name",
		strings.Repeat("a", 64),
	}
	for _, slug := range invalidSlugs {
		testutil.False(t, IsValidSlug(slug))
	}
}

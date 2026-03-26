package auth

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRefreshTokenOptions_PreservesAMRForAAL1(t *testing.T) {
	t.Parallel()

	opts := refreshTokenOptions("aal1", "password")
	testutil.True(t, opts != nil, "expected opts when AMR is present")
	testutil.Equal(t, "aal1", opts.AAL)
	testutil.Equal(t, 1, len(opts.AMR))
	testutil.Equal(t, "password", opts.AMR[0])
}

func TestRefreshTokenOptions_DefaultsToNilWhenSessionIsDefaultAAL1(t *testing.T) {
	t.Parallel()

	opts := refreshTokenOptions("aal1", "")
	testutil.True(t, opts == nil, "expected nil opts for default session values")
}

func TestRefreshTokenOptions_PreservesAAL2WithAMR(t *testing.T) {
	t.Parallel()

	opts := refreshTokenOptions("aal2", "password,totp")
	testutil.True(t, opts != nil, "expected opts for AAL2 session")
	testutil.Equal(t, "aal2", opts.AAL)
	testutil.Equal(t, 2, len(opts.AMR))
	testutil.Equal(t, "password", opts.AMR[0])
	testutil.Equal(t, "totp", opts.AMR[1])
}

func TestRefreshTokenOptions_EmptyAALDefaultsToAAL1(t *testing.T) {
	t.Parallel()

	opts := refreshTokenOptions("", "password")
	testutil.True(t, opts != nil, "expected opts when AMR is present")
	testutil.Equal(t, "aal1", opts.AAL)
	testutil.Equal(t, 1, len(opts.AMR))
}

func TestRefreshTokenOptions_NilWhenBothEmpty(t *testing.T) {
	t.Parallel()

	opts := refreshTokenOptions("", "")
	testutil.True(t, opts == nil, "expected nil opts when both empty")
}

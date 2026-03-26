package auth

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestFirstFactorMethodFromPendingClaims(t *testing.T) {
	t.Parallel()

	testutil.Equal(t, "oauth", firstFactorMethodFromPendingClaims(&Claims{
		AMR: []string{"oauth", "totp"},
	}))
}

func TestFirstFactorMethodFromPendingClaims_DefaultsToPassword(t *testing.T) {
	t.Parallel()

	testutil.Equal(t, "password", firstFactorMethodFromPendingClaims(nil))
	testutil.Equal(t, "password", firstFactorMethodFromPendingClaims(&Claims{}))
	testutil.Equal(t, "password", firstFactorMethodFromPendingClaims(&Claims{AMR: []string{""}}))
}

func TestMFASessionOptions(t *testing.T) {
	t.Parallel()

	opts := mfaSessionOptions("oauth", "email_otp")
	testutil.True(t, opts != nil, "expected options")
	testutil.Equal(t, "aal2", opts.AAL)
	testutil.Equal(t, 2, len(opts.AMR))
	testutil.Equal(t, "oauth", opts.AMR[0])
	testutil.Equal(t, "email_otp", opts.AMR[1])
}

func TestMFASessionOptions_EmptyFirstFactor(t *testing.T) {
	t.Parallel()

	opts := mfaSessionOptions("", "totp")
	testutil.True(t, opts != nil, "expected options")
	testutil.Equal(t, "aal2", opts.AAL)
	testutil.Equal(t, 2, len(opts.AMR))
	testutil.Equal(t, "password", opts.AMR[0])
	testutil.Equal(t, "totp", opts.AMR[1])
}

func TestMFASessionOptions_EmptySecondFactor(t *testing.T) {
	t.Parallel()

	opts := mfaSessionOptions("oauth", "")
	testutil.True(t, opts != nil, "expected options")
	testutil.Equal(t, "aal2", opts.AAL)
	testutil.Equal(t, 1, len(opts.AMR))
	testutil.Equal(t, "oauth", opts.AMR[0])
}

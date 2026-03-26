// Package auth mfa_amr.go provides utilities for working with authentication method references (AMR) in MFA sessions, extracting method claims and configuring token options.
package auth

import "strings"

const defaultFirstFactorMethod = "password"

func firstFactorMethodFromPendingClaims(claims *Claims) string {
	if claims != nil && len(claims.AMR) > 0 {
		if method := strings.TrimSpace(claims.AMR[0]); method != "" {
			return method
		}
	}
	return defaultFirstFactorMethod
}

// mfaSessionOptions returns a tokenOptions configured for a multi-factor authentication session with the given factor authentication methods, defaulting the first factor to password if empty.
func mfaSessionOptions(firstFactorMethod, secondFactorMethod string) *tokenOptions {
	amr := make([]string, 0, 2)

	first := strings.TrimSpace(firstFactorMethod)
	if first == "" {
		first = defaultFirstFactorMethod
	}
	amr = append(amr, first)

	second := strings.TrimSpace(secondFactorMethod)
	if second != "" {
		amr = append(amr, second)
	}

	return &tokenOptions{
		AAL: "aal2",
		AMR: amr,
	}
}

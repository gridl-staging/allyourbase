// Package auth Provides HTTP handlers for TOTP-based multi-factor authentication, including enrollment, verification, and factor management endpoints.
package auth

import (
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

// SetTOTPEnabled enables or disables the TOTP MFA endpoints.
func (h *Handler) SetTOTPEnabled(enabled bool) {
	h.totpEnabled = enabled
}

// requireTOTPEnabled is middleware that returns 404 when TOTP is disabled.
func (h *Handler) requireTOTPEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.totpEnabled {
			httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "TOTP MFA is not enabled",
				"https://allyourbase.io/guide/authentication#totp")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type totpEnrollConfirmRequest struct {
	Code string `json:"code"`
}

type totpVerifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
}

// handleTOTPEnroll initiates TOTP enrollment for the authenticated user, generating a secret and QR code for scanning into an authenticator application. It enforces additional authentication (AAL2) if the user already has an MFA factor enrolled.
func (h *Handler) handleTOTPEnroll(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, ErrAnonymousMFABlock.Error())
		return
	}

	// If user already has an enabled MFA factor, require AAL2 to enroll additional factors.
	if h.enforceAAL2ForExistingMFA(w, r, claims) {
		return
	}

	enrollment, err := h.auth.EnrollTOTP(r.Context(), claims.Subject, claims.Email, h.totpIssuer())
	if err != nil {
		switch {
		case errors.Is(err, ErrTOTPAlreadyEnrolled):
			httputil.WriteError(w, http.StatusConflict, "TOTP MFA already enrolled")
		case errors.Is(err, ErrEncryptionKeyNotSet):
			h.logger.Error("TOTP enroll: encryption key not configured")
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		default:
			h.logger.Error("TOTP enroll error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, enrollment)
}

// handleTOTPEnrollConfirm confirms a pending TOTP enrollment by validating the provided TOTP code. It requires an authenticated, non-anonymous user and activates the factor on successful verification.
func (h *Handler) handleTOTPEnrollConfirm(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, ErrAnonymousMFABlock.Error())
		return
	}

	var req totpEnrollConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Code == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	if err := h.auth.ConfirmTOTPEnrollment(r.Context(), claims.Subject, req.Code); err != nil {
		switch {
		case errors.Is(err, ErrTOTPNotEnrolled):
			httputil.WriteError(w, http.StatusNotFound, "no pending TOTP enrollment found")
		case errors.Is(err, ErrTOTPInvalidCode):
			httputil.WriteError(w, http.StatusUnauthorized, "invalid TOTP code")
		default:
			h.logger.Error("TOTP enroll confirm error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "TOTP MFA enrollment confirmed",
	})
}

// handleTOTPChallenge creates a TOTP challenge during the MFA verification flow, returning a challenge ID for use in the subsequent verification request.
func (h *Handler) handleTOTPChallenge(w http.ResponseWriter, r *http.Request) {
	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	challengeID, err := h.auth.CreateTOTPChallenge(r.Context(), claims.Subject, mfaChallengeIP(r))
	if err != nil {
		switch {
		case errors.Is(err, ErrTOTPNotEnrolled):
			httputil.WriteError(w, http.StatusNotFound, "no TOTP factor enrolled")
		default:
			h.logger.Error("TOTP challenge error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"challenge_id": challengeID,
	})
}

// handleTOTPVerify verifies a TOTP challenge by validating the provided code, completing the MFA flow and returning access/refresh tokens and user data. It enforces cumulative failure rate limiting and rejects replayed codes.
func (h *Handler) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	var req totpVerifyRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.ChallengeID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "challenge_id is required")
		return
	}
	if req.Code == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	// Check cumulative lockout before attempting verification.
	if h.auth.IsMFALocked(claims.Subject) {
		httputil.WriteError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	user, accessToken, refreshToken, err := h.auth.VerifyTOTPChallenge(
		r.Context(), claims.Subject, req.ChallengeID, req.Code, firstFactorMethodFromPendingClaims(claims),
	)
	if err != nil {
		// Record failure for cumulative lockout tracking.
		if errors.Is(err, ErrTOTPInvalidCode) || errors.Is(err, ErrTOTPReplay) {
			h.auth.RecordMFAFailure(claims.Subject)
		}
		switch {
		case errors.Is(err, ErrTOTPChallengeNotFound):
			httputil.WriteError(w, http.StatusNotFound, "challenge not found or expired")
		case errors.Is(err, ErrTOTPChallengeUsed):
			httputil.WriteError(w, http.StatusConflict, "challenge already verified")
		case errors.Is(err, ErrTOTPInvalidCode):
			httputil.WriteError(w, http.StatusUnauthorized, "invalid TOTP code")
		case errors.Is(err, ErrTOTPReplay):
			httputil.WriteError(w, http.StatusUnauthorized, "TOTP code already used")
		default:
			h.logger.Error("TOTP verify error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Reset failure tracker on successful verification.
	h.auth.ResetMFAFailures(claims.Subject)

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// handleMFAFactors returns the list of MFA factors enrolled for the authenticated user, accepting both regular and MFA-pending authorization tokens.
func (h *Handler) handleMFAFactors(w http.ResponseWriter, r *http.Request) {
	// Accept both regular auth and MFA pending tokens.
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		claims = mfaPendingClaimsFromContext(r.Context())
	}
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.Subject == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	factors, err := h.auth.GetUserMFAFactors(r.Context(), claims.Subject)
	if err != nil {
		h.logger.Error("MFA factors error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"factors": factors,
	})
}

// totpIssuer returns the issuer name for TOTP URIs.
func (h *Handler) totpIssuer() string {
	if h.auth.appName != "" {
		return h.auth.appName
	}
	return "AllYourBase"
}

// mfaChallengeIP returns a normalized client IP suitable for INET storage.
func mfaChallengeIP(r *http.Request) string {
	return clientIP(r)
}

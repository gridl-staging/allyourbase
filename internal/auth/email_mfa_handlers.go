// Package auth provides HTTP handlers for the email-based multi-factor authentication enrollment and verification flows.
package auth

import (
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

// SetEmailMFAEnabled enables or disables the email MFA endpoints.
func (h *Handler) SetEmailMFAEnabled(enabled bool) {
	h.emailMFAEnabled = enabled
}

// requireEmailMFAEnabled is middleware that returns 404 when email MFA is disabled.
func (h *Handler) requireEmailMFAEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.emailMFAEnabled {
			httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "Email MFA is not enabled",
				"https://allyourbase.io/guide/authentication#email-mfa")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type emailMFAEnrollConfirmRequest struct {
	Code string `json:"code"`
}

type emailMFAVerifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
}

// handleEmailMFAEnroll initiates email MFA enrollment for the authenticated user, sending a verification code to their email address. It rejects anonymous users and requires AAL2 authentication if the user already has another MFA factor enrolled.
func (h *Handler) handleEmailMFAEnroll(w http.ResponseWriter, r *http.Request) {
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

	if err := h.auth.EnrollEmailMFA(r.Context(), claims.Subject, claims.Email); err != nil {
		switch {
		case errors.Is(err, ErrEmailMFAAlreadyEnrolled):
			httputil.WriteError(w, http.StatusConflict, "email MFA already enrolled")
		default:
			h.logger.Error("email MFA enroll error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "verification code sent to your email",
	})
}

// handleEmailMFAEnrollConfirm completes email MFA enrollment by validating the verification code sent during the enrollment step.
func (h *Handler) handleEmailMFAEnrollConfirm(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, ErrAnonymousMFABlock.Error())
		return
	}

	var req emailMFAEnrollConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Code == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	if err := h.auth.ConfirmEmailMFAEnrollment(r.Context(), claims.Subject, req.Code); err != nil {
		switch {
		case errors.Is(err, ErrEmailMFANotEnrolled):
			httputil.WriteError(w, http.StatusNotFound, "no pending email MFA enrollment found")
		case errors.Is(err, ErrEmailMFAInvalidCode):
			httputil.WriteError(w, http.StatusUnauthorized, "invalid email MFA code")
		case errors.Is(err, ErrEmailMFAExpired):
			httputil.WriteError(w, http.StatusGone, "email MFA code expired")
		default:
			h.logger.Error("email MFA enroll confirm error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "email MFA enrollment confirmed",
	})
}

// handleEmailMFAChallenge issues an email MFA challenge for a user with pending MFA requirements, sending a code to their enrolled email address and returning a challenge ID.
func (h *Handler) handleEmailMFAChallenge(w http.ResponseWriter, r *http.Request) {
	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	challengeID, err := h.auth.ChallengeEmailMFA(r.Context(), claims.Subject, claims.Email)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailMFANotEnrolled):
			httputil.WriteError(w, http.StatusNotFound, "no email MFA factor enrolled")
		case errors.Is(err, ErrEmailMFARateLimit):
			httputil.WriteError(w, http.StatusTooManyRequests, "too many email challenges, try again later")
		default:
			h.logger.Error("email MFA challenge error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"challenge_id": challengeID,
	})
}

// handleEmailMFAVerify verifies an email MFA code against the issued challenge, enforcing cumulative failure lockout and returning session tokens on success. It tracks failed attempts for lockout and resets the failure counter upon successful verification.
func (h *Handler) handleEmailMFAVerify(w http.ResponseWriter, r *http.Request) {
	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	var req emailMFAVerifyRequest
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

	user, accessToken, refreshToken, err := h.auth.VerifyEmailMFA(
		r.Context(), claims.Subject, req.ChallengeID, req.Code, firstFactorMethodFromPendingClaims(claims),
	)
	if err != nil {
		// Record failure for cumulative lockout tracking.
		if errors.Is(err, ErrEmailMFAInvalidCode) || errors.Is(err, ErrEmailMFAExpired) {
			h.auth.RecordMFAFailure(claims.Subject)
		}
		switch {
		case errors.Is(err, ErrTOTPChallengeNotFound):
			httputil.WriteError(w, http.StatusNotFound, "challenge not found or expired")
		case errors.Is(err, ErrTOTPChallengeUsed):
			httputil.WriteError(w, http.StatusConflict, "challenge already verified")
		case errors.Is(err, ErrEmailMFAInvalidCode):
			httputil.WriteError(w, http.StatusUnauthorized, "invalid email MFA code")
		case errors.Is(err, ErrEmailMFAExpired):
			httputil.WriteError(w, http.StatusGone, "email MFA code expired")
		case errors.Is(err, ErrEmailMFALocked):
			httputil.WriteError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		default:
			h.logger.Error("email MFA verify error", "error", err)
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

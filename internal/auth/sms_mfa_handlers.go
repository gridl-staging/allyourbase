// Package auth provides SMS MFA handlers that manage enrollment, challenge, and verification of SMS-based multi-factor authentication.
package auth

import (
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

type mfaEnrollRequest struct {
	Phone string `json:"phone"`
}

type mfaEnrollConfirmRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

type mfaVerifyRequest struct {
	Code string `json:"code"`
}

type mfaPendingResponse struct {
	MFAPending bool   `json:"mfa_pending"`
	MFAToken   string `json:"mfa_token"`
}

// handleMFAEnroll initiates SMS MFA enrollment by sending a verification code to the provided phone number. It requires an authenticated non-anonymous user, enforces additional authentication (AAL2) if the user already has an MFA factor enrolled, and validates the phone number format.
func (h *Handler) handleMFAEnroll(w http.ResponseWriter, r *http.Request) {
	if !h.smsEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS MFA is not enabled",
			"https://allyourbase.io/guide/authentication#sms")
		return
	}

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

	var req mfaEnrollRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Phone == "" {
		httputil.WriteError(w, http.StatusBadRequest, "phone is required")
		return
	}

	if err := h.auth.EnrollSMSMFA(r.Context(), claims.Subject, req.Phone); err != nil {
		switch {
		case errors.Is(err, ErrInvalidPhoneNumber):
			httputil.WriteError(w, http.StatusBadRequest, "invalid phone number format")
		case errors.Is(err, ErrMFAAlreadyEnrolled):
			httputil.WriteError(w, http.StatusConflict, "SMS MFA already enrolled")
		default:
			h.logger.Error("MFA enroll error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "verification code sent",
	})
}

// handleMFAEnrollConfirm completes SMS MFA enrollment by validating the verification code sent to the user's phone. It requires authentication and confirms the code before enabling the MFA factor.
func (h *Handler) handleMFAEnrollConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.smsEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS MFA is not enabled",
			"https://allyourbase.io/guide/authentication#sms")
		return
	}

	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, ErrAnonymousMFABlock.Error())
		return
	}

	var req mfaEnrollConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Phone == "" {
		httputil.WriteError(w, http.StatusBadRequest, "phone is required")
		return
	}
	if req.Code == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	if err := h.auth.ConfirmSMSMFAEnrollment(r.Context(), claims.Subject, req.Phone, req.Code); err != nil {
		if errors.Is(err, ErrInvalidSMSCode) {
			httputil.WriteError(w, http.StatusUnauthorized, "invalid or expired code")
			return
		}
		h.logger.Error("MFA enroll confirm error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "MFA enrollment confirmed",
	})
}

// handleMFAChallenge requests that a verification code be sent to the user's enrolled phone during MFA verification. It requires an active MFA challenge pending in the user's session.
func (h *Handler) handleMFAChallenge(w http.ResponseWriter, r *http.Request) {
	if !h.smsEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS MFA is not enabled",
			"https://allyourbase.io/guide/authentication#sms")
		return
	}

	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	if err := h.auth.ChallengeSMSMFA(r.Context(), claims.Subject); err != nil {
		h.logger.Error("MFA challenge error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "verification code sent",
	})
}

// handleMFAVerify validates the verification code provided by the user to complete the MFA challenge. It enforces cumulative failure lockout tracking, records failed attempts for rate limiting, and on success returns access and refresh tokens.
func (h *Handler) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	if !h.smsEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS MFA is not enabled",
			"https://allyourbase.io/guide/authentication#sms")
		return
	}

	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	var req mfaVerifyRequest
	if !decodeBody(w, r, &req) {
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

	user, accessToken, refreshToken, err := h.auth.VerifySMSMFA(
		r.Context(), claims.Subject, req.Code, firstFactorMethodFromPendingClaims(claims),
	)
	if err != nil {
		if errors.Is(err, ErrInvalidSMSCode) {
			// Record failure for cumulative lockout tracking.
			h.auth.RecordMFAFailure(claims.Subject)
			httputil.WriteError(w, http.StatusUnauthorized, "invalid or expired code")
			return
		}
		h.logger.Error("MFA verify error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
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

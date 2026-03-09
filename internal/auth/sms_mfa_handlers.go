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

	user, accessToken, refreshToken, err := h.auth.VerifySMSMFA(r.Context(), claims.Subject, req.Code)
	if err != nil {
		if errors.Is(err, ErrInvalidSMSCode) {
			httputil.WriteError(w, http.StatusUnauthorized, "invalid or expired code")
			return
		}
		h.logger.Error("MFA verify error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

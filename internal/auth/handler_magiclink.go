package auth

import (
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

type magicLinkRequest struct {
	Email string `json:"email"`
}

// handleMagicLinkRequest initiates passwordless authentication by sending a magic link to the user's email, returning 200 regardless of email validity to prevent enumeration.
func (h *Handler) handleMagicLinkRequest(w http.ResponseWriter, r *http.Request) {
	if !h.magicLinkEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "magic link authentication is not enabled",
			"https://allyourbase.io/guide/authentication#magic-link")
		return
	}

	var req magicLinkRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Always return 200 to prevent email enumeration.
	if err := h.auth.RequestMagicLink(r.Context(), req.Email); err != nil {
		h.logger.Error("magic link request error", "error", err)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "if valid, a login link has been sent"})
}

// handleMagicLinkConfirm completes passwordless login by validating a magic link token and returning tokens or a pending MFA token if factors are enrolled.
func (h *Handler) handleMagicLinkConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.magicLinkEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "magic link authentication is not enabled",
			"https://allyourbase.io/guide/authentication#magic-link")
		return
	}

	var req tokenRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	user, accessToken, refreshToken, err := h.auth.ConfirmMagicLink(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrInvalidMagicLinkToken) {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired magic link token",
				"https://allyourbase.io/guide/authentication#magic-link")
			return
		}
		h.logger.Error("magic link confirm error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if refreshToken == "" {
		httputil.WriteJSON(w, http.StatusOK, mfaPendingResponse{
			MFAPending: true,
			MFAToken:   accessToken,
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{Token: accessToken, RefreshToken: refreshToken, User: user})
}

// requireSMSEnabled is middleware that returns 404 when SMS is not configured.
func (h *Handler) requireSMSEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.smsEnabled {
			httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS MFA is not enabled",
				"https://allyourbase.io/guide/authentication#sms")
			return
		}
		next.ServeHTTP(w, r)
	})
}

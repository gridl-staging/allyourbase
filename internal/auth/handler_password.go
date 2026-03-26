package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
)

type passwordResetRequest struct {
	Email string `json:"email"`
}

type passwordResetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

// handlePasswordReset initiates a password reset flow by sending a reset token to the user's email, returning 200 regardless of whether the email exists to prevent email enumeration.
func (h *Handler) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Always return 200 to prevent email enumeration.
	if err := h.auth.RequestPasswordReset(r.Context(), req.Email); err != nil {
		h.logger.Error("password reset error", "error", err)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "if that email exists, a reset link has been sent"})
}

// handlePasswordResetConfirm completes the password reset flow by validating the token and setting a new password.
func (h *Handler) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var req passwordResetConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}
	if req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "password is required")
		return
	}

	err := h.auth.ConfirmPasswordReset(r.Context(), req.Token, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidResetToken):
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired reset token",
				"https://allyourbase.io/guide/authentication")
		case errors.Is(err, ErrValidation):
			msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
			httputil.WriteError(w, http.StatusBadRequest, msg)
		default:
			h.logger.Error("password reset confirm error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "password has been reset"})
}

// handleVerifyEmail confirms an email address using a verification token sent via email.
func (h *Handler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	err := h.auth.ConfirmEmail(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrInvalidVerifyToken) {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired verification token",
				"https://allyourbase.io/guide/authentication")
			return
		}
		h.logger.Error("email verification error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

func (h *Handler) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.auth.SendVerificationEmail(r.Context(), claims.Subject, claims.Email); err != nil {
		h.logger.Error("resend verification error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "verification email sent"})
}

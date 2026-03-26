// Package auth Provides HTTP handlers for backup code operations: generation, verification with MFA lockout tracking, and counting remaining codes.
package auth

import (
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

type backupVerifyRequest struct {
	Code string `json:"code"`
}

func backupClaimsSubjectOrUnauthorized(w http.ResponseWriter, claims *Claims) (string, bool) {
	if claims == nil || claims.Subject == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return "", false
	}
	return claims.Subject, true
}

func (h *Handler) handleBackupCodeGenerate(w http.ResponseWriter, r *http.Request) {
	h.handleBackupCodeIssue(w, r)
}

func (h *Handler) handleBackupCodeRegenerate(w http.ResponseWriter, r *http.Request) {
	h.handleBackupCodeIssue(w, r)
}

// Generates backup codes for the authenticated user and returns them in the response.
func (h *Handler) handleBackupCodeIssue(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	subject, ok := backupClaimsSubjectOrUnauthorized(w, claims)
	if !ok {
		return
	}

	codes, err := h.auth.GenerateBackupCodes(r.Context(), subject)
	if err != nil {
		h.logger.Error("backup code generation error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"codes": codes,
	})
}

// Verifies a backup code during MFA authentication, tracking failed attempts for cumulative lockout and returning authentication tokens on success.
func (h *Handler) handleBackupCodeVerify(w http.ResponseWriter, r *http.Request) {
	claims := mfaPendingClaimsFromContext(r.Context())
	subject, ok := backupClaimsSubjectOrUnauthorized(w, claims)
	if !ok {
		return
	}

	var req backupVerifyRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Code == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	// Check cumulative lockout before attempting verification.
	if h.auth.IsMFALocked(subject) {
		httputil.WriteError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	user, accessToken, refreshToken, err := h.auth.VerifyBackupCodeMFA(
		r.Context(), subject, req.Code, firstFactorMethodFromPendingClaims(claims),
	)
	if err != nil {
		if errors.Is(err, ErrBackupCodeInvalid) {
			// Record failure for cumulative lockout tracking.
			h.auth.RecordMFAFailure(subject)
			httputil.WriteError(w, http.StatusUnauthorized, "invalid or already used backup code")
			return
		}
		h.logger.Error("backup code verify error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Reset failure tracker on successful verification.
	h.auth.ResetMFAFailures(subject)

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// Returns the number of remaining backup codes for the authenticated user.
func (h *Handler) handleBackupCodeCount(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	subject, ok := backupClaimsSubjectOrUnauthorized(w, claims)
	if !ok {
		return
	}

	count, err := h.auth.GetBackupCodeCount(r.Context(), subject)
	if err != nil {
		h.logger.Error("backup code count error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]int{
		"remaining": count,
	})
}

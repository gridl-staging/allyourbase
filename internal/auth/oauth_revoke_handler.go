// Package auth Implements RFC 7009 OAuth token revocation handling for the auth handler.
package auth

import (
	"context"
	"net/http"
)

// oauthRevokeProvider is the subset of auth.Service used by the OAuth revocation handler.
type oauthRevokeProvider interface {
	RevokeOAuthToken(ctx context.Context, token string) error
}

// Handles an OAuth token revocation request per RFC 7009, validating that the request body is form-encoded and contains a required token parameter, then invoking the underlying service to revoke the token. Always returns 200 OK regardless of whether revocation succeeds; revocation errors are logged but do not affect the HTTP response.
func (h *Handler) handleOAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if !isFormURLEncoded(r.Header.Get("Content-Type")) {
		writeOAuthError(w, http.StatusBadRequest, OAuthErrInvalidRequest, "Content-Type must be application/x-www-form-urlencoded")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, OAuthErrInvalidRequest, "invalid form body")
		return
	}

	token := r.PostForm.Get("token")
	if token == "" {
		writeOAuthError(w, http.StatusBadRequest, OAuthErrInvalidRequest, "token is required")
		return
	}

	// token_type_hint is optional per RFC 7009 §2.1 — used as a hint only.
	// We ignore it and let the service figure out the token type by hash lookup.

	// Per RFC 7009: always return 200 OK regardless of outcome.
	if err := h.oauthRevoke.RevokeOAuthToken(r.Context(), token); err != nil {
		h.logger.Error("oauth token revocation failed", "error", err)
	}

	w.WriteHeader(http.StatusOK)
}

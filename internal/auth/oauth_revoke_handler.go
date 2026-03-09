package auth

import (
	"context"
	"net/http"
)

// oauthRevokeProvider is the subset of auth.Service used by the OAuth revocation handler.
type oauthRevokeProvider interface {
	RevokeOAuthToken(ctx context.Context, token string) error
}

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

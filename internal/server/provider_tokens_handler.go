// Package server Handlers for admin operations on OAuth provider tokens. Supports listing all provider tokens for a user and deleting specific provider tokens.
package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type userProviderTokenManager interface {
	ListProviderTokens(ctx context.Context, userID string) ([]auth.ProviderTokenInfo, error)
	DeleteProviderToken(ctx context.Context, userID, provider string) error
}

// handleAdminListUserProviderTokens returns an HTTP handler that retrieves all OAuth provider tokens associated with a user, validating the user ID format and returning the tokens as JSON.
func handleAdminListUserProviderTokens(svc userProviderTokenManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")
		if userID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "user id is required")
			return
		}
		if !httputil.IsValidUUID(userID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid user id format")
			return
		}

		tokens, err := svc.ListProviderTokens(r.Context(), userID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list provider tokens")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, tokens)
	}
}

// handleAdminDeleteUserProviderToken returns an HTTP handler that revokes an OAuth provider token for a user, validating the user and provider identifiers, and returning appropriate error responses for missing or invalid tokens.
func handleAdminDeleteUserProviderToken(svc userProviderTokenManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")
		if userID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "user id is required")
			return
		}
		if !httputil.IsValidUUID(userID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid user id format")
			return
		}

		provider := strings.TrimSpace(chi.URLParam(r, "provider"))
		if provider == "" {
			httputil.WriteError(w, http.StatusBadRequest, "provider is required")
			return
		}

		err := svc.DeleteProviderToken(r.Context(), userID, provider)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrProviderTokenNotFound):
				httputil.WriteError(w, http.StatusNotFound, "provider token not found")
			case errors.Is(err, auth.ErrValidation):
				httputil.WriteError(w, http.StatusBadRequest, "invalid provider token request")
			default:
				httputil.WriteError(w, http.StatusInternalServerError, "failed to revoke provider token")
			}
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

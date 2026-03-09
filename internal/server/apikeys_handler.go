package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// apiKeyManager is the interface for admin API key operations.
// auth.Service satisfies this interface.
type apiKeyManager interface {
	ListAllAPIKeys(ctx context.Context, page, perPage int) (*auth.APIKeyListResult, error)
	AdminRevokeAPIKey(ctx context.Context, keyID string) error
	CreateAPIKey(ctx context.Context, userID, name string, opts ...auth.CreateAPIKeyOptions) (string, *auth.APIKey, error)
}

// handleAdminListAPIKeys returns a paginated list of all API keys.
func handleAdminListAPIKeys(svc apiKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		result, err := svc.ListAllAPIKeys(r.Context(), page, perPage)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list api keys")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

// handleAdminRevokeAPIKey revokes any API key by ID.
func handleAdminRevokeAPIKey(svc apiKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			httputil.WriteError(w, http.StatusBadRequest, "api key id is required")
			return
		}
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid api key id format")
			return
		}

		err := svc.AdminRevokeAPIKey(r.Context(), id)
		if err != nil {
			if errors.Is(err, auth.ErrAPIKeyNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "api key not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to revoke api key")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type adminCreateAPIKeyRequest struct {
	UserID        string   `json:"userId"`
	Name          string   `json:"name"`
	Scope         string   `json:"scope"`         // "*", "readonly", "readwrite"; defaults to "*"
	AllowedTables []string `json:"allowedTables"` // empty = all tables
	AppID         *string  `json:"appId"`         // nil = user-scoped key; non-nil = app-scoped key
}

type adminCreateAPIKeyResponse struct {
	Key    string       `json:"key"`
	APIKey *auth.APIKey `json:"apiKey"`
}

// handleAdminCreateAPIKey creates an API key for any user (admin-only).
func handleAdminCreateAPIKey(svc apiKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminCreateAPIKeyRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.UserID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "userId is required")
			return
		}
		if !httputil.IsValidUUID(req.UserID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid userId format")
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.AppID != nil && !httputil.IsValidUUID(*req.AppID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid appId format")
			return
		}

		opts := auth.CreateAPIKeyOptions{
			Scope:         req.Scope,
			AllowedTables: req.AllowedTables,
			AppID:         req.AppID,
		}

		plaintext, key, err := svc.CreateAPIKey(r.Context(), req.UserID, req.Name, opts)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidScope) {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			if errors.Is(err, auth.ErrInvalidAppID) {
				httputil.WriteError(w, http.StatusBadRequest, "app not found")
				return
			}
			if errors.Is(err, auth.ErrUserNotFound) {
				httputil.WriteError(w, http.StatusBadRequest, "user not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create api key")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, adminCreateAPIKeyResponse{Key: plaintext, APIKey: key})
	}
}

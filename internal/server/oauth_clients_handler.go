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

// oauthClientManager is the interface for admin OAuth client operations.
type oauthClientManager interface {
	RegisterOAuthClient(ctx context.Context, appID, name, clientType string, redirectURIs, scopes []string) (string, *auth.OAuthClient, error)
	GetOAuthClient(ctx context.Context, clientID string) (*auth.OAuthClient, error)
	GetOAuthClientByUUID(ctx context.Context, id string) (*auth.OAuthClient, error)
	ListOAuthClients(ctx context.Context, page, perPage int) (*auth.OAuthClientListResult, error)
	UpdateOAuthClient(ctx context.Context, clientID, name string, redirectURIs, scopes []string) (*auth.OAuthClient, error)
	RevokeOAuthClient(ctx context.Context, clientID string) error
	RegenerateOAuthClientSecret(ctx context.Context, clientID string) (string, error)
}

type createOAuthClientRequest struct {
	AppID        string   `json:"appId"`
	Name         string   `json:"name"`
	ClientType   string   `json:"clientType"`
	RedirectURIs []string `json:"redirectUris"`
	Scopes       []string `json:"scopes"`
}

type createOAuthClientResponse struct {
	ClientSecret string            `json:"clientSecret,omitempty"` // shown once for confidential clients
	Client       *auth.OAuthClient `json:"client"`
}

type rotateSecretResponse struct {
	ClientSecret string `json:"clientSecret"` // new secret, shown once
}

// handleAdminListOAuthClients returns a paginated list of OAuth clients.
func handleAdminListOAuthClients(svc oauthClientManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		result, err := svc.ListOAuthClients(r.Context(), page, perPage)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list oauth clients")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

// handleAdminGetOAuthClient returns a single OAuth client by client_id.
func handleAdminGetOAuthClient(svc oauthClientManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := chi.URLParam(r, "clientId")
		if clientID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "client_id is required")
			return
		}

		client, err := svc.GetOAuthClient(r.Context(), clientID)
		if err != nil {
			if errors.Is(err, auth.ErrOAuthClientNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "oauth client not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get oauth client")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, client)
	}
}

// handleAdminCreateOAuthClient registers a new OAuth client.
func handleAdminCreateOAuthClient(svc oauthClientManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createOAuthClientRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.AppID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "appId is required")
			return
		}
		if !httputil.IsValidUUID(req.AppID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid appId format")
			return
		}
		if req.ClientType == "" {
			req.ClientType = auth.OAuthClientTypeConfidential
		}
		if err := auth.ValidateClientType(req.ClientType); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := auth.ValidateRedirectURIs(req.RedirectURIs); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := auth.ValidateOAuthScopes(req.Scopes); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		secret, client, err := svc.RegisterOAuthClient(r.Context(), req.AppID, req.Name, req.ClientType, req.RedirectURIs, req.Scopes)
		if err != nil {
			if errors.Is(err, auth.ErrAppNotFound) {
				httputil.WriteError(w, http.StatusBadRequest, "app not found")
				return
			}
			if errors.Is(err, auth.ErrOAuthClientNameRequired) {
				httputil.WriteError(w, http.StatusBadRequest, "name is required")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create oauth client")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, createOAuthClientResponse{
			ClientSecret: secret,
			Client:       client,
		})
	}
}

// handleAdminRevokeOAuthClient soft-deletes an OAuth client.
func handleAdminRevokeOAuthClient(svc oauthClientManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := chi.URLParam(r, "clientId")
		if clientID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "client_id is required")
			return
		}

		err := svc.RevokeOAuthClient(r.Context(), clientID)
		if err != nil {
			if errors.Is(err, auth.ErrOAuthClientNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "oauth client not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to revoke oauth client")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleAdminRotateOAuthClientSecret regenerates the client secret.
func handleAdminRotateOAuthClientSecret(svc oauthClientManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := chi.URLParam(r, "clientId")
		if clientID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "client_id is required")
			return
		}

		newSecret, err := svc.RegenerateOAuthClientSecret(r.Context(), clientID)
		if err != nil {
			if errors.Is(err, auth.ErrOAuthClientNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "oauth client not found")
				return
			}
			if errors.Is(err, auth.ErrOAuthClientRevoked) {
				httputil.WriteError(w, http.StatusBadRequest, "oauth client has been revoked")
				return
			}
			if errors.Is(err, auth.ErrOAuthClientPublicSecretRotator) {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to rotate oauth client secret")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, rotateSecretResponse{ClientSecret: newSecret})
	}
}

type updateOAuthClientRequest struct {
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirectUris"`
	Scopes       []string `json:"scopes"`
}

// handleAdminUpdateOAuthClient updates an OAuth client's name, redirect URIs, and scopes.
func handleAdminUpdateOAuthClient(svc oauthClientManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := chi.URLParam(r, "clientId")
		if clientID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "client_id is required")
			return
		}

		var req updateOAuthClientRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := auth.ValidateRedirectURIs(req.RedirectURIs); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := auth.ValidateOAuthScopes(req.Scopes); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		client, err := svc.UpdateOAuthClient(r.Context(), clientID, req.Name, req.RedirectURIs, req.Scopes)
		if err != nil {
			if errors.Is(err, auth.ErrOAuthClientNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "oauth client not found")
				return
			}
			if errors.Is(err, auth.ErrOAuthClientRevoked) {
				httputil.WriteError(w, http.StatusBadRequest, "oauth client has been revoked")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update oauth client")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, client)
	}
}

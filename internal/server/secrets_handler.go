// Package server Secrets handler provides HTTP handlers for managing vault secrets, supporting create, read, update, and delete operations through admin endpoints.
package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/vault"
	"github.com/go-chi/chi/v5"
)

// VaultSecretStore is the storage contract used by admin vault secret handlers.
type VaultSecretStore interface {
	ListSecrets(ctx context.Context) ([]vault.SecretMetadata, error)
	GetSecret(ctx context.Context, name string) ([]byte, error)
	CreateSecret(ctx context.Context, name string, value []byte) error
	UpdateSecret(ctx context.Context, name string, value []byte) error
	DeleteSecret(ctx context.Context, name string) error
}

type createSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type updateSecretRequest struct {
	Value string `json:"value"`
}

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	if s.vaultStore == nil {
		serviceUnavailable(w, serviceUnavailableVaultSecrets)
		return
	}

	secrets, err := s.vaultStore.ListSecrets(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list secrets")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, secrets)
}

// Retrieves a vault secret by name from the URL parameter. Returns the secret value with HTTP 200, or returns 404 if not found, 400 for invalid names, or 503 if vault secrets management is not enabled.
func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	if s.vaultStore == nil {
		serviceUnavailable(w, serviceUnavailableVaultSecrets)
		return
	}

	name, err := vault.NormalizeSecretName(chi.URLParam(r, "name"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
		return
	}

	value, err := s.vaultStore.GetSecret(r.Context(), name)
	if err != nil {
		if errors.Is(err, vault.ErrSecretNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "secret not found")
			return
		}
		if errors.Is(err, vault.ErrInvalidSecretName) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get secret")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"name": name, "value": string(value)})
}

// Creates a new vault secret from a JSON request containing name and value. Returns HTTP 201 with the secret name on success, 409 if the secret already exists, 400 for invalid names, or 503 if vault secrets management is not enabled.
func (s *Server) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	if s.vaultStore == nil {
		serviceUnavailable(w, serviceUnavailableVaultSecrets)
		return
	}

	var req createSecretRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	name, err := vault.NormalizeSecretName(req.Name)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
		return
	}

	if err := s.vaultStore.CreateSecret(r.Context(), name, []byte(req.Value)); err != nil {
		if errors.Is(err, vault.ErrSecretAlreadyExists) {
			httputil.WriteError(w, http.StatusConflict, "secret already exists")
			return
		}
		if errors.Is(err, vault.ErrInvalidSecretName) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create secret")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"name": name})
}

// Updates an existing vault secret's value from a JSON request. Normalizes the secret name from the URL parameter and updates the vault. Returns HTTP 200 with the secret name on success, 404 if not found, 400 for invalid names, or 503 if vault secrets management is not enabled.
func (s *Server) handleUpdateSecret(w http.ResponseWriter, r *http.Request) {
	if s.vaultStore == nil {
		serviceUnavailable(w, serviceUnavailableVaultSecrets)
		return
	}

	name, err := vault.NormalizeSecretName(chi.URLParam(r, "name"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
		return
	}

	var req updateSecretRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := s.vaultStore.UpdateSecret(r.Context(), name, []byte(req.Value)); err != nil {
		if errors.Is(err, vault.ErrSecretNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "secret not found")
			return
		}
		if errors.Is(err, vault.ErrInvalidSecretName) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to update secret")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"name": name})
}

// Deletes a vault secret by name. Normalizes the secret name from the URL parameter and removes it from the vault. Returns HTTP 204 No Content on success, 404 if not found, 400 for invalid names, or 503 if vault secrets management is not enabled.
func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if s.vaultStore == nil {
		serviceUnavailable(w, serviceUnavailableVaultSecrets)
		return
	}

	name, err := vault.NormalizeSecretName(chi.URLParam(r, "name"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
		return
	}

	if err := s.vaultStore.DeleteSecret(r.Context(), name); err != nil {
		if errors.Is(err, vault.ErrSecretNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "secret not found")
			return
		}
		if errors.Is(err, vault.ErrInvalidSecretName) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid secret name")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete secret")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

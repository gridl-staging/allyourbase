package auth

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type createAPIKeyRequest struct {
	Name          string   `json:"name"`
	Scope         string   `json:"scope"`         // "*", "readonly", "readwrite"; defaults to "*"
	AllowedTables []string `json:"allowedTables"` // empty = all tables
}

type createAPIKeyResponse struct {
	Key    string  `json:"key"` // plaintext, shown once
	APIKey *APIKey `json:"apiKey"`
}

func (h *Handler) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createAPIKeyRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	opts := CreateAPIKeyOptions{
		Scope:         req.Scope,
		AllowedTables: req.AllowedTables,
	}

	plaintext, key, err := h.auth.CreateAPIKey(r.Context(), claims.Subject, req.Name, opts)
	if err != nil {
		if err == ErrInvalidScope {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, err.Error(),
				"https://allyourbase.io/guide/api-reference")
			return
		}
		h.logger.Error("create api key error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, createAPIKeyResponse{Key: plaintext, APIKey: key})
}

func (h *Handler) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	keys, err := h.auth.ListAPIKeys(r.Context(), claims.Subject)
	if err != nil {
		h.logger.Error("list api keys error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list api keys")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, keys)
}

func (h *Handler) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "api key id is required")
		return
	}
	if !httputil.IsValidUUID(id) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid api key id format")
		return
	}

	err := h.auth.RevokeAPIKey(r.Context(), id, claims.Subject)
	if err != nil {
		if err == ErrAPIKeyNotFound {
			httputil.WriteError(w, http.StatusNotFound, "api key not found")
			return
		}
		h.logger.Error("revoke api key error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

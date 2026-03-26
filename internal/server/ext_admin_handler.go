// Package server Implements HTTP handlers for the admin extension management API, supporting listing, enabling, and disabling extensions.
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/extensions"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// extensionAdmin is the interface the extension admin handlers need.
// extensions.Service satisfies this.
type extensionAdmin interface {
	List(ctx context.Context) ([]extensions.ExtensionInfo, error)
	Enable(ctx context.Context, name string) error
	Disable(ctx context.Context, name string) error
}

// Serves an HTTP endpoint that returns a JSON list of all available extensions and their total count. Returns HTTP 200 with an empty extensions list if extension management is not configured, and HTTP 500 on service errors.
func (s *Server) handleAdminExtensionList(w http.ResponseWriter, r *http.Request) {
	if s.extService == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"extensions": []extensions.ExtensionInfo{}, "total": 0})
		return
	}

	exts, err := s.extService.List(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if exts == nil {
		exts = []extensions.ExtensionInfo{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"extensions": exts, "total": len(exts)})
}

type enableExtensionRequest struct {
	Name string `json:"name"`
}

// Serves an HTTP endpoint that enables an extension specified in the request body. Expects a JSON object with a name field. Returns HTTP 200 with confirmation on success, HTTP 400 for invalid names, HTTP 404 if the extension is not available, and HTTP 500 on other errors. Returns HTTP 503 if extension management is not configured.
func (s *Server) handleAdminExtensionEnable(w http.ResponseWriter, r *http.Request) {
	if s.extService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "extension management not configured")
		return
	}

	var req enableExtensionRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := s.extService.Enable(r.Context(), req.Name); err != nil {
		if strings.Contains(err.Error(), "invalid characters") || strings.Contains(err.Error(), "must not be empty") || strings.Contains(err.Error(), "must not exceed") {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not available") {
			httputil.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"name": req.Name, "enabled": true})
}

// Serves an HTTP endpoint that disables an extension specified by the name URL parameter. Returns HTTP 204 No Content on success, HTTP 400 if the extension name parameter is missing, HTTP 409 if dependent objects prevent disabling, and HTTP 500 on other errors. Returns HTTP 503 if extension management is not configured.
func (s *Server) handleAdminExtensionDisable(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "extension name is required")
		return
	}
	if s.extService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "extension management not configured")
		return
	}

	if err := s.extService.Disable(r.Context(), name); err != nil {
		if strings.Contains(err.Error(), "invalid characters") || strings.Contains(err.Error(), "must not be empty") || strings.Contains(err.Error(), "must not exceed") {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "dependent objects") {
			httputil.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

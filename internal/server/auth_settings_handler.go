package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
)

// handleAdminAuthSettingsGet returns the current auth feature toggle states.
func (s *Server) handleAdminAuthSettingsGet(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		httputil.WriteError(w, http.StatusNotFound, "auth is not enabled")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s.authHandler.GetAuthSettings())
}

// handleAdminAuthSettingsUpdate updates auth feature toggles at runtime.
func (s *Server) handleAdminAuthSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		httputil.WriteError(w, http.StatusNotFound, "auth is not enabled")
		return
	}

	var req auth.AuthSettings
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	s.authHandler.UpdateAuthSettings(req)

	httputil.WriteJSON(w, http.StatusOK, s.authHandler.GetAuthSettings())
}

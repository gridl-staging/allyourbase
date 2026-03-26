package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

func validateOwnerUserID(w http.ResponseWriter, ownerUserID string) bool {
	if ownerUserID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ownerUserId is required")
		return false
	}
	if !httputil.IsValidUUID(ownerUserID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid ownerUserId format")
		return false
	}
	return true
}

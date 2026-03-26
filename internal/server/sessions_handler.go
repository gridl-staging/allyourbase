// Package server Provides HTTP handlers for administrators to manage user sessions, including listing active sessions and revoking individual or all sessions for a user.
package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
)

type userSessionManager interface {
	ListSessions(ctx context.Context, userID, currentSessionID string) ([]auth.SessionInfo, error)
	RevokeSession(ctx context.Context, userID, sessionID string) error
	RevokeAllSessions(ctx context.Context, userID string) error
}

// Returns an HTTP handler that lists all active sessions for a user, identified by the user ID URL parameter, and responds with the session information as JSON.
func handleAdminListUserSessions(svc userSessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUUIDParamWithLabel(w, r, "id", "user id")
		if !ok {
			return
		}

		sessions, err := svc.ListSessions(r.Context(), userID.String(), "")
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list sessions")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, sessions)
	}
}

// Returns an HTTP handler that revokes a specific session for a user, identified by both user ID and session ID URL parameters, and responds with 204 No Content on success.
func handleAdminDeleteUserSession(svc userSessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUUIDParamWithLabel(w, r, "id", "user id")
		if !ok {
			return
		}

		sessionID, ok := parseUUIDParamWithLabel(w, r, "session_id", "session id")
		if !ok {
			return
		}

		err := svc.RevokeSession(r.Context(), userID.String(), sessionID.String())
		if err != nil {
			if errors.Is(err, auth.ErrSessionNotFound) || errors.Is(err, auth.ErrSessionForbidden) {
				httputil.WriteError(w, http.StatusNotFound, "session not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to revoke session")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// Returns an HTTP handler that revokes all sessions for a user, identified by the user ID URL parameter, and responds with 204 No Content on success.
func handleAdminDeleteAllUserSessions(svc userSessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUUIDParamWithLabel(w, r, "id", "user id")
		if !ok {
			return
		}

		if err := svc.RevokeAllSessions(r.Context(), userID.String()); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to revoke sessions")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

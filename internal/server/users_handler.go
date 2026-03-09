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

// userManager is the interface for user management operations.
// auth.Service satisfies this interface.
type userManager interface {
	ListUsers(ctx context.Context, page, perPage int, search string) (*auth.UserListResult, error)
	DeleteUser(ctx context.Context, id string) error
}

// handleAdminListUsers returns a paginated list of auth users.
func handleAdminListUsers(svc userManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
		search := r.URL.Query().Get("search")

		result, err := svc.ListUsers(r.Context(), page, perPage, search)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list users")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

// handleAdminDeleteUser deletes a user by ID.
func handleAdminDeleteUser(svc userManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			httputil.WriteError(w, http.StatusBadRequest, "user id is required")
			return
		}
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid user id format")
			return
		}

		err := svc.DeleteUser(r.Context(), id)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "user not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

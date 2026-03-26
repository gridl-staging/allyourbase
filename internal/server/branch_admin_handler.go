// Package server branch_admin_handler.go contains HTTP handlers for branch management operations, defining endpoints for listing, creating, and deleting branches through a branch service interface.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/branching"
	"github.com/allyourbase/ayb/internal/httputil"
)

// branchAdmin is the interface the branch admin handlers need.
type branchAdmin interface {
	Create(ctx context.Context, name, sourceURL string) (*branching.BranchRecord, error)
	Delete(ctx context.Context, name string) error
	List(ctx context.Context) ([]branching.BranchRecord, error)
}

// handleAdminBranchList is an HTTP handler that returns a list of all configured branches as JSON. If the branch service is not configured, an empty list is returned with a 200 OK status.
func (s *Server) handleAdminBranchList(w http.ResponseWriter, r *http.Request) {
	if s.branchService == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"branches": []branching.BranchRecord{}})
		return
	}

	records, err := s.branchService.List(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if records == nil {
		records = []branching.BranchRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"branches": records})
}

// handleAdminBranchCreate is an HTTP handler that creates a new branch from a JSON request body containing name and from fields. It validates the branch name and returns the created branch record with a 201 Created status. Returns 409 Conflict if the branch already exists.
func (s *Server) handleAdminBranchCreate(w http.ResponseWriter, r *http.Request) {
	if s.branchService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "branch service not configured")
		return
	}

	var body struct {
		Name string `json:"name"`
		From string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := branching.ValidateBranchName(body.Name); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	rec, err := s.branchService.Create(r.Context(), body.Name, body.From)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			httputil.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, rec)
}

// handleAdminBranchDelete is an HTTP handler that deletes a branch by name extracted from the URL path. Returns 200 OK on success or 404 Not Found if the branch does not exist.
func (s *Server) handleAdminBranchDelete(w http.ResponseWriter, r *http.Request) {
	if s.branchService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "branch service not configured")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "branch name required")
		return
	}

	err := s.branchService.Delete(r.Context(), name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

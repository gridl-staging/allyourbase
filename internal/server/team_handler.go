// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/team_handler.go.
package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type createTeamRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type updateTeamRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
}

type teamListResult struct {
	Items []tenant.Team `json:"items"`
}

func teamIDFromURL(r *http.Request, w http.ResponseWriter) (string, bool) {
	teamID := chi.URLParam(r, "teamId")
	if teamID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "team id is required")
		return "", false
	}
	return teamID, true
}

// TODO: Document lookupTeamInOrg.
func lookupTeamInOrg(r *http.Request, w http.ResponseWriter, store tenant.TeamStore, orgID string) (*tenant.Team, bool) {
	teamID, ok := teamIDFromURL(r, w)
	if !ok {
		return nil, false
	}
	team, err := store.GetTeam(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, tenant.ErrTeamNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "team not found")
			return nil, false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get team")
		return nil, false
	}
	if team.OrgID != orgID {
		httputil.WriteError(w, http.StatusNotFound, "team not found")
		return nil, false
	}
	return team, true
}

func (s *Server) orgTeamStoreHandler(handler func(tenant.OrgStore, tenant.TeamStore) http.HandlerFunc) http.HandlerFunc {
	return s.orgAndTeamStoreHandler(handler)
}

// TODO: Document handleAdminCreateTeam.
func handleAdminCreateTeam(orgStore tenant.OrgStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := lookupOrg(r, w, orgStore)
		if !ok {
			return
		}

		var req createTeamRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Slug = strings.TrimSpace(req.Slug)
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Slug == "" {
			httputil.WriteError(w, http.StatusBadRequest, "slug is required")
			return
		}
		if !tenant.IsValidSlug(req.Slug) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
			return
		}

		createdTeam, err := teamStore.CreateTeam(r.Context(), org.ID, req.Name, req.Slug)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			if errors.Is(err, tenant.ErrTeamSlugTaken) {
				httputil.WriteError(w, http.StatusConflict, "team slug is already taken")
				return
			}
			if errors.Is(err, tenant.ErrInvalidSlug) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create team")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, createdTeam)
	}
}

// TODO: Document handleAdminListTeams.
func handleAdminListTeams(orgStore tenant.OrgStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := lookupOrg(r, w, orgStore)
		if !ok {
			return
		}

		teams, err := teamStore.ListTeams(r.Context(), org.ID)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list teams")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, teamListResult{Items: teams})
	}
}

func handleAdminGetTeam(orgStore tenant.OrgStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromURL(r, w)
		if !ok {
			return
		}
		team, ok := lookupTeamInOrg(r, w, teamStore, orgID)
		if !ok {
			return
		}
		httputil.WriteJSON(w, http.StatusOK, team)
	}
}

// TODO: Document handleAdminUpdateTeam.
func handleAdminUpdateTeam(orgStore tenant.OrgStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromURL(r, w)
		if !ok {
			return
		}
		if _, ok := lookupTeamInOrg(r, w, teamStore, orgID); !ok {
			return
		}

		teamID, ok := teamIDFromURL(r, w)
		if !ok {
			return
		}

		var req updateTeamRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Name != nil {
			trimmedName := strings.TrimSpace(*req.Name)
			if trimmedName == "" {
				httputil.WriteError(w, http.StatusBadRequest, "name cannot be empty")
				return
			}
			req.Name = &trimmedName
		}
		if req.Slug != nil {
			trimmedSlug := strings.TrimSpace(*req.Slug)
			if !tenant.IsValidSlug(trimmedSlug) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
				return
			}
			req.Slug = &trimmedSlug
		}

		updatedTeam, err := teamStore.UpdateTeam(r.Context(), teamID, tenant.TeamUpdate{Name: req.Name, Slug: req.Slug})
		if err != nil {
			if errors.Is(err, tenant.ErrTeamNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "team not found")
				return
			}
			if errors.Is(err, tenant.ErrTeamSlugTaken) {
				httputil.WriteError(w, http.StatusConflict, "team slug is already taken")
				return
			}
			if errors.Is(err, tenant.ErrInvalidSlug) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update team")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, updatedTeam)
	}
}

// TODO: Document handleAdminDeleteTeam.
func handleAdminDeleteTeam(orgStore tenant.OrgStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromURL(r, w)
		if !ok {
			return
		}
		if _, ok := lookupTeamInOrg(r, w, teamStore, orgID); !ok {
			return
		}

		teamID, ok := teamIDFromURL(r, w)
		if !ok {
			return
		}
		err := teamStore.DeleteTeam(r.Context(), teamID)
		if err != nil {
			if errors.Is(err, tenant.ErrTeamNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "team not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete team")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/team_membership_handler.go.
package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

type teamMembershipListResult struct {
	Items []tenant.TeamMembership `json:"items"`
}

func orgIDFromTeamMembershipRoute(r *http.Request, w http.ResponseWriter) (string, bool) {
	return requiredUUIDRouteParam(r, w, "orgId", "org id")
}

func teamIDFromTeamMembershipRoute(r *http.Request, w http.ResponseWriter) (string, bool) {
	return requiredUUIDRouteParam(r, w, "teamId", "team id")
}

func writeTeamNotFoundError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, tenant.ErrTeamNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "team not found")
		return true
	}
	return false
}

func writeTeamMembershipNotFoundError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, tenant.ErrTeamMembershipNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "team membership not found")
		return true
	}
	return false
}

// TODO: Document Server.teamMembershipStoresHandler.
func (s *Server) teamMembershipStoresHandler(handler func(tenant.TeamMembershipStore, tenant.OrgMembershipStore, tenant.TeamStore) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.teamMembershipStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "team membership store not configured")
			return
		}
		if s.orgMembershipStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org membership store not configured")
			return
		}
		if s.teamStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "team store not configured")
			return
		}
		handler(s.teamMembershipStore, s.orgMembershipStore, s.teamStore).ServeHTTP(w, r)
	}
}

func (s *Server) teamMembershipStoreAndTeamStoreHandler(handler func(tenant.TeamMembershipStore, tenant.TeamStore) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.teamMembershipStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "team membership store not configured")
			return
		}
		if s.teamStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "team store not configured")
			return
		}
		handler(s.teamMembershipStore, s.teamStore).ServeHTTP(w, r)
	}
}

// TODO: Document lookupTeamForMembershipRoute.
func lookupTeamForMembershipRoute(r *http.Request, w http.ResponseWriter, teamStore tenant.TeamStore) (*tenant.Team, bool) {
	orgID, ok := orgIDFromTeamMembershipRoute(r, w)
	if !ok {
		return nil, false
	}
	teamID, ok := teamIDFromTeamMembershipRoute(r, w)
	if !ok {
		return nil, false
	}
	team, err := teamStore.GetTeam(r.Context(), teamID)
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

// TODO: Document handleAdminAddTeamMember.
func handleAdminAddTeamMember(teamMembershipStore tenant.TeamMembershipStore, orgMembershipStore tenant.OrgMembershipStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		team, ok := lookupTeamForMembershipRoute(r, w, teamStore)
		if !ok {
			return
		}

		var req addMemberRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.UserID = strings.TrimSpace(req.UserID)
		req.Role = strings.TrimSpace(req.Role)
		if req.UserID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "userId is required")
			return
		}
		if !httputil.IsValidUUID(req.UserID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid user id format")
			return
		}
		if req.Role == "" {
			httputil.WriteError(w, http.StatusBadRequest, "role is required")
			return
		}
		if !tenant.IsValidTeamRole(req.Role) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid role")
			return
		}
		if _, err := orgMembershipStore.GetOrgMembership(r.Context(), team.OrgID, req.UserID); err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			if errors.Is(err, tenant.ErrOrgMembershipNotFound) {
				httputil.WriteError(w, http.StatusConflict, "user must be an org member before joining a team")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to validate org membership")
			return
		}

		membership, err := teamMembershipStore.AddTeamMembership(r.Context(), team.ID, req.UserID, req.Role)
		if err != nil {
			if writeTeamNotFoundError(w, err) {
				return
			}
			if errors.Is(err, tenant.ErrTeamMembershipExists) {
				httputil.WriteError(w, http.StatusConflict, "team membership already exists")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to add team member")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, membership)
	}
}

// TODO: Document handleAdminListTeamMembers.
func handleAdminListTeamMembers(teamMembershipStore tenant.TeamMembershipStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		team, ok := lookupTeamForMembershipRoute(r, w, teamStore)
		if !ok {
			return
		}
		memberships, err := teamMembershipStore.ListTeamMemberships(r.Context(), team.ID)
		if err != nil {
			if writeTeamNotFoundError(w, err) {
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list team members")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, teamMembershipListResult{Items: memberships})
	}
}

// TODO: Document handleAdminUpdateTeamMemberRole.
func handleAdminUpdateTeamMemberRole(teamMembershipStore tenant.TeamMembershipStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		team, ok := lookupTeamForMembershipRoute(r, w, teamStore)
		if !ok {
			return
		}
		userID, ok := requiredUUIDRouteParam(r, w, "userId", "user id")
		if !ok {
			return
		}

		var req updateRoleRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.Role = strings.TrimSpace(req.Role)
		if req.Role == "" {
			httputil.WriteError(w, http.StatusBadRequest, "role is required")
			return
		}
		if !tenant.IsValidTeamRole(req.Role) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid role")
			return
		}

		membership, err := teamMembershipStore.UpdateTeamMembershipRole(r.Context(), team.ID, userID, req.Role)
		if err != nil {
			if writeTeamNotFoundError(w, err) {
				return
			}
			if writeTeamMembershipNotFoundError(w, err) {
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update team member role")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, membership)
	}
}

// TODO: Document handleAdminRemoveTeamMember.
func handleAdminRemoveTeamMember(teamMembershipStore tenant.TeamMembershipStore, teamStore tenant.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		team, ok := lookupTeamForMembershipRoute(r, w, teamStore)
		if !ok {
			return
		}
		userID, ok := requiredUUIDRouteParam(r, w, "userId", "user id")
		if !ok {
			return
		}

		err := teamMembershipStore.RemoveTeamMembership(r.Context(), team.ID, userID)
		if err != nil {
			if writeTeamNotFoundError(w, err) {
				return
			}
			if writeTeamMembershipNotFoundError(w, err) {
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to remove team member")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

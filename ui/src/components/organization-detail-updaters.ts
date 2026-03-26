/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/organization-detail-updaters.ts.
 */
import type { OrgMemberRole, Team, TeamMemberRole, TeamMembership } from "../types/organizations";
import type { Tenant } from "../types/tenants";
import type { OrgDetailState } from "./organizations-hooks";

export function applyCreatedOrgMember(state: OrgDetailState, membership: OrgDetailState["members"][number]): OrgDetailState {
  return {
    ...state,
    members: [...state.members, membership],
    error: null,
  };
}

export function applyDeletedOrgMember(state: OrgDetailState, userId: string): OrgDetailState {
  return {
    ...state,
    members: state.members.filter((member) => member.userId !== userId),
    error: null,
  };
}

export function applyUpdatedOrgMemberRole(state: OrgDetailState, userId: string, role: OrgMemberRole): OrgDetailState {
  return {
    ...state,
    members: state.members.map((member) => (member.userId === userId ? { ...member, role } : member)),
    error: null,
  };
}

export function applyCreatedTeam(state: OrgDetailState, team: Team): OrgDetailState {
  return {
    ...state,
    org: state.org ? { ...state.org, teamCount: state.org.teamCount + 1 } : state.org,
    teams: [...state.teams, team],
    error: null,
  };
}

export function applyUpdatedTeam(state: OrgDetailState, teamId: string, updatedTeam: Team): OrgDetailState {
  return {
    ...state,
    teams: state.teams.map((team) => (team.id === teamId ? updatedTeam : team)),
    error: null,
  };
}

export function applyDeletedTeam(state: OrgDetailState, teamId: string): OrgDetailState {
  const nextTeamMembersByTeamId = { ...state.teamMembersByTeamId };
  delete nextTeamMembersByTeamId[teamId];

  return {
    ...state,
    org: state.org ? { ...state.org, teamCount: Math.max(0, state.org.teamCount - 1) } : state.org,
    teams: state.teams.filter((team) => team.id !== teamId),
    teamMembersByTeamId: nextTeamMembersByTeamId,
    error: null,
  };
}

export function applyCreatedTeamMember(
  state: OrgDetailState,
  teamId: string,
  membership: TeamMembership,
): OrgDetailState {
  const existing = state.teamMembersByTeamId[teamId] ?? [];
  return {
    ...state,
    teamMembersByTeamId: { ...state.teamMembersByTeamId, [teamId]: [...existing, membership] },
    error: null,
  };
}

export function applyDeletedTeamMember(state: OrgDetailState, teamId: string, userId: string): OrgDetailState {
  const existing = state.teamMembersByTeamId[teamId] ?? [];
  return {
    ...state,
    teamMembersByTeamId: {
      ...state.teamMembersByTeamId,
      [teamId]: existing.filter((member) => member.userId !== userId),
    },
    error: null,
  };
}

/**
 * TODO: Document applyUpdatedTeamMemberRole.
 */
export function applyUpdatedTeamMemberRole(
  state: OrgDetailState,
  teamId: string,
  userId: string,
  role: TeamMemberRole,
): OrgDetailState {
  const existing = state.teamMembersByTeamId[teamId] ?? [];
  return {
    ...state,
    teamMembersByTeamId: {
      ...state.teamMembersByTeamId,
      [teamId]: existing.map((member) => (member.userId === userId ? { ...member, role } : member)),
    },
    error: null,
  };
}

export function applyTenantAssignment(state: OrgDetailState, tenants: Tenant[]): OrgDetailState {
  return {
    ...state,
    org: state.org ? { ...state.org, tenantCount: tenants.length } : state.org,
    tenants,
    error: null,
  };
}

export function applyTenantUnassignment(state: OrgDetailState, tenantId: string): OrgDetailState {
  return {
    ...state,
    org: state.org ? { ...state.org, tenantCount: Math.max(0, state.org.tenantCount - 1) } : state.org,
    tenants: state.tenants.filter((tenant) => tenant.id !== tenantId),
    error: null,
  };
}

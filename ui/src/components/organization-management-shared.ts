import type { Dispatch, SetStateAction } from "react";
import { ORG_MEMBER_ROLE_VALUES, TEAM_MEMBER_ROLE_VALUES } from "../types/organizations";
import type { OrgMemberRole, TeamMemberRole } from "../types/organizations";
import type { OrgDetailState } from "./organizations-hooks";

export interface UseOrgManagementArgs {
  selectedId: string | null;
  selectedTeamId: string | null;
  detail: OrgDetailState;
  setDetail: Dispatch<SetStateAction<OrgDetailState>>;
  refreshList: () => void;
  onOrgDeleted: () => void;
  onOrgCreated: (orgId: string) => void;
  onTeamDeleted: () => void;
}

export function normalizeOrgRole(value: string): OrgMemberRole {
  return ORG_MEMBER_ROLE_VALUES.includes(value as OrgMemberRole) ? (value as OrgMemberRole) : "member";
}

export function normalizeTeamRole(value: string): TeamMemberRole {
  return TEAM_MEMBER_ROLE_VALUES.includes(value as TeamMemberRole) ? (value as TeamMemberRole) : "member";
}

export function teamMemberDraftKey(teamId: string, userId: string): string {
  return `${teamId}:${userId}`;
}

export function toErrorMessage(error: unknown): string {
  return String((error as Error)?.message ?? error);
}

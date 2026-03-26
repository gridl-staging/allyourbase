import type { Tenant, TenantAuditEvent } from "./tenants";

export const ORG_MEMBER_ROLE_VALUES = ["owner", "admin", "member", "viewer"] as const;
export type OrgMemberRole = (typeof ORG_MEMBER_ROLE_VALUES)[number];

export const TEAM_MEMBER_ROLE_VALUES = ["lead", "member"] as const;
export type TeamMemberRole = (typeof TEAM_MEMBER_ROLE_VALUES)[number];

export const ORG_DETAIL_TAB_VALUES = ["info", "members", "teams", "tenants", "usage", "audit"] as const;
export type OrgDetailTab = (typeof ORG_DETAIL_TAB_VALUES)[number];

export const TEAM_DETAIL_TAB_VALUES = ["members"] as const;
export type TeamDetailTab = (typeof TEAM_DETAIL_TAB_VALUES)[number];

export const ORG_USAGE_PERIOD_VALUES = ["day", "week", "month"] as const;
export type OrgUsagePeriod = (typeof ORG_USAGE_PERIOD_VALUES)[number];

export interface Organization {
  id: string;
  name: string;
  slug: string;
  parentOrgId: string | null;
  planTier: string;
  createdAt: string;
  updatedAt: string;
}

export interface OrgDetailResponse extends Organization {
  childOrgCount: number;
  teamCount: number;
  tenantCount: number;
}

export interface CreateOrgRequest {
  name: string;
  slug: string;
  planTier: string;
  parentOrgId?: string;
}

export interface UpdateOrgRequest {
  name?: string;
  slug?: string;
  parentOrgId?: string;
}

export interface OrgListResponse {
  items: Organization[];
}

export interface OrgMembership {
  id: string;
  orgId: string;
  userId: string;
  role: OrgMemberRole;
  createdAt: string;
}

export interface OrgMembershipListResponse {
  items: OrgMembership[];
}

export interface Team {
  id: string;
  orgId: string;
  name: string;
  slug: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateTeamRequest {
  name: string;
  slug: string;
}

export interface UpdateTeamRequest {
  name?: string;
  slug?: string;
}

export interface TeamListResponse {
  items: Team[];
}

export interface TeamMembership {
  id: string;
  teamId: string;
  userId: string;
  role: TeamMemberRole;
  createdAt: string;
}

export interface TeamMemberListResponse {
  items: TeamMembership[];
}

export interface TenantAssignmentResponse {
  status: string;
}

export interface OrgTenantListResponse {
  items: Tenant[];
}

export interface UsageDayEntry {
  date: string;
  apiRequests: number;
  storageBytesUsed: number;
  bandwidthBytes: number;
  functionInvocations: number;
}

export interface UsageTotals {
  apiRequests: number;
  storageBytesUsed: number;
  bandwidthBytes: number;
  functionInvocations: number;
}

export interface OrgUsageSummary {
  orgId: string;
  tenantCount: number;
  period: OrgUsagePeriod;
  data: UsageDayEntry[];
  totals: UsageTotals;
}

export interface OrgUsageQuery {
  period: OrgUsagePeriod;
  from: string | null;
  to: string | null;
}

export type OrgAuditEvent = TenantAuditEvent;

export interface OrgAuditQuery {
  limit: number;
  offset: number;
  from?: string;
  to?: string;
  action?: string;
  result?: string;
  actorId?: string;
}

export interface OrgAuditResponse {
  items: OrgAuditEvent[];
  count: number;
  limit: number;
  offset: number;
}


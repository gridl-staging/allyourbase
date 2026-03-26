/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_orgs.ts.
 */
import { request, requestNoBody } from "./api_client";
import { asInteger, asRecord, asString, asStringOrNull, withQueryString } from "./lib/normalize";
import type {
  CreateOrgRequest,
  CreateTeamRequest,
  OrgAuditQuery,
  OrgAuditResponse,
  OrgDetailResponse,
  OrgListResponse,
  OrgMemberRole,
  OrgMembership,
  OrgMembershipListResponse,
  Organization,
  OrgTenantListResponse,
  OrgUsagePeriod,
  OrgUsageQuery,
  OrgUsageSummary,
  Team,
  TeamMemberListResponse,
  TeamMemberRole,
  TeamMembership,
  TeamListResponse,
  TenantAssignmentResponse,
  UpdateOrgRequest,
  UpdateTeamRequest,
  UsageDayEntry,
  UsageTotals,
} from "./types/organizations";
import { ORG_MEMBER_ROLE_VALUES, TEAM_MEMBER_ROLE_VALUES } from "./types/organizations";
import { normalizeAuditEvent, normalizeTenant } from "./api_tenants";

function orgPath(orgId: string): string {
  return `/api/admin/orgs/${encodeURIComponent(orgId)}`;
}

function teamPath(orgId: string, teamId: string): string {
  return `${orgPath(orgId)}/teams/${encodeURIComponent(teamId)}`;
}

function asOrgMemberRole(value: unknown): OrgMemberRole {
  return typeof value === "string" && ORG_MEMBER_ROLE_VALUES.includes(value as OrgMemberRole)
    ? (value as OrgMemberRole)
    : "member";
}

function asTeamMemberRole(value: unknown): TeamMemberRole {
  return typeof value === "string" && TEAM_MEMBER_ROLE_VALUES.includes(value as TeamMemberRole)
    ? (value as TeamMemberRole)
    : "member";
}

function asOrgUsagePeriod(value: unknown): OrgUsagePeriod {
  return value === "day" || value === "week" || value === "month" ? value : "month";
}

function normalizeOrganization(raw: unknown): Organization {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    name: asString(r?.name),
    slug: asString(r?.slug),
    parentOrgId: asStringOrNull(r?.parentOrgId),
    planTier: asString(r?.planTier),
    createdAt: asString(r?.createdAt),
    updatedAt: asString(r?.updatedAt),
  };
}

function normalizeOrgDetail(raw: unknown): OrgDetailResponse {
  const r = asRecord(raw);
  return {
    ...normalizeOrganization(r),
    childOrgCount: asInteger(r?.childOrgCount),
    teamCount: asInteger(r?.teamCount),
    tenantCount: asInteger(r?.tenantCount),
  };
}

function normalizeOrgMembership(raw: unknown): OrgMembership {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    orgId: asString(r?.orgId),
    userId: asString(r?.userId),
    role: asOrgMemberRole(r?.role),
    createdAt: asString(r?.createdAt),
  };
}

function normalizeTeam(raw: unknown): Team {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    orgId: asString(r?.orgId),
    name: asString(r?.name),
    slug: asString(r?.slug),
    createdAt: asString(r?.createdAt),
    updatedAt: asString(r?.updatedAt),
  };
}

function normalizeTeamMembership(raw: unknown): TeamMembership {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    teamId: asString(r?.teamId),
    userId: asString(r?.userId),
    role: asTeamMemberRole(r?.role),
    createdAt: asString(r?.createdAt),
  };
}

function normalizeUsageDay(raw: unknown): UsageDayEntry {
  const r = asRecord(raw);
  return {
    date: asString(r?.date),
    apiRequests: asInteger(r?.apiRequests),
    storageBytesUsed: asInteger(r?.storageBytesUsed),
    bandwidthBytes: asInteger(r?.bandwidthBytes),
    functionInvocations: asInteger(r?.functionInvocations),
  };
}

function normalizeUsageTotals(raw: unknown): UsageTotals {
  const r = asRecord(raw);
  return {
    apiRequests: asInteger(r?.apiRequests),
    storageBytesUsed: asInteger(r?.storageBytesUsed),
    bandwidthBytes: asInteger(r?.bandwidthBytes),
    functionInvocations: asInteger(r?.functionInvocations),
  };
}

function applyPeriodOrDateRange(params: URLSearchParams, query: OrgUsageQuery): void {
  if (query.from || query.to) {
    if (query.from) params.set("from", query.from);
    if (query.to) params.set("to", query.to);
    return;
  }
  params.set("period", query.period || "month");
}

export async function fetchOrgList(): Promise<OrgListResponse> {
  const payload = await request<unknown>("/api/admin/orgs");
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return { items: rawItems.map(normalizeOrganization) };
}

export async function createOrg(body: CreateOrgRequest): Promise<Organization> {
  const payload = await request<unknown>("/api/admin/orgs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return normalizeOrganization(payload);
}

export async function getOrg(orgId: string): Promise<OrgDetailResponse> {
  const payload = await request<unknown>(orgPath(orgId));
  return normalizeOrgDetail(payload);
}

export async function updateOrg(orgId: string, body: UpdateOrgRequest): Promise<Organization> {
  const payload = await request<unknown>(orgPath(orgId), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return normalizeOrganization(payload);
}

export async function deleteOrg(orgId: string): Promise<void> {
  await requestNoBody(`${orgPath(orgId)}?confirm=true`, { method: "DELETE" });
}

export async function fetchOrgMembers(orgId: string): Promise<OrgMembershipListResponse> {
  const payload = await request<unknown>(`${orgPath(orgId)}/members`);
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return { items: rawItems.map(normalizeOrgMembership) };
}

export async function addOrgMember(
  orgId: string,
  userId: string,
  role: OrgMemberRole,
): Promise<OrgMembership> {
  const payload = await request<unknown>(`${orgPath(orgId)}/members`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ userId, role }),
  });
  return normalizeOrgMembership(payload);
}

export async function updateOrgMemberRole(
  orgId: string,
  userId: string,
  role: OrgMemberRole,
): Promise<OrgMembership> {
  const payload = await request<unknown>(`${orgPath(orgId)}/members/${encodeURIComponent(userId)}/role`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ role }),
  });
  return normalizeOrgMembership(payload);
}

export async function removeOrgMember(orgId: string, userId: string): Promise<void> {
  await requestNoBody(`${orgPath(orgId)}/members/${encodeURIComponent(userId)}`, {
    method: "DELETE",
  });
}

export async function assignTenantToOrg(orgId: string, tenantId: string): Promise<TenantAssignmentResponse> {
  const payload = await request<unknown>(`${orgPath(orgId)}/tenants`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tenantId }),
  });
  const r = asRecord(payload);
  return { status: asString(r?.status) };
}

export async function fetchOrgTenants(orgId: string): Promise<OrgTenantListResponse> {
  const payload = await request<unknown>(`${orgPath(orgId)}/tenants`);
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return { items: rawItems.map(normalizeTenant) };
}

export async function unassignTenantFromOrg(orgId: string, tenantId: string): Promise<void> {
  await requestNoBody(`${orgPath(orgId)}/tenants/${encodeURIComponent(tenantId)}`, { method: "DELETE" });
}

export async function createTeam(orgId: string, body: CreateTeamRequest): Promise<Team> {
  const payload = await request<unknown>(`${orgPath(orgId)}/teams`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return normalizeTeam(payload);
}

export async function fetchTeams(orgId: string): Promise<TeamListResponse> {
  const payload = await request<unknown>(`${orgPath(orgId)}/teams`);
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return { items: rawItems.map(normalizeTeam) };
}

export async function getTeam(orgId: string, teamId: string): Promise<Team> {
  const payload = await request<unknown>(teamPath(orgId, teamId));
  return normalizeTeam(payload);
}

export async function updateTeam(orgId: string, teamId: string, body: UpdateTeamRequest): Promise<Team> {
  const payload = await request<unknown>(teamPath(orgId, teamId), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return normalizeTeam(payload);
}

export async function deleteTeam(orgId: string, teamId: string): Promise<void> {
  await requestNoBody(teamPath(orgId, teamId), { method: "DELETE" });
}

export async function fetchTeamMembers(orgId: string, teamId: string): Promise<TeamMemberListResponse> {
  const payload = await request<unknown>(`${teamPath(orgId, teamId)}/members`);
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return { items: rawItems.map(normalizeTeamMembership) };
}

export async function addTeamMember(
  orgId: string,
  teamId: string,
  userId: string,
  role: TeamMemberRole,
): Promise<TeamMembership> {
  const payload = await request<unknown>(`${teamPath(orgId, teamId)}/members`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ userId, role }),
  });
  return normalizeTeamMembership(payload);
}

/**
 * TODO: Document updateTeamMemberRole.
 */
export async function updateTeamMemberRole(
  orgId: string,
  teamId: string,
  userId: string,
  role: TeamMemberRole,
): Promise<TeamMembership> {
  const payload = await request<unknown>(
    `${teamPath(orgId, teamId)}/members/${encodeURIComponent(userId)}/role`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ role }),
    },
  );
  return normalizeTeamMembership(payload);
}

export async function removeTeamMember(orgId: string, teamId: string, userId: string): Promise<void> {
  await requestNoBody(`${teamPath(orgId, teamId)}/members/${encodeURIComponent(userId)}`, {
    method: "DELETE",
  });
}

/**
 * TODO: Document fetchOrgUsage.
 */
export async function fetchOrgUsage(orgId: string, query: OrgUsageQuery): Promise<OrgUsageSummary> {
  const params = new URLSearchParams();
  applyPeriodOrDateRange(params, query);

  const payload = await request<unknown>(withQueryString(`${orgPath(orgId)}/usage`, params));
  const r = asRecord(payload);
  const rawDays = Array.isArray(r?.data) ? r.data : [];

  return {
    orgId: asString(r?.orgId),
    tenantCount: asInteger(r?.tenantCount),
    period: asOrgUsagePeriod(r?.period),
    data: rawDays.map(normalizeUsageDay),
    totals: normalizeUsageTotals(r?.totals),
  };
}

/**
 * TODO: Document fetchOrgAudit.
 */
export async function fetchOrgAudit(orgId: string, query: OrgAuditQuery): Promise<OrgAuditResponse> {
  const params = new URLSearchParams();
  params.set("limit", String(query.limit));
  params.set("offset", String(query.offset));
  if (query.from) params.set("from", query.from);
  if (query.to) params.set("to", query.to);
  if (query.action) params.set("action", query.action);
  if (query.result) params.set("result", query.result);
  if (query.actorId) params.set("actor_id", query.actorId);

  const payload = await request<unknown>(withQueryString(`${orgPath(orgId)}/audit`, params));
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];

  return {
    items: rawItems.map(normalizeAuditEvent),
    count: asInteger(r?.count),
    limit: asInteger(r?.limit),
    offset: asInteger(r?.offset),
  };
}

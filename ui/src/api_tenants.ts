/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_tenants.ts.
 */
import { request, requestNoBody } from "./api_client";
import { asBoolean, asInteger, asRecord, asString, asStringOrNull, withQueryString } from "./lib/normalize";
import type {
  BreakerStateResponse,
  CreateTenantRequest,
  Tenant,
  TenantAuditEvent,
  TenantAuditQuery,
  TenantAuditResponse,
  TenantListQuery,
  TenantListResponse,
  TenantMaintenanceState,
  TenantMemberListResponse,
  TenantMembership,
  TenantState,
  UpdateTenantRequest,
} from "./types/tenants";
import { BREAKER_STATE_VALUES, TENANT_STATE_VALUES } from "./types/tenants";

// ---------------------------------------------------------------------------
// Defensive normalization helpers (same pattern as api_usage.ts)
// ---------------------------------------------------------------------------

export function asTenantState(value: unknown): TenantState {
  return typeof value === "string" && TENANT_STATE_VALUES.includes(value as TenantState)
    ? (value as TenantState)
    : "provisioning";
}

function asBreakerState(value: unknown): BreakerStateResponse["state"] {
  return typeof value === "string" &&
    BREAKER_STATE_VALUES.includes(value as BreakerStateResponse["state"])
    ? (value as BreakerStateResponse["state"])
    : "closed";
}

// ---------------------------------------------------------------------------
// Single-object normalizers
// ---------------------------------------------------------------------------

/**
 * TODO: Document normalizeTenant.
 */
export function normalizeTenant(raw: unknown): Tenant {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    name: asString(r?.name),
    slug: asString(r?.slug),
    isolationMode: asString(r?.isolationMode),
    planTier: asString(r?.planTier),
    region: asString(r?.region),
    orgId: asStringOrNull(r?.orgId),
    orgMetadata: r?.orgMetadata ?? null,
    state: asTenantState(r?.state),
    idempotencyKey: asStringOrNull(r?.idempotencyKey),
    createdAt: asString(r?.createdAt),
    updatedAt: asString(r?.updatedAt),
  };
}

function normalizeMembership(raw: unknown): TenantMembership {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    tenantId: asString(r?.tenantId),
    userId: asString(r?.userId),
    role: asString(r?.role),
    createdAt: asString(r?.createdAt),
  };
}

function normalizeMaintenanceState(raw: unknown): TenantMaintenanceState {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    tenantId: asString(r?.tenantId),
    enabled: asBoolean(r?.enabled),
    reason: asStringOrNull(r?.reason),
    enabledAt: asStringOrNull(r?.enabledAt),
    enabledBy: asStringOrNull(r?.enabledBy),
    createdAt: asString(r?.createdAt),
    updatedAt: asString(r?.updatedAt),
  };
}

function normalizeBreakerState(raw: unknown): BreakerStateResponse {
  const r = asRecord(raw);
  return {
    state: asBreakerState(r?.state),
    consecutiveFailures: asInteger(r?.consecutiveFailures),
    halfOpenProbes: asInteger(r?.halfOpenProbes),
  };
}

export function normalizeAuditEvent(raw: unknown): TenantAuditEvent {
  const r = asRecord(raw);
  return {
    id: asString(r?.id),
    tenantId: asString(r?.tenantId),
    actorId: asStringOrNull(r?.actorId),
    action: asString(r?.action),
    result: asString(r?.result),
    metadata: r?.metadata ?? null,
    ipAddress: asStringOrNull(r?.ipAddress),
    createdAt: asString(r?.createdAt),
  };
}

// ---------------------------------------------------------------------------
// Collection normalizers
// ---------------------------------------------------------------------------

export function normalizeTenantListPayload(payload: unknown): TenantListResponse {
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return {
    items: rawItems.map(normalizeTenant),
    page: asInteger(r?.page),
    perPage: asInteger(r?.perPage),
    totalItems: asInteger(r?.totalItems),
    totalPages: asInteger(r?.totalPages),
  };
}

export function normalizeTenantAuditPayload(payload: unknown): TenantAuditResponse {
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return {
    items: rawItems.map(normalizeAuditEvent),
    count: asInteger(r?.count),
    limit: asInteger(r?.limit),
    offset: asInteger(r?.offset),
  };
}

// ---------------------------------------------------------------------------
// URL helpers
// ---------------------------------------------------------------------------

function tenantPath(tenantId: string): string {
  return `/api/admin/tenants/${encodeURIComponent(tenantId)}`;
}

// ---------------------------------------------------------------------------
// CRUD / Lifecycle fetchers (7 endpoints)
// ---------------------------------------------------------------------------

export async function fetchTenantList(query: TenantListQuery): Promise<TenantListResponse> {
  const params = new URLSearchParams();
  params.set("page", String(query.page));
  params.set("perPage", String(query.perPage));
  const payload = await request<unknown>(withQueryString("/api/admin/tenants", params));
  return normalizeTenantListPayload(payload);
}

export async function createTenant(body: CreateTenantRequest): Promise<Tenant> {
  const payload = await request<unknown>("/api/admin/tenants", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return normalizeTenant(payload);
}

export async function getTenant(tenantId: string): Promise<Tenant> {
  const payload = await request<unknown>(tenantPath(tenantId));
  return normalizeTenant(payload);
}

export async function updateTenant(tenantId: string, body: UpdateTenantRequest): Promise<Tenant> {
  const payload = await request<unknown>(tenantPath(tenantId), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return normalizeTenant(payload);
}

export async function deleteTenant(tenantId: string): Promise<Tenant> {
  const payload = await request<unknown>(tenantPath(tenantId), { method: "DELETE" });
  return normalizeTenant(payload);
}

export async function suspendTenant(tenantId: string): Promise<Tenant> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/suspend`, { method: "POST" });
  return normalizeTenant(payload);
}

export async function resumeTenant(tenantId: string): Promise<Tenant> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/resume`, { method: "POST" });
  return normalizeTenant(payload);
}

// ---------------------------------------------------------------------------
// Membership fetchers (4 endpoints)
// ---------------------------------------------------------------------------

export async function fetchTenantMembers(tenantId: string): Promise<TenantMemberListResponse> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/members`);
  const r = asRecord(payload);
  const rawItems = Array.isArray(r?.items) ? r.items : [];
  return { items: rawItems.map(normalizeMembership) };
}

export async function addTenantMember(
  tenantId: string,
  userId: string,
  role: string,
): Promise<TenantMembership> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/members`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ userId, role }),
  });
  return normalizeMembership(payload);
}

export async function updateTenantMemberRole(
  tenantId: string,
  userId: string,
  role: string,
): Promise<TenantMembership> {
  const payload = await request<unknown>(
    `${tenantPath(tenantId)}/members/${encodeURIComponent(userId)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ role }),
    },
  );
  return normalizeMembership(payload);
}

export async function removeTenantMember(tenantId: string, userId: string): Promise<void> {
  await requestNoBody(`${tenantPath(tenantId)}/members/${encodeURIComponent(userId)}`, {
    method: "DELETE",
  });
}

// ---------------------------------------------------------------------------
// Maintenance / Breaker fetchers (5 endpoints)
// ---------------------------------------------------------------------------

export async function fetchMaintenanceState(tenantId: string): Promise<TenantMaintenanceState> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/maintenance`);
  return normalizeMaintenanceState(payload);
}

export async function enableMaintenance(
  tenantId: string,
  reason: string,
): Promise<TenantMaintenanceState> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/maintenance/enable`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reason }),
  });
  return normalizeMaintenanceState(payload);
}

export async function disableMaintenance(tenantId: string): Promise<TenantMaintenanceState> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/maintenance/disable`, {
    method: "POST",
  });
  return normalizeMaintenanceState(payload);
}

export async function fetchBreakerState(tenantId: string): Promise<BreakerStateResponse> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/breaker`);
  return normalizeBreakerState(payload);
}

export async function resetBreaker(tenantId: string): Promise<BreakerStateResponse> {
  const payload = await request<unknown>(`${tenantPath(tenantId)}/breaker/reset`, {
    method: "POST",
  });
  return normalizeBreakerState(payload);
}

// ---------------------------------------------------------------------------
// Audit query fetcher (1 endpoint)
// ---------------------------------------------------------------------------

export async function fetchTenantAudit(
  tenantId: string,
  query: TenantAuditQuery,
): Promise<TenantAuditResponse> {
  const params = new URLSearchParams();
  params.set("limit", String(query.limit));
  params.set("offset", String(query.offset));
  if (query.from) params.set("from", query.from);
  if (query.to) params.set("to", query.to);
  if (query.action) params.set("action", query.action);
  if (query.result) params.set("result", query.result);
  if (query.actorId) params.set("actor_id", query.actorId);
  const payload = await request<unknown>(withQueryString(`${tenantPath(tenantId)}/audit`, params));
  return normalizeTenantAuditPayload(payload);
}

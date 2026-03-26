/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/types/tenants.ts.
 */
export const TENANT_STATE_VALUES = ["provisioning", "active", "suspended", "deleting", "deleted"] as const;
export type TenantState = (typeof TENANT_STATE_VALUES)[number];

export const MEMBER_ROLE_VALUES = ["owner", "admin", "member", "viewer"] as const;
export type MemberRole = (typeof MEMBER_ROLE_VALUES)[number];

export const BREAKER_STATE_VALUES = ["closed", "open", "half_open"] as const;
export type BreakerState = (typeof BREAKER_STATE_VALUES)[number];

export const DETAIL_TAB_VALUES = ["info", "members", "maintenance", "audit"] as const;
export type DetailTab = (typeof DETAIL_TAB_VALUES)[number];

export const CREATE_TENANT_ISOLATION_MODE_OPTIONS = [
  { value: "shared", label: "Shared" },
  { value: "schema", label: "Schema" },
] as const;

export const CREATE_TENANT_PLAN_TIER_OPTIONS = [
  { value: "free", label: "Free" },
  { value: "starter", label: "Starter" },
  { value: "pro", label: "Pro" },
  { value: "enterprise", label: "Enterprise" },
] as const;

export const DEFAULT_CREATE_TENANT_ISOLATION_MODE =
  CREATE_TENANT_ISOLATION_MODE_OPTIONS.find((option) => option.value === "shared")?.value ??
  CREATE_TENANT_ISOLATION_MODE_OPTIONS[0].value;

export const DEFAULT_CREATE_TENANT_PLAN_TIER =
  CREATE_TENANT_PLAN_TIER_OPTIONS.find((option) => option.value === "pro")?.value ??
  CREATE_TENANT_PLAN_TIER_OPTIONS[0].value;

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  isolationMode: string;
  planTier: string;
  region: string;
  orgId: string | null;
  orgMetadata: unknown;
  state: TenantState;
  idempotencyKey: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface TenantListResponse {
  items: Tenant[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface CreateTenantRequest {
  name: string;
  slug: string;
  ownerUserId: string;
  isolationMode: string;
  planTier: string;
  region: string;
  orgMetadata?: unknown;
  idempotencyKey?: string;
}

export interface UpdateTenantRequest {
  name?: string;
  orgMetadata?: unknown;
}

export interface TenantMembership {
  id: string;
  tenantId: string;
  userId: string;
  role: string;
  createdAt: string;
}

export interface TenantMemberListResponse {
  items: TenantMembership[];
}

export interface TenantMaintenanceState {
  id: string;
  tenantId: string;
  enabled: boolean;
  reason: string | null;
  enabledAt: string | null;
  enabledBy: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface BreakerStateResponse {
  state: BreakerState;
  consecutiveFailures: number;
  halfOpenProbes: number;
}

export interface TenantAuditEvent {
  id: string;
  tenantId: string;
  actorId: string | null;
  action: string;
  result: string;
  metadata: unknown;
  ipAddress: string | null;
  createdAt: string;
}

export interface TenantAuditResponse {
  items: TenantAuditEvent[];
  count: number;
  limit: number;
  offset: number;
}

export interface TenantListQuery {
  page: number;
  perPage: number;
}

export interface TenantAuditQuery {
  limit: number;
  offset: number;
  from?: string;
  to?: string;
  action?: string;
  result?: string;
  actorId?: string;
}

/**
 * TODO: Document TenantPageState.
 */
export interface TenantPageState {
  selectedTenantId: string | null;
  activeDetailTab: DetailTab;
  listQuery: TenantListQuery;
  listMeta: { totalItems: number; totalPages: number };
  auditQuery: TenantAuditQuery;
  infoState: { tenant: Tenant | null; isLoading: boolean; error: string | null };
  membersState: { items: TenantMembership[]; isLoading: boolean; error: string | null };
  maintenanceState: {
    state: TenantMaintenanceState | null;
    breaker: BreakerStateResponse | null;
    isLoading: boolean;
    error: string | null;
  };
  auditState: { items: TenantAuditEvent[]; count: number; isLoading: boolean; error: string | null };
}

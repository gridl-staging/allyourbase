/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/__tests__/tenants-test-helpers.ts.
 */
import { beforeEach, vi } from "vitest";
import { screen } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { listUsers } from "../../api_admin";
import {
  addTenantMember,
  createTenant,
  deleteTenant,
  disableMaintenance,
  enableMaintenance,
  fetchBreakerState,
  fetchMaintenanceState,
  fetchTenantAudit,
  fetchTenantList,
  fetchTenantMembers,
  getTenant,
  removeTenantMember,
  resumeTenant,
  suspendTenant,
  updateTenant,
  updateTenantMemberRole,
} from "../../api_tenants";
import type { AdminUser, UserListResponse } from "../../types";
import type {
  BreakerStateResponse,
  CreateTenantRequest,
  Tenant,
  TenantAuditResponse,
  TenantListResponse,
  TenantMaintenanceState,
  TenantMemberListResponse,
} from "../../types/tenants";
import {
  CREATE_TENANT_ISOLATION_MODE_OPTIONS,
  CREATE_TENANT_PLAN_TIER_OPTIONS,
  DEFAULT_CREATE_TENANT_ISOLATION_MODE,
  DEFAULT_CREATE_TENANT_PLAN_TIER,
} from "../../types/tenants";

vi.mock("../../api_tenants", () => ({
  fetchTenantList: vi.fn(),
  createTenant: vi.fn(),
  getTenant: vi.fn(),
  updateTenant: vi.fn(),
  deleteTenant: vi.fn(),
  suspendTenant: vi.fn(),
  resumeTenant: vi.fn(),
  fetchTenantMembers: vi.fn(),
  addTenantMember: vi.fn(),
  updateTenantMemberRole: vi.fn(),
  removeTenantMember: vi.fn(),
  fetchMaintenanceState: vi.fn(),
  enableMaintenance: vi.fn(),
  disableMaintenance: vi.fn(),
  fetchBreakerState: vi.fn(),
  resetBreaker: vi.fn(),
  fetchTenantAudit: vi.fn(),
}));

vi.mock("../../api_admin", () => ({
  listUsers: vi.fn(),
}));

export const mockListUsers = vi.mocked(listUsers);
export const mockFetchTenantList = vi.mocked(fetchTenantList);
export const mockAddTenantMember = vi.mocked(addTenantMember);
export const mockCreateTenant = vi.mocked(createTenant);
export const mockDeleteTenant = vi.mocked(deleteTenant);
export const mockGetTenant = vi.mocked(getTenant);
export const mockRemoveTenantMember = vi.mocked(removeTenantMember);
export const mockSuspendTenant = vi.mocked(suspendTenant);
export const mockResumeTenant = vi.mocked(resumeTenant);
export const mockFetchTenantMembers = vi.mocked(fetchTenantMembers);
export const mockFetchMaintenanceState = vi.mocked(fetchMaintenanceState);
export const mockFetchBreakerState = vi.mocked(fetchBreakerState);
export const mockFetchTenantAudit = vi.mocked(fetchTenantAudit);
export const mockEnableMaintenance = vi.mocked(enableMaintenance);
export const mockDisableMaintenance = vi.mocked(disableMaintenance);
export const mockUpdateTenant = vi.mocked(updateTenant);
export const mockUpdateTenantMemberRole = vi.mocked(updateTenantMemberRole);

export const CREATE_TENANT_DIALOG_DEFAULTS = {
  name: "",
  slug: "",
  ownerUserId: "",
  isolationMode: DEFAULT_CREATE_TENANT_ISOLATION_MODE,
  planTier: DEFAULT_CREATE_TENANT_PLAN_TIER,
  region: "us-east-1",
} as const;

export const CREATE_TENANT_CANONICAL_SELECTIONS = {
  isolationMode:
    CREATE_TENANT_ISOLATION_MODE_OPTIONS.find(
      (option) => option.value !== DEFAULT_CREATE_TENANT_ISOLATION_MODE,
    )?.value ?? DEFAULT_CREATE_TENANT_ISOLATION_MODE,
  planTier:
    CREATE_TENANT_PLAN_TIER_OPTIONS[CREATE_TENANT_PLAN_TIER_OPTIONS.length - 1]?.value ??
    DEFAULT_CREATE_TENANT_PLAN_TIER,
} as const;

export function makeUserSearchResponse(
  items: AdminUser[] = [],
  overrides: Partial<UserListResponse> = {},
): UserListResponse {
  return {
    items,
    page: 1,
    perPage: 10,
    totalItems: items.length,
    totalPages: items.length > 0 ? 1 : 0,
    ...overrides,
  };
}

export function mockListUsersSearchResults(
  items: AdminUser[] = [],
  overrides: Partial<UserListResponse> = {},
) {
  mockListUsers.mockResolvedValue(makeUserSearchResponse(items, overrides));
}

/**
 * TODO: Document getCreateTenantDialogControls.
 */
export function getCreateTenantDialogControls() {
  return {
    nameInput: screen.getByLabelText("Tenant Name", { selector: "#tenant-create-name" }),
    slugInput: screen.getByLabelText("Slug", { selector: "#tenant-create-slug" }),
    ownerUserIdInput: screen.getByRole("combobox", {
      name: "Owner User ID",
    }),
    isolationModeSelect: screen.getByLabelText("Isolation Mode", {
      selector: "#tenant-create-isolation-mode",
    }),
    planTierSelect: screen.getByLabelText("Plan Tier", {
      selector: "#tenant-create-plan-tier",
    }),
    regionInput: screen.getByLabelText("Region", { selector: "#tenant-create-region" }),
    createButton: screen.getByRole("button", { name: "Create" }),
  };
}

export async function openCreateTenantDialog(user: UserEvent) {
  await user.click(screen.getByRole("button", { name: "Create Tenant" }));
  return getCreateTenantDialogControls();
}

export function makeExpectedCreateTenantPayload(
  overrides: Partial<CreateTenantRequest> & Pick<CreateTenantRequest, "name" | "slug">,
): CreateTenantRequest {
  return {
    ...CREATE_TENANT_DIALOG_DEFAULTS,
    ...overrides,
  };
}

export function makeApiError(status: number, message: string): Error & { status: number } {
  return Object.assign(new Error(message), { status });
}

/**
 * TODO: Document makeTenant.
 */
export function makeTenant(overrides: Partial<Tenant> = {}): Tenant {
  return {
    id: "t-1",
    name: "Acme",
    slug: "acme",
    isolationMode: CREATE_TENANT_DIALOG_DEFAULTS.isolationMode,
    planTier: CREATE_TENANT_DIALOG_DEFAULTS.planTier,
    region: CREATE_TENANT_DIALOG_DEFAULTS.region,
    orgId: null,
    orgMetadata: null,
    state: "active",
    idempotencyKey: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
    ...overrides,
  };
}

export function makeListResponse(overrides: Partial<TenantListResponse> = {}): TenantListResponse {
  return {
    items: [
      makeTenant(),
      makeTenant({ id: "t-2", name: "Beta Corp", slug: "beta-corp", state: "suspended" }),
    ],
    page: 1,
    perPage: 20,
    totalItems: 2,
    totalPages: 1,
    ...overrides,
  };
}

export function makeMemberList(overrides: Partial<TenantMemberListResponse> = {}): TenantMemberListResponse {
  return {
    items: [
      { id: "m-1", tenantId: "t-1", userId: "u-1", role: "owner", createdAt: "2026-01-01T00:00:00Z" },
      { id: "m-2", tenantId: "t-1", userId: "u-2", role: "admin", createdAt: "2026-01-02T00:00:00Z" },
    ],
    ...overrides,
  };
}

export function makeMaintenanceState(overrides: Partial<TenantMaintenanceState> = {}): TenantMaintenanceState {
  return {
    id: "ms-1",
    tenantId: "t-1",
    enabled: false,
    reason: null,
    enabledAt: null,
    enabledBy: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export function makeBreakerState(overrides: Partial<BreakerStateResponse> = {}): BreakerStateResponse {
  return {
    state: "closed",
    consecutiveFailures: 0,
    halfOpenProbes: 0,
    ...overrides,
  };
}

/**
 * TODO: Document makeAuditResponse.
 */
export function makeAuditResponse(overrides: Partial<TenantAuditResponse> = {}): TenantAuditResponse {
  return {
    items: [
      {
        id: "a-1",
        tenantId: "t-1",
        actorId: "u-1",
        action: "tenant.created",
        result: "success",
        metadata: null,
        ipAddress: "10.0.0.1",
        createdAt: "2026-01-01T00:00:00Z",
      },
    ],
    count: 1,
    limit: 50,
    offset: 0,
    ...overrides,
  };
}

/**
 * TODO: Document setupTenantTestMocks.
 */
export function setupTenantTestMocks() {
  mockListUsersSearchResults();
  mockFetchTenantList.mockResolvedValue(makeListResponse());
  mockAddTenantMember.mockResolvedValue({
    id: "m-3",
    tenantId: "t-1",
    userId: "u-3",
    role: "viewer",
    createdAt: "2026-01-03T00:00:00Z",
  });
  mockCreateTenant.mockResolvedValue(makeTenant({ id: "t-3", name: "Gamma", slug: "gamma" }));
  mockGetTenant.mockResolvedValue(makeTenant());
  mockRemoveTenantMember.mockResolvedValue();
  mockFetchTenantMembers.mockResolvedValue(makeMemberList());
  mockFetchMaintenanceState.mockResolvedValue(makeMaintenanceState());
  mockFetchBreakerState.mockResolvedValue(makeBreakerState());
  mockFetchTenantAudit.mockResolvedValue(makeAuditResponse());
  mockSuspendTenant.mockResolvedValue(makeTenant({ state: "suspended" }));
  mockResumeTenant.mockResolvedValue(makeTenant({ state: "active" }));
  mockDeleteTenant.mockResolvedValue(makeTenant({ state: "deleting" }));
  mockEnableMaintenance.mockResolvedValue(makeMaintenanceState({ enabled: true, reason: "deploy" }));
  mockDisableMaintenance.mockResolvedValue(makeMaintenanceState({ enabled: false }));
  mockUpdateTenant.mockResolvedValue(makeTenant({ name: "Acme Updated", orgMetadata: { tier: "gold" } }));
  mockUpdateTenantMemberRole.mockResolvedValue({
    id: "m-2",
    tenantId: "t-1",
    userId: "u-2",
    role: "viewer",
    createdAt: "2026-01-02T00:00:00Z",
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  setupTenantTestMocks();
});

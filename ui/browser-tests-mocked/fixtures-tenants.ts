/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures-tenants.ts.
 */
import type { Page, Route } from "@playwright/test";

function json(route: Route, status: number, body: unknown): Promise<void> {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

interface MockTenant {
  id: string;
  name: string;
  slug: string;
  isolationMode: string;
  planTier: string;
  region: string;
  orgId: string | null;
  orgMetadata: Record<string, unknown> | null;
  state: "provisioning" | "active" | "suspended" | "deleting" | "deleted";
  idempotencyKey: string | null;
  createdAt: string;
  updatedAt: string;
}

interface MockTenantAuditEvent {
  id: string;
  tenantId: string;
  actorId: string | null;
  action: string;
  result: string;
  metadata: Record<string, unknown> | null;
  ipAddress: string | null;
  createdAt: string;
}

export interface TenantAdminMockState {
  lifecycleCalls: {
    suspend: number;
    resume: number;
  };
  maintenanceCalls: {
    enable: number;
    disable: number;
  };
}

function nowIso(): string {
  return "2026-03-15T23:00:00Z";
}

function cloneTenant(tenant: MockTenant): MockTenant {
  return { ...tenant, orgMetadata: tenant.orgMetadata ? { ...tenant.orgMetadata } : null };
}

function buildAuditResponse(items: MockTenantAuditEvent[], limit: number, offset: number) {
  const sliced = items.slice(offset, offset + limit);
  return {
    items: sliced,
    count: sliced.length,
    limit,
    offset,
  };
}

/**
 * TODO: Document mockTenantAdminApis.
 */
export async function mockTenantAdminApis(page: Page): Promise<TenantAdminMockState> {
  const tenantOne: MockTenant = {
    id: "tenant-1",
    name: "Acme Labs",
    slug: "acme-labs",
    isolationMode: "shared",
    planTier: "pro",
    region: "us-east-1",
    orgId: null,
    orgMetadata: { industry: "saas" },
    state: "active",
    idempotencyKey: null,
    createdAt: "2026-03-01T00:00:00Z",
    updatedAt: "2026-03-01T00:00:00Z",
  };
  const tenantTwo: MockTenant = {
    id: "tenant-2",
    name: "Beta Ops",
    slug: "beta-ops",
    isolationMode: "schema",
    planTier: "enterprise",
    region: "us-west-2",
    orgId: null,
    orgMetadata: { industry: "ops" },
    state: "suspended",
    idempotencyKey: null,
    createdAt: "2026-03-02T00:00:00Z",
    updatedAt: "2026-03-02T00:00:00Z",
  };
  const tenantThree: MockTenant = {
    id: "tenant-3",
    name: "Gamma Provisioning",
    slug: "gamma-provisioning",
    isolationMode: "shared",
    planTier: "starter",
    region: "us-central-1",
    orgId: null,
    orgMetadata: { industry: "iot" },
    state: "provisioning",
    idempotencyKey: null,
    createdAt: "2026-03-03T00:00:00Z",
    updatedAt: "2026-03-03T00:00:00Z",
  };
  const tenantFour: MockTenant = {
    id: "tenant-4",
    name: "Delta Deleting",
    slug: "delta-deleting",
    isolationMode: "schema",
    planTier: "pro",
    region: "us-east-2",
    orgId: null,
    orgMetadata: { industry: "manufacturing" },
    state: "deleting",
    idempotencyKey: null,
    createdAt: "2026-03-04T00:00:00Z",
    updatedAt: "2026-03-04T00:00:00Z",
  };
  const tenantFive: MockTenant = {
    id: "tenant-5",
    name: "Echo Deleted",
    slug: "echo-deleted",
    isolationMode: "shared",
    planTier: "free",
    region: "us-west-1",
    orgId: null,
    orgMetadata: { industry: "retail" },
    state: "deleted",
    idempotencyKey: null,
    createdAt: "2026-03-05T00:00:00Z",
    updatedAt: "2026-03-05T00:00:00Z",
  };

  const tenantsById = new Map<string, MockTenant>([
    [tenantOne.id, tenantOne],
    [tenantTwo.id, tenantTwo],
    [tenantThree.id, tenantThree],
    [tenantFour.id, tenantFour],
    [tenantFive.id, tenantFive],
  ]);
  const membersByTenant = new Map<string, Array<Record<string, string>>>([
    [
      "tenant-1",
      [
        { id: "m-1", tenantId: "tenant-1", userId: "user-1", role: "owner", createdAt: nowIso() },
        { id: "m-2", tenantId: "tenant-1", userId: "user-2", role: "admin", createdAt: nowIso() },
      ],
    ],
    ["tenant-2", [{ id: "m-3", tenantId: "tenant-2", userId: "user-3", role: "owner", createdAt: nowIso() }]],
    ["tenant-3", [{ id: "m-4", tenantId: "tenant-3", userId: "user-4", role: "owner", createdAt: nowIso() }]],
    ["tenant-4", [{ id: "m-5", tenantId: "tenant-4", userId: "user-5", role: "admin", createdAt: nowIso() }]],
    ["tenant-5", [{ id: "m-6", tenantId: "tenant-5", userId: "user-6", role: "viewer", createdAt: nowIso() }]],
  ]);
  const maintenanceByTenant = new Map<string, Record<string, unknown>>([
    [
      "tenant-1",
      {
        id: "maint-1",
        tenantId: "tenant-1",
        enabled: false,
        reason: null,
        enabledAt: null,
        enabledBy: null,
        createdAt: nowIso(),
        updatedAt: nowIso(),
      },
    ],
    [
      "tenant-2",
      {
        id: "maint-2",
        tenantId: "tenant-2",
        enabled: true,
        reason: "planned upgrade",
        enabledAt: nowIso(),
        enabledBy: "user-3",
        createdAt: nowIso(),
        updatedAt: nowIso(),
      },
    ],
    [
      "tenant-3",
      {
        id: "maint-3",
        tenantId: "tenant-3",
        enabled: false,
        reason: null,
        enabledAt: null,
        enabledBy: null,
        createdAt: nowIso(),
        updatedAt: nowIso(),
      },
    ],
    [
      "tenant-4",
      {
        id: "maint-4",
        tenantId: "tenant-4",
        enabled: false,
        reason: null,
        enabledAt: null,
        enabledBy: null,
        createdAt: nowIso(),
        updatedAt: nowIso(),
      },
    ],
    [
      "tenant-5",
      {
        id: "maint-5",
        tenantId: "tenant-5",
        enabled: false,
        reason: null,
        enabledAt: null,
        enabledBy: null,
        createdAt: nowIso(),
        updatedAt: nowIso(),
      },
    ],
  ]);
  const breakerByTenant = new Map<string, Record<string, unknown>>([
    ["tenant-1", { state: "closed", consecutiveFailures: 0, halfOpenProbes: 0 }],
    ["tenant-2", { state: "open", consecutiveFailures: 5, halfOpenProbes: 1 }],
    ["tenant-3", { state: "closed", consecutiveFailures: 0, halfOpenProbes: 0 }],
    ["tenant-4", { state: "half_open", consecutiveFailures: 2, halfOpenProbes: 1 }],
    ["tenant-5", { state: "closed", consecutiveFailures: 0, halfOpenProbes: 0 }],
  ]);
  const auditByTenant = new Map<string, MockTenantAuditEvent[]>([
    [
      "tenant-1",
      [
        {
          id: "audit-1",
          tenantId: "tenant-1",
          actorId: "admin-user",
          action: "tenant.created",
          result: "success",
          metadata: { name: "Acme Labs" },
          ipAddress: "127.0.0.1",
          createdAt: nowIso(),
        },
      ],
    ],
    ["tenant-2", []],
    ["tenant-3", []],
    ["tenant-4", []],
    ["tenant-5", []],
  ]);

  const state: TenantAdminMockState = {
    lifecycleCalls: { suspend: 0, resume: 0 },
    maintenanceCalls: { enable: 0, disable: 0 },
  };

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/status") {
      return json(route, 200, { auth: true });
    }

    if (method === "GET" && path === "/api/schema") {
      return json(route, 200, { tables: {}, schemas: ["public"], builtAt: nowIso() });
    }

    if (method === "GET" && path === "/api/admin/tenants") {
      const pageValue = Number(url.searchParams.get("page") || "1");
      const perPageValue = Number(url.searchParams.get("perPage") || "20");
      const items = Array.from(tenantsById.values()).map(cloneTenant);
      return json(route, 200, {
        items,
        page: pageValue,
        perPage: perPageValue,
        totalItems: items.length,
        totalPages: 1,
      });
    }

    const tenantPathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)$/);
    if (method === "GET" && tenantPathMatch) {
      const tenantId = decodeURIComponent(tenantPathMatch[1]);
      const tenant = tenantsById.get(tenantId);
      if (!tenant) return json(route, 404, { code: 404, message: "tenant not found" });
      return json(route, 200, cloneTenant(tenant));
    }

    const membersPathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)\/members$/);
    if (method === "GET" && membersPathMatch) {
      const tenantId = decodeURIComponent(membersPathMatch[1]);
      return json(route, 200, { items: membersByTenant.get(tenantId) ?? [] });
    }

    const maintenancePathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)\/maintenance$/);
    if (method === "GET" && maintenancePathMatch) {
      const tenantId = decodeURIComponent(maintenancePathMatch[1]);
      return json(route, 200, maintenanceByTenant.get(tenantId) ?? { tenantId, enabled: false });
    }

    const enableMaintenancePathMatch = path.match(
      /^\/api\/admin\/tenants\/([^/]+)\/maintenance\/enable$/,
    );
    if (method === "POST" && enableMaintenancePathMatch) {
      const tenantId = decodeURIComponent(enableMaintenancePathMatch[1]);
      const payload = request.postDataJSON() as { reason?: string } | null;
      const existingState = maintenanceByTenant.get(tenantId);
      const updatedState = {
        id: String(existingState?.id ?? `maint-${tenantId}`),
        tenantId,
        enabled: true,
        reason: payload?.reason ?? "mock-enabled",
        enabledAt: nowIso(),
        enabledBy: "admin-user",
        createdAt: String(existingState?.createdAt ?? nowIso()),
        updatedAt: nowIso(),
      };
      maintenanceByTenant.set(tenantId, updatedState);
      state.maintenanceCalls.enable += 1;
      return json(route, 200, updatedState);
    }

    const disableMaintenancePathMatch = path.match(
      /^\/api\/admin\/tenants\/([^/]+)\/maintenance\/disable$/,
    );
    if (method === "POST" && disableMaintenancePathMatch) {
      const tenantId = decodeURIComponent(disableMaintenancePathMatch[1]);
      const existingState = maintenanceByTenant.get(tenantId);
      const updatedState = {
        id: String(existingState?.id ?? `maint-${tenantId}`),
        tenantId,
        enabled: false,
        reason: null,
        enabledAt: null,
        enabledBy: null,
        createdAt: String(existingState?.createdAt ?? nowIso()),
        updatedAt: nowIso(),
      };
      maintenanceByTenant.set(tenantId, updatedState);
      state.maintenanceCalls.disable += 1;
      return json(route, 200, updatedState);
    }

    const breakerPathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)\/breaker$/);
    if (method === "GET" && breakerPathMatch) {
      const tenantId = decodeURIComponent(breakerPathMatch[1]);
      return json(route, 200, breakerByTenant.get(tenantId) ?? { state: "closed", consecutiveFailures: 0, halfOpenProbes: 0 });
    }

    const resetBreakerPathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)\/breaker\/reset$/);
    if (method === "POST" && resetBreakerPathMatch) {
      const tenantId = decodeURIComponent(resetBreakerPathMatch[1]);
      const updatedBreaker = { state: "closed", consecutiveFailures: 0, halfOpenProbes: 0 };
      breakerByTenant.set(tenantId, updatedBreaker);
      return json(route, 200, updatedBreaker);
    }

    const auditPathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)\/audit$/);
    if (method === "GET" && auditPathMatch) {
      const tenantId = decodeURIComponent(auditPathMatch[1]);
      const limit = Number(url.searchParams.get("limit") || "50");
      const offset = Number(url.searchParams.get("offset") || "0");
      const events = auditByTenant.get(tenantId) ?? [];
      return json(route, 200, buildAuditResponse(events, limit, offset));
    }

    const suspendPathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)\/suspend$/);
    if (method === "POST" && suspendPathMatch) {
      const tenantId = decodeURIComponent(suspendPathMatch[1]);
      const tenant = tenantsById.get(tenantId);
      if (!tenant) return json(route, 404, { code: 404, message: "tenant not found" });
      tenant.state = "suspended";
      tenant.updatedAt = nowIso();
      state.lifecycleCalls.suspend += 1;
      const tenantEvents = auditByTenant.get(tenantId) ?? [];
      tenantEvents.unshift({
        id: `audit-suspend-${state.lifecycleCalls.suspend}`,
        tenantId,
        actorId: "admin-user",
        action: "tenant.suspended",
        result: "success",
        metadata: { previousState: "active" },
        ipAddress: "127.0.0.1",
        createdAt: nowIso(),
      });
      auditByTenant.set(tenantId, tenantEvents);
      return json(route, 200, cloneTenant(tenant));
    }

    const resumePathMatch = path.match(/^\/api\/admin\/tenants\/([^/]+)\/resume$/);
    if (method === "POST" && resumePathMatch) {
      const tenantId = decodeURIComponent(resumePathMatch[1]);
      const tenant = tenantsById.get(tenantId);
      if (!tenant) return json(route, 404, { code: 404, message: "tenant not found" });
      tenant.state = "active";
      tenant.updatedAt = nowIso();
      state.lifecycleCalls.resume += 1;
      return json(route, 200, cloneTenant(tenant));
    }

    return json(route, 500, {
      message: `Unhandled mocked API route: ${method} ${path}`,
    });
  });

  return state;
}

import { beforeEach, describe, expect, it, vi } from "vitest";
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
  normalizeTenantAuditPayload,
  normalizeTenantListPayload,
  removeTenantMember,
  resetBreaker,
  resumeTenant,
  suspendTenant,
  updateTenant,
  updateTenantMemberRole,
} from "../api_tenants";

function okJson(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function errJson(status: number, message: string): Response {
  return new Response(JSON.stringify({ code: status, message }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const SAMPLE_TENANT = {
  id: "t-1",
  name: "Acme",
  slug: "acme",
  isolationMode: "shared",
  planTier: "pro",
  region: "us-east-1",
  orgId: null,
  orgMetadata: null,
  state: "active",
  idempotencyKey: null,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-02T00:00:00Z",
};

describe("tenant API request paths", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  // -----------------------------------------------------------------------
  // CRUD / Lifecycle
  // -----------------------------------------------------------------------

  it("calls GET /api/admin/tenants with page/perPage and admin auth header", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        items: [SAMPLE_TENANT],
        page: 2,
        perPage: 10,
        totalItems: 25,
        totalPages: 3,
      }),
    );

    const result = await fetchTenantList({ page: 2, perPage: 10 });

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/tenants?page=2&perPage=10", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result.items).toHaveLength(1);
    expect(result.items[0].slug).toBe("acme");
    expect(result.page).toBe(2);
    expect(result.totalPages).toBe(3);
  });

  it("calls POST /api/admin/tenants with create body", async () => {
    fetchMock.mockResolvedValueOnce(okJson(SAMPLE_TENANT, 201));

    const result = await createTenant({
      name: "Acme",
      slug: "acme",
      ownerUserId: "u-1",
      isolationMode: "shared",
      planTier: "pro",
      region: "us-east-1",
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(init?.body as string)).toEqual({
      name: "Acme",
      slug: "acme",
      ownerUserId: "u-1",
      isolationMode: "shared",
      planTier: "pro",
      region: "us-east-1",
    });
    expect(result.id).toBe("t-1");
  });

  it("calls GET /api/admin/tenants/{tenantId} for get", async () => {
    fetchMock.mockResolvedValueOnce(okJson(SAMPLE_TENANT));

    const result = await getTenant("t-1");

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/tenants/t-1", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result.name).toBe("Acme");
  });

  it("calls PUT /api/admin/tenants/{tenantId} with update body", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ ...SAMPLE_TENANT, name: "Acme Corp" }));

    const result = await updateTenant("t-1", { name: "Acme Corp" });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(init?.body as string)).toEqual({ name: "Acme Corp" });
    expect(result.name).toBe("Acme Corp");
  });

  it("calls DELETE /api/admin/tenants/{tenantId} and returns transitioning tenant", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ ...SAMPLE_TENANT, state: "deleting" }));

    const result = await deleteTenant("t-1");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1");
    expect(init?.method).toBe("DELETE");
    expect(result.state).toBe("deleting");
  });

  it("calls POST /api/admin/tenants/{tenantId}/suspend", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ ...SAMPLE_TENANT, state: "suspended" }));

    const result = await suspendTenant("t-1");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/suspend");
    expect(init?.method).toBe("POST");
    expect(result.state).toBe("suspended");
  });

  it("calls POST /api/admin/tenants/{tenantId}/resume", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ ...SAMPLE_TENANT, state: "active" }));

    const result = await resumeTenant("t-1");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/resume");
    expect(init?.method).toBe("POST");
    expect(result.state).toBe("active");
  });

  // -----------------------------------------------------------------------
  // Membership
  // -----------------------------------------------------------------------

  it("calls GET /api/admin/tenants/{tenantId}/members", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        items: [{ id: "m-1", tenantId: "t-1", userId: "u-1", role: "owner", createdAt: "2026-01-01T00:00:00Z" }],
      }),
    );

    const result = await fetchTenantMembers("t-1");

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/tenants/t-1/members", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result.items).toHaveLength(1);
    expect(result.items[0].role).toBe("owner");
  });

  it("calls POST /api/admin/tenants/{tenantId}/members with userId and role", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson(
        { id: "m-2", tenantId: "t-1", userId: "u-2", role: "admin", createdAt: "2026-01-01T00:00:00Z" },
        201,
      ),
    );

    const result = await addTenantMember("t-1", "u-2", "admin");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/members");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(init?.body as string)).toEqual({ userId: "u-2", role: "admin" });
    expect(result.role).toBe("admin");
  });

  it("calls PUT /api/admin/tenants/{tenantId}/members/{userId} with role", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({ id: "m-2", tenantId: "t-1", userId: "u-2", role: "viewer", createdAt: "2026-01-01T00:00:00Z" }),
    );

    const result = await updateTenantMemberRole("t-1", "u-2", "viewer");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/members/u-2");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(init?.body as string)).toEqual({ role: "viewer" });
    expect(result.role).toBe("viewer");
  });

  it("calls DELETE /api/admin/tenants/{tenantId}/members/{userId} with no response body", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await removeTenantMember("t-1", "u-2");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/members/u-2");
    expect(init?.method).toBe("DELETE");
  });

  // -----------------------------------------------------------------------
  // Maintenance / Breaker
  // -----------------------------------------------------------------------

  it("calls GET /api/admin/tenants/{tenantId}/maintenance", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        id: "ms-1",
        tenantId: "t-1",
        enabled: true,
        reason: "deploy",
        enabledAt: "2026-01-01T00:00:00Z",
        enabledBy: "u-1",
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      }),
    );

    const result = await fetchMaintenanceState("t-1");

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/tenants/t-1/maintenance", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result.enabled).toBe(true);
    expect(result.reason).toBe("deploy");
  });

  it("calls POST /api/admin/tenants/{tenantId}/maintenance/enable with reason", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        id: "ms-1",
        tenantId: "t-1",
        enabled: true,
        reason: "scheduled upgrade",
        enabledAt: "2026-01-01T00:00:00Z",
        enabledBy: "u-1",
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      }),
    );

    await enableMaintenance("t-1", "scheduled upgrade");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/maintenance/enable");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(init?.body as string)).toEqual({ reason: "scheduled upgrade" });
  });

  it("calls POST /api/admin/tenants/{tenantId}/maintenance/disable", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({ tenantId: "t-1", enabled: false, id: "", createdAt: "", updatedAt: "" }),
    );

    const result = await disableMaintenance("t-1");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/maintenance/disable");
    expect(init?.method).toBe("POST");
    expect(result.enabled).toBe(false);
  });

  it("calls GET /api/admin/tenants/{tenantId}/breaker", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({ state: "open", consecutiveFailures: 5, halfOpenProbes: 2 }),
    );

    const result = await fetchBreakerState("t-1");

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/tenants/t-1/breaker", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result.state).toBe("open");
    expect(result.consecutiveFailures).toBe(5);
  });

  it("calls POST /api/admin/tenants/{tenantId}/breaker/reset", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({ state: "closed", consecutiveFailures: 0, halfOpenProbes: 0 }),
    );

    const result = await resetBreaker("t-1");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/tenants/t-1/breaker/reset");
    expect(init?.method).toBe("POST");
    expect(result.state).toBe("closed");
  });

  // -----------------------------------------------------------------------
  // Audit
  // -----------------------------------------------------------------------

  it("calls GET /api/admin/tenants/{tenantId}/audit with filter params", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        items: [
          {
            id: "a-1",
            tenantId: "t-1",
            actorId: "u-1",
            action: "tenant.created",
            result: "success",
            metadata: { name: "Acme" },
            ipAddress: "10.0.0.1",
            createdAt: "2026-01-01T00:00:00Z",
          },
        ],
        count: 1,
        limit: 50,
        offset: 0,
      }),
    );

    const result = await fetchTenantAudit("t-1", {
      limit: 50,
      offset: 0,
      from: "2026-01-01T00:00:00Z",
      to: "2026-02-01T00:00:00Z",
      action: "tenant.created",
      result: "success",
      actorId: "u-1",
    });

    const calledUrl = fetchMock.mock.calls[0][0] as string;
    expect(calledUrl).toContain("/api/admin/tenants/t-1/audit?");
    const params = new URL(calledUrl, "http://localhost").searchParams;
    expect(params.get("limit")).toBe("50");
    expect(params.get("offset")).toBe("0");
    expect(params.get("from")).toBe("2026-01-01T00:00:00Z");
    expect(params.get("to")).toBe("2026-02-01T00:00:00Z");
    expect(params.get("action")).toBe("tenant.created");
    expect(params.get("result")).toBe("success");
    expect(params.get("actor_id")).toBe("u-1");
    expect(result.items).toHaveLength(1);
    expect(result.items[0].action).toBe("tenant.created");
    expect(result.count).toBe(1);
  });

  it("omits optional audit filter params when not set", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ items: [], count: 0, limit: 50, offset: 0 }));

    await fetchTenantAudit("t-1", { limit: 50, offset: 0 });

    const calledUrl = fetchMock.mock.calls[0][0] as string;
    const params = new URL(calledUrl, "http://localhost").searchParams;
    expect(params.get("limit")).toBe("50");
    expect(params.get("offset")).toBe("0");
    expect(params.has("from")).toBe(false);
    expect(params.has("to")).toBe(false);
    expect(params.has("action")).toBe(false);
    expect(params.has("result")).toBe(false);
    expect(params.has("actor_id")).toBe(false);
  });

  // -----------------------------------------------------------------------
  // Error propagation
  // -----------------------------------------------------------------------

  it("propagates 400 validation errors", async () => {
    fetchMock.mockResolvedValueOnce(errJson(400, "name is required"));
    await expect(createTenant({ name: "", slug: "s", ownerUserId: "u", isolationMode: "", planTier: "", region: "" }))
      .rejects.toThrow("name is required");
  });

  it("propagates 404 not-found errors", async () => {
    fetchMock.mockResolvedValueOnce(errJson(404, "tenant not found"));
    await expect(getTenant("no-such")).rejects.toThrow("tenant not found");
  });

  it("propagates 409 conflict errors", async () => {
    fetchMock.mockResolvedValueOnce(errJson(409, "tenant slug is already taken"));
    await expect(
      createTenant({ name: "X", slug: "taken", ownerUserId: "u", isolationMode: "", planTier: "", region: "" }),
    ).rejects.toThrow("tenant slug is already taken");
  });

  // -----------------------------------------------------------------------
  // Malformed payload normalization
  // -----------------------------------------------------------------------

  it("normalizes malformed tenant list payloads into safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(okJson(null));
    const result = await fetchTenantList({ page: 1, perPage: 20 });
    expect(result).toEqual({ items: [], page: 0, perPage: 0, totalItems: 0, totalPages: 0 });
  });

  it("normalizes malformed audit payloads into safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ items: [null], count: "bad" }));
    const result = await fetchTenantAudit("t-1", { limit: 50, offset: 0 });
    expect(result.items).toHaveLength(1);
    expect(result.items[0]).toEqual({
      id: "",
      tenantId: "",
      actorId: null,
      action: "",
      result: "",
      metadata: null,
      ipAddress: null,
      createdAt: "",
    });
    expect(result.count).toBe(0);
  });

  it("normalizes malformed tenant list payload via exported helper", () => {
    expect(normalizeTenantListPayload("not an object")).toEqual({
      items: [],
      page: 0,
      perPage: 0,
      totalItems: 0,
      totalPages: 0,
    });
  });

  it("normalizes malformed audit payload via exported helper", () => {
    expect(normalizeTenantAuditPayload(undefined)).toEqual({
      items: [],
      count: 0,
      limit: 0,
      offset: 0,
    });
  });

  // -----------------------------------------------------------------------
  // URL encoding
  // -----------------------------------------------------------------------

  it("encodes tenant ID and user ID path segments", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await removeTenantMember("tenant/special", "user with spaces");

    const calledUrl = fetchMock.mock.calls[0][0] as string;
    expect(calledUrl).toBe("/api/admin/tenants/tenant%2Fspecial/members/user%20with%20spaces");
  });
});

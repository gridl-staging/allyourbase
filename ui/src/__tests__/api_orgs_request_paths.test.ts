import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  addOrgMember,
  addTeamMember,
  assignTenantToOrg,
  createOrg,
  createTeam,
  deleteOrg,
  deleteTeam,
  fetchOrgAudit,
  fetchOrgList,
  fetchOrgMembers,
  fetchOrgTenants,
  fetchOrgUsage,
  fetchTeamMembers,
  fetchTeams,
  getOrg,
  getTeam,
  removeOrgMember,
  removeTeamMember,
  unassignTenantFromOrg,
  updateOrg,
  updateOrgMemberRole,
  updateTeam,
  updateTeamMemberRole,
} from "../api_orgs";

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

const SAMPLE_ORG = {
  id: "550e8400-e29b-41d4-a716-446655440001",
  name: "Acme Org",
  slug: "acme-org",
  parentOrgId: null,
  planTier: "free",
  createdAt: "2026-03-01T00:00:00Z",
  updatedAt: "2026-03-02T00:00:00Z",
};

describe("org API request paths", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("calls GET /api/admin/orgs without pagination query params", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ items: [SAMPLE_ORG] }));

    const result = await fetchOrgList();

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/orgs", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result.items).toHaveLength(1);
    expect(result.items[0].slug).toBe("acme-org");
  });

  it("normalizes malformed org list payloads to safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response("null", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(fetchOrgList()).resolves.toEqual({ items: [] });
  });

  it("normalizes invalid org/team member roles and tenant state values to safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        items: [
          {
            id: "m-1",
            orgId: SAMPLE_ORG.id,
            userId: "u-1",
            role: "super-admin",
            createdAt: "2026-03-01T00:00:00Z",
          },
        ],
      }),
    );
    const members = await fetchOrgMembers(SAMPLE_ORG.id);
    expect(members.items[0].role).toBe("member");

    fetchMock.mockResolvedValueOnce(
      okJson({
        items: [
          {
            id: "tenant-1",
            name: "Tenant One",
            slug: "tenant-one",
            isolationMode: "shared",
            planTier: "pro",
            region: "us-east-1",
            orgId: SAMPLE_ORG.id,
            orgMetadata: null,
            state: "mystery-state",
            idempotencyKey: null,
            createdAt: "2026-03-01T00:00:00Z",
            updatedAt: "2026-03-01T00:00:00Z",
          },
        ],
      }),
    );
    const tenants = await fetchOrgTenants(SAMPLE_ORG.id);
    expect(tenants.items[0].state).toBe("provisioning");

    fetchMock.mockResolvedValueOnce(
      okJson({
        items: [
          {
            id: "tm-1",
            teamId: "team-1",
            userId: "u-2",
            role: "captain",
            createdAt: "2026-03-01T00:00:00Z",
          },
        ],
      }),
    );
    const teamMembers = await fetchTeamMembers(SAMPLE_ORG.id, "team-1");
    expect(teamMembers.items[0].role).toBe("member");
  });

  it("calls POST /api/admin/orgs with slug, planTier, and optional parentOrgId", async () => {
    fetchMock.mockResolvedValueOnce(okJson(SAMPLE_ORG, 201));

    const result = await createOrg({
      name: "Acme Org",
      slug: "acme-org",
      planTier: "free",
      parentOrgId: "550e8400-e29b-41d4-a716-446655440010",
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/orgs");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(init?.body as string)).toEqual({
      name: "Acme Org",
      slug: "acme-org",
      planTier: "free",
      parentOrgId: "550e8400-e29b-41d4-a716-446655440010",
    });
    expect(result.id).toBe(SAMPLE_ORG.id);
  });

  it("calls GET /api/admin/orgs/{orgId} and reads enriched detail counts", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        ...SAMPLE_ORG,
        childOrgCount: 2,
        teamCount: 3,
        tenantCount: 4,
      }),
    );

    const result = await getOrg("550e8400-e29b-41d4-a716-446655440001");

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/orgs/550e8400-e29b-41d4-a716-446655440001", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result.childOrgCount).toBe(2);
    expect(result.teamCount).toBe(3);
    expect(result.tenantCount).toBe(4);
  });

  it("calls PUT /api/admin/orgs/{orgId} with patch payload", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ ...SAMPLE_ORG, name: "Acme Org Updated" }));

    const result = await updateOrg("550e8400-e29b-41d4-a716-446655440001", {
      name: "Acme Org Updated",
      parentOrgId: "",
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/orgs/550e8400-e29b-41d4-a716-446655440001");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(init?.body as string)).toEqual({ name: "Acme Org Updated", parentOrgId: "" });
    expect(result.name).toBe("Acme Org Updated");
  });

  it("calls DELETE /api/admin/orgs/{orgId}?confirm=true with no body parser", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await deleteOrg("550e8400-e29b-41d4-a716-446655440001");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/admin/orgs/550e8400-e29b-41d4-a716-446655440001?confirm=true");
    expect(init?.method).toBe("DELETE");
  });

  it("serializes org membership CRUD endpoints", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ items: [{ id: "m-1", orgId: SAMPLE_ORG.id, userId: "u-1", role: "owner", createdAt: "2026-03-01T00:00:00Z" }] }));
    const listed = await fetchOrgMembers(SAMPLE_ORG.id);
    expect(fetchMock).toHaveBeenNthCalledWith(1, `/api/admin/orgs/${SAMPLE_ORG.id}/members`, {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(listed.items[0].role).toBe("owner");

    fetchMock.mockResolvedValueOnce(okJson({ id: "m-2", orgId: SAMPLE_ORG.id, userId: "u-2", role: "admin", createdAt: "2026-03-01T00:00:00Z" }, 201));
    await addOrgMember(SAMPLE_ORG.id, "u-2", "admin");
    const [addURL, addInit] = fetchMock.mock.calls[1];
    expect(addURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/members`);
    expect(addInit?.method).toBe("POST");
    expect(JSON.parse(addInit?.body as string)).toEqual({ userId: "u-2", role: "admin" });

    fetchMock.mockResolvedValueOnce(okJson({ id: "m-2", orgId: SAMPLE_ORG.id, userId: "u-2", role: "viewer", createdAt: "2026-03-01T00:00:00Z" }));
    await updateOrgMemberRole(SAMPLE_ORG.id, "u-2", "viewer");
    const [updateURL, updateInit] = fetchMock.mock.calls[2];
    expect(updateURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/members/u-2/role`);
    expect(updateInit?.method).toBe("PUT");
    expect(JSON.parse(updateInit?.body as string)).toEqual({ role: "viewer" });

    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await removeOrgMember(SAMPLE_ORG.id, "u-2");
    const [removeURL, removeInit] = fetchMock.mock.calls[3];
    expect(removeURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/members/u-2`);
    expect(removeInit?.method).toBe("DELETE");
  });

  it("serializes tenant assignment endpoints", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ status: "assigned" }));
    await assignTenantToOrg(SAMPLE_ORG.id, "tenant-1");
    const [assignURL, assignInit] = fetchMock.mock.calls[0];
    expect(assignURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/tenants`);
    expect(assignInit?.method).toBe("POST");
    expect(JSON.parse(assignInit?.body as string)).toEqual({ tenantId: "tenant-1" });

    fetchMock.mockResolvedValueOnce(okJson({ items: [{ id: "tenant-1", name: "Tenant One", slug: "tenant-one" }] }));
    const listed = await fetchOrgTenants(SAMPLE_ORG.id);
    expect(fetchMock).toHaveBeenNthCalledWith(2, `/api/admin/orgs/${SAMPLE_ORG.id}/tenants`, {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(listed.items).toHaveLength(1);

    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await unassignTenantFromOrg(SAMPLE_ORG.id, "tenant-1");
    const [removeURL, removeInit] = fetchMock.mock.calls[2];
    expect(removeURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/tenants/tenant-1`);
    expect(removeInit?.method).toBe("DELETE");
  });

  it("serializes team CRUD endpoints inside org scope", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ id: "team-1", orgId: SAMPLE_ORG.id, name: "Core", slug: "core", createdAt: "2026-03-01T00:00:00Z", updatedAt: "2026-03-01T00:00:00Z" }, 201));
    await createTeam(SAMPLE_ORG.id, { name: "Core", slug: "core" });
    const [createURL, createInit] = fetchMock.mock.calls[0];
    expect(createURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/teams`);
    expect(createInit?.method).toBe("POST");

    fetchMock.mockResolvedValueOnce(okJson({ items: [{ id: "team-1", orgId: SAMPLE_ORG.id, name: "Core", slug: "core", createdAt: "2026-03-01T00:00:00Z", updatedAt: "2026-03-01T00:00:00Z" }] }));
    const list = await fetchTeams(SAMPLE_ORG.id);
    expect(list.items).toHaveLength(1);

    fetchMock.mockResolvedValueOnce(okJson({ id: "team-1", orgId: SAMPLE_ORG.id, name: "Core", slug: "core", createdAt: "2026-03-01T00:00:00Z", updatedAt: "2026-03-01T00:00:00Z" }));
    const team = await getTeam(SAMPLE_ORG.id, "team-1");
    expect(team.id).toBe("team-1");

    fetchMock.mockResolvedValueOnce(okJson({ id: "team-1", orgId: SAMPLE_ORG.id, name: "Core Platform", slug: "core", createdAt: "2026-03-01T00:00:00Z", updatedAt: "2026-03-02T00:00:00Z" }));
    await updateTeam(SAMPLE_ORG.id, "team-1", { name: "Core Platform" });
    const [updateURL, updateInit] = fetchMock.mock.calls[3];
    expect(updateURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/teams/team-1`);
    expect(updateInit?.method).toBe("PUT");

    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await deleteTeam(SAMPLE_ORG.id, "team-1");
    const [deleteURL, deleteInit] = fetchMock.mock.calls[4];
    expect(deleteURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/teams/team-1`);
    expect(deleteInit?.method).toBe("DELETE");
  });

  it("serializes team membership CRUD and prerequisite-conflict propagation", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ items: [{ id: "tm-1", teamId: "team-1", userId: "u-1", role: "lead", createdAt: "2026-03-01T00:00:00Z" }] }));
    const listed = await fetchTeamMembers(SAMPLE_ORG.id, "team-1");
    expect(fetchMock).toHaveBeenNthCalledWith(1, `/api/admin/orgs/${SAMPLE_ORG.id}/teams/team-1/members`, {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(listed.items[0].role).toBe("lead");

    fetchMock.mockResolvedValueOnce(errJson(409, "user must be an org member before joining a team"));
    await expect(addTeamMember(SAMPLE_ORG.id, "team-1", "u-2", "member")).rejects.toThrow(
      "user must be an org member before joining a team",
    );

    fetchMock.mockResolvedValueOnce(okJson({ id: "tm-2", teamId: "team-1", userId: "u-2", role: "lead", createdAt: "2026-03-01T00:00:00Z" }));
    await updateTeamMemberRole(SAMPLE_ORG.id, "team-1", "u-2", "lead");
    const [updateURL, updateInit] = fetchMock.mock.calls[2];
    expect(updateURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/teams/team-1/members/u-2/role`);
    expect(updateInit?.method).toBe("PUT");

    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await removeTeamMember(SAMPLE_ORG.id, "team-1", "u-2");
    const [deleteURL, deleteInit] = fetchMock.mock.calls[3];
    expect(deleteURL).toBe(`/api/admin/orgs/${SAMPLE_ORG.id}/teams/team-1/members/u-2`);
    expect(deleteInit?.method).toBe("DELETE");
  });

  it("serializes org usage query params with default period month", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        orgId: SAMPLE_ORG.id,
        tenantCount: 2,
        period: "month",
        data: [{ date: "2026-03-01", apiRequests: 12, storageBytesUsed: 100, bandwidthBytes: 50, functionInvocations: 4 }],
        totals: { apiRequests: 12, storageBytesUsed: 100, bandwidthBytes: 50, functionInvocations: 4 },
      }),
    );

    const summary = await fetchOrgUsage(SAMPLE_ORG.id, { period: "month", from: null, to: null });

    expect(fetchMock).toHaveBeenCalledWith(`/api/admin/orgs/${SAMPLE_ORG.id}/usage?period=month`, {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(summary.period).toBe("month");
    expect(summary.data).toHaveLength(1);
  });

  it("serializes org usage custom date range query", async () => {
    fetchMock.mockResolvedValueOnce(
      okJson({
        orgId: SAMPLE_ORG.id,
        tenantCount: 1,
        period: "month",
        data: [],
        totals: { apiRequests: 0, storageBytesUsed: 0, bandwidthBytes: 0, functionInvocations: 0 },
      }),
    );

    await fetchOrgUsage(SAMPLE_ORG.id, { period: "month", from: "2026-03-01", to: "2026-03-07" });

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/admin/orgs/${SAMPLE_ORG.id}/usage?from=2026-03-01&to=2026-03-07`,
      {
        headers: { Authorization: "Bearer admin-token" },
      },
    );
  });

  it("forwards partial org usage date filters so the backend can reject incomplete ranges", async () => {
    fetchMock.mockResolvedValueOnce(errJson(400, "both from and to are required when either is provided"));

    await expect(
      fetchOrgUsage(SAMPLE_ORG.id, { period: "month", from: "2026-03-01", to: null }),
    ).rejects.toThrow("both from and to are required when either is provided");

    expect(fetchMock).toHaveBeenCalledWith(`/api/admin/orgs/${SAMPLE_ORG.id}/usage?from=2026-03-01`, {
      headers: { Authorization: "Bearer admin-token" },
    });
  });

  it("forwards to-only usage date filter without from", async () => {
    fetchMock.mockResolvedValueOnce(errJson(400, "both from and to are required when either is provided"));

    await expect(
      fetchOrgUsage(SAMPLE_ORG.id, { period: "month", from: null, to: "2026-03-31" }),
    ).rejects.toThrow("both from and to are required when either is provided");

    expect(fetchMock).toHaveBeenCalledWith(`/api/admin/orgs/${SAMPLE_ORG.id}/usage?to=2026-03-31`, {
      headers: { Authorization: "Bearer admin-token" },
    });
  });

  it("serializes org audit filters and paginated query params", async () => {
    fetchMock.mockResolvedValueOnce(okJson({ items: [], count: 0, limit: 25, offset: 50 }));

    await fetchOrgAudit(SAMPLE_ORG.id, {
      limit: 25,
      offset: 50,
      from: "2026-03-01T00:00:00Z",
      to: "2026-03-10T00:00:00Z",
      action: "org.member.add",
      result: "success",
      actorId: "550e8400-e29b-41d4-a716-446655440099",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/admin/orgs/${SAMPLE_ORG.id}/audit?limit=25&offset=50&from=2026-03-01T00%3A00%3A00Z&to=2026-03-10T00%3A00%3A00Z&action=org.member.add&result=success&actor_id=550e8400-e29b-41d4-a716-446655440099`,
      {
        headers: { Authorization: "Bearer admin-token" },
      },
    );
  });

  it("propagates malformed membership identifier 400 responses from org and team membership endpoints", async () => {
    fetchMock.mockResolvedValueOnce(errJson(400, "invalid org id format"));
    await expect(fetchOrgMembers("not-a-uuid")).rejects.toThrow("invalid org id format");
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/admin/orgs/not-a-uuid/members", {
      headers: { Authorization: "Bearer admin-token" },
    });

    fetchMock.mockResolvedValueOnce(errJson(400, "invalid user id format"));
    await expect(updateOrgMemberRole(SAMPLE_ORG.id, "not-a-uuid", "viewer")).rejects.toThrow(
      "invalid user id format",
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      `/api/admin/orgs/${SAMPLE_ORG.id}/members/not-a-uuid/role`,
      expect.objectContaining({
        method: "PUT",
      }),
    );

    fetchMock.mockResolvedValueOnce(errJson(400, "invalid team id format"));
    await expect(fetchTeamMembers(SAMPLE_ORG.id, "not-a-uuid")).rejects.toThrow("invalid team id format");
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      `/api/admin/orgs/${SAMPLE_ORG.id}/teams/not-a-uuid/members`,
      {
        headers: { Authorization: "Bearer admin-token" },
      },
    );

    fetchMock.mockResolvedValueOnce(errJson(400, "invalid user id format"));
    await expect(removeTeamMember(SAMPLE_ORG.id, "team-1", "not-a-uuid")).rejects.toThrow(
      "invalid user id format",
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      `/api/admin/orgs/${SAMPLE_ORG.id}/teams/team-1/members/not-a-uuid`,
      expect.objectContaining({
        method: "DELETE",
      }),
    );
  });

  it("propagates 400/404/409 backend errors without swallowing messages", async () => {
    fetchMock.mockResolvedValueOnce(errJson(400, "invalid period or date range"));
    await expect(fetchOrgUsage(SAMPLE_ORG.id, { period: "month", from: null, to: null })).rejects.toThrow(
      "invalid period or date range",
    );

    fetchMock.mockResolvedValueOnce(errJson(404, "org not found"));
    await expect(getOrg("550e8400-e29b-41d4-a716-446655440404")).rejects.toThrow("org not found");

    fetchMock.mockResolvedValueOnce(errJson(409, "cannot demote the last owner"));
    await expect(updateOrgMemberRole(SAMPLE_ORG.id, "u-1", "admin")).rejects.toThrow(
      "cannot demote the last owner",
    );
  });
});

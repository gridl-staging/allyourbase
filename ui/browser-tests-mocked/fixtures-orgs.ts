/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures-orgs.ts.
 */
import type { Page, Route } from "@playwright/test";

function json(route: Route, status: number, body: unknown): Promise<void> {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

interface MockOrganization {
  id: string;
  name: string;
  slug: string;
  parentOrgId: string | null;
  planTier: string;
  createdAt: string;
  updatedAt: string;
}

interface MockOrgMembership {
  id: string;
  orgId: string;
  userId: string;
  role: "owner" | "admin" | "member" | "viewer";
  createdAt: string;
}

interface MockTeam {
  id: string;
  orgId: string;
  name: string;
  slug: string;
  createdAt: string;
  updatedAt: string;
}

interface MockTeamMembership {
  id: string;
  teamId: string;
  userId: string;
  role: "lead" | "member";
  createdAt: string;
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
  state: string;
  idempotencyKey: string | null;
  createdAt: string;
  updatedAt: string;
}

interface MockAuditEvent {
  id: string;
  tenantId: string;
  actorId: string | null;
  action: string;
  result: string;
  metadata: Record<string, unknown> | null;
  ipAddress: string | null;
  createdAt: string;
}

/**
 * TODO: Document OrgAdminMockState.
 */
export interface OrgAdminMockState {
  addOrgMemberCalls: number;
  createTeamCalls: number;
  assignTenantCalls: number;
  deleteConfirmTrueCalls: number;
  lastOwnerProtectionHits: number;
  lastUsageQuery: {
    period: string | null;
    from: string | null;
    to: string | null;
  } | null;
  lastAuditQuery: {
    limit: number;
    offset: number;
    from: string | null;
    to: string | null;
    action: string | null;
    result: string | null;
    actorId: string | null;
  } | null;
}

const NOW_ISO = "2026-03-15T23:00:00Z";
const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])$/;

function isValidSlug(value: string): boolean {
  return SLUG_PATTERN.test(value);
}

/**
 * TODO: Document makeTenant.
 */
function makeTenant(tenantId: string, orgId: string): MockTenant {
  return {
    id: tenantId,
    name: `Tenant ${tenantId}`,
    slug: tenantId,
    isolationMode: "shared",
    planTier: "pro",
    region: "us-east-1",
    orgId,
    orgMetadata: null,
    state: "active",
    idempotencyKey: null,
    createdAt: NOW_ISO,
    updatedAt: NOW_ISO,
  };
}

function buildOrgDetail(
  org: MockOrganization,
  orgsById: Map<string, MockOrganization>,
  teamsByOrg: Map<string, MockTeam[]>,
  tenantsByOrg: Map<string, MockTenant[]>,
): Record<string, unknown> {
  const childOrgCount = Array.from(orgsById.values()).filter((current) => current.parentOrgId === org.id)
    .length;
  return {
    ...org,
    childOrgCount,
    teamCount: (teamsByOrg.get(org.id) ?? []).length,
    tenantCount: (tenantsByOrg.get(org.id) ?? []).length,
  };
}

function parsePath(pathname: string, pattern: RegExp): string[] | null {
  const match = pathname.match(pattern);
  if (!match) return null;
  return match.slice(1).map((segment) => decodeURIComponent(segment));
}

/**
 * TODO: Document mockOrgAdminApis.
 */
export async function mockOrgAdminApis(page: Page): Promise<OrgAdminMockState> {
  const orgOne: MockOrganization = {
    id: "org-1",
    name: "Acme Inc",
    slug: "acme-inc",
    parentOrgId: null,
    planTier: "pro",
    createdAt: NOW_ISO,
    updatedAt: NOW_ISO,
  };
  const orgTwo: MockOrganization = {
    id: "org-2",
    name: "Beta Corp",
    slug: "beta-corp",
    parentOrgId: null,
    planTier: "free",
    createdAt: NOW_ISO,
    updatedAt: NOW_ISO,
  };

  const orgsById = new Map<string, MockOrganization>([
    [orgOne.id, orgOne],
    [orgTwo.id, orgTwo],
  ]);

  const orgMembersByOrg = new Map<string, MockOrgMembership[]>([
    [
      "org-1",
      [
        { id: "om-1", orgId: "org-1", userId: "u-1", role: "owner", createdAt: NOW_ISO },
        { id: "om-2", orgId: "org-1", userId: "u-2", role: "admin", createdAt: NOW_ISO },
      ],
    ],
    ["org-2", [{ id: "om-3", orgId: "org-2", userId: "u-3", role: "owner", createdAt: NOW_ISO }]],
  ]);

  const teamsByOrg = new Map<string, MockTeam[]>([
    [
      "org-1",
      [
        {
          id: "team-1",
          orgId: "org-1",
          name: "Engineering",
          slug: "engineering",
          createdAt: NOW_ISO,
          updatedAt: NOW_ISO,
        },
        {
          id: "team-2",
          orgId: "org-1",
          name: "Design",
          slug: "design",
          createdAt: NOW_ISO,
          updatedAt: NOW_ISO,
        },
      ],
    ],
    ["org-2", []],
  ]);

  const teamMembersByTeam = new Map<string, MockTeamMembership[]>([
    [
      "team-1",
      [
        { id: "tm-1", teamId: "team-1", userId: "u-1", role: "lead", createdAt: NOW_ISO },
        { id: "tm-2", teamId: "team-1", userId: "u-2", role: "member", createdAt: NOW_ISO },
      ],
    ],
    ["team-2", [{ id: "tm-3", teamId: "team-2", userId: "u-3", role: "lead", createdAt: NOW_ISO }]],
  ]);

  const tenantsByOrg = new Map<string, MockTenant[]>([
    ["org-1", [makeTenant("t-1", "org-1")]],
    ["org-2", []],
  ]);

  const auditByOrg = new Map<string, MockAuditEvent[]>([
    [
      "org-1",
      [
        {
          id: "oa-1",
          tenantId: "org-1",
          actorId: "u-1",
          action: "org.created",
          result: "success",
          metadata: { source: "mock" },
          ipAddress: "127.0.0.1",
          createdAt: NOW_ISO,
        },
        {
          id: "oa-2",
          tenantId: "org-1",
          actorId: "u-2",
          action: "org.member.add",
          result: "success",
          metadata: { userId: "u-9" },
          ipAddress: "127.0.0.1",
          createdAt: NOW_ISO,
        },
      ],
    ],
    ["org-2", []],
  ]);

  const state: OrgAdminMockState = {
    addOrgMemberCalls: 0,
    createTeamCalls: 0,
    assignTenantCalls: 0,
    deleteConfirmTrueCalls: 0,
    lastOwnerProtectionHits: 0,
    lastUsageQuery: null,
    lastAuditQuery: null,
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
      return json(route, 200, { tables: {}, schemas: ["public"], builtAt: NOW_ISO });
    }

    if (method === "GET" && path === "/api/admin/orgs") {
      return json(route, 200, { items: Array.from(orgsById.values()) });
    }

    if (method === "POST" && path === "/api/admin/orgs") {
      const payload = request.postDataJSON() as {
        name?: string;
        slug?: string;
        planTier?: string;
        parentOrgId?: string;
      };

      const name = payload.name?.trim() ?? "";
      const slug = payload.slug?.trim() ?? "";
      const planTier = payload.planTier?.trim() || "free";
      const parentOrgId = payload.parentOrgId?.trim() || null;

      if (!name || !slug) {
        return json(route, 400, { code: 400, message: "name and slug are required" });
      }
      if (!isValidSlug(slug)) {
        return json(route, 400, { code: 400, message: "invalid slug format" });
      }
      if (Array.from(orgsById.values()).some((org) => org.slug === slug)) {
        return json(route, 409, { code: 409, message: "organization slug already exists" });
      }
      if (parentOrgId === "org-cycle") {
        return json(route, 409, { code: 409, message: "circular parent org hierarchy" });
      }

      const id = `org-${orgsById.size + 1}`;
      const created: MockOrganization = {
        id,
        name,
        slug,
        parentOrgId,
        planTier,
        createdAt: NOW_ISO,
        updatedAt: NOW_ISO,
      };
      orgsById.set(id, created);
      orgMembersByOrg.set(id, []);
      teamsByOrg.set(id, []);
      tenantsByOrg.set(id, []);
      auditByOrg.set(id, []);
      return json(route, 201, created);
    }

    const teamMembersRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/teams\/([^/]+)\/members$/);
    if (teamMembersRoute && method === "GET") {
      const [orgId, teamId] = teamMembersRoute;
      if (!orgsById.has(orgId)) return json(route, 404, { code: 404, message: "org not found" });
      const teams = teamsByOrg.get(orgId) ?? [];
      if (!teams.some((team) => team.id === teamId)) {
        return json(route, 404, { code: 404, message: "team not found" });
      }
      return json(route, 200, { items: teamMembersByTeam.get(teamId) ?? [] });
    }

    if (teamMembersRoute && method === "POST") {
      const [orgId, teamId] = teamMembersRoute;
      const payload = request.postDataJSON() as { userId?: string; role?: "lead" | "member" };
      const userId = payload.userId?.trim() ?? "";
      const role = payload.role === "lead" ? "lead" : "member";
      if (!userId) return json(route, 400, { code: 400, message: "userId is required" });

      const orgMembers = orgMembersByOrg.get(orgId) ?? [];
      if (!orgMembers.some((member) => member.userId === userId)) {
        return json(route, 409, {
          code: 409,
          message: "user must be an org member before joining a team",
        });
      }

      const existing = teamMembersByTeam.get(teamId) ?? [];
      if (existing.some((member) => member.userId === userId)) {
        return json(route, 409, { code: 409, message: "team membership already exists" });
      }

      const created: MockTeamMembership = {
        id: `tm-${existing.length + 10}`,
        teamId,
        userId,
        role,
        createdAt: NOW_ISO,
      };
      teamMembersByTeam.set(teamId, [...existing, created]);
      return json(route, 201, created);
    }

    const teamMemberRoleRoute = parsePath(
      path,
      /^\/api\/admin\/orgs\/([^/]+)\/teams\/([^/]+)\/members\/([^/]+)\/role$/,
    );
    if (teamMemberRoleRoute && method === "PUT") {
      const [, teamId, userId] = teamMemberRoleRoute;
      const payload = request.postDataJSON() as { role?: "lead" | "member" };
      const role = payload.role === "lead" ? "lead" : "member";
      const existing = teamMembersByTeam.get(teamId) ?? [];
      const index = existing.findIndex((member) => member.userId === userId);
      if (index < 0) {
        return json(route, 404, { code: 404, message: "team membership not found" });
      }
      const updated: MockTeamMembership = { ...existing[index], role };
      const next = [...existing];
      next[index] = updated;
      teamMembersByTeam.set(teamId, next);
      return json(route, 200, updated);
    }

    const teamMemberDeleteRoute = parsePath(
      path,
      /^\/api\/admin\/orgs\/([^/]+)\/teams\/([^/]+)\/members\/([^/]+)$/,
    );
    if (teamMemberDeleteRoute && method === "DELETE") {
      const [, teamId, userId] = teamMemberDeleteRoute;
      const existing = teamMembersByTeam.get(teamId) ?? [];
      teamMembersByTeam.set(
        teamId,
        existing.filter((member) => member.userId !== userId),
      );
      return route.fulfill({ status: 204 });
    }

    const orgMembersRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/members$/);
    if (orgMembersRoute && method === "GET") {
      const [orgId] = orgMembersRoute;
      if (!orgsById.has(orgId)) return json(route, 404, { code: 404, message: "org not found" });
      return json(route, 200, { items: orgMembersByOrg.get(orgId) ?? [] });
    }

    if (orgMembersRoute && method === "POST") {
      const [orgId] = orgMembersRoute;
      const payload = request.postDataJSON() as {
        userId?: string;
        role?: "owner" | "admin" | "member" | "viewer";
      };
      const userId = payload.userId?.trim() ?? "";
      const role = payload.role ?? "member";
      if (!userId) return json(route, 400, { code: 400, message: "userId is required" });
      const existing = orgMembersByOrg.get(orgId) ?? [];
      if (existing.some((member) => member.userId === userId)) {
        return json(route, 409, { code: 409, message: "membership already exists" });
      }
      const created: MockOrgMembership = {
        id: `om-${existing.length + 10}`,
        orgId,
        userId,
        role,
        createdAt: NOW_ISO,
      };
      orgMembersByOrg.set(orgId, [...existing, created]);
      state.addOrgMemberCalls += 1;
      return json(route, 201, created);
    }

    const orgMemberRoleRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/members\/([^/]+)\/role$/);
    if (orgMemberRoleRoute && method === "PUT") {
      const [orgId, userId] = orgMemberRoleRoute;
      const payload = request.postDataJSON() as { role?: MockOrgMembership["role"] };
      const role = payload.role ?? "member";
      const existing = orgMembersByOrg.get(orgId) ?? [];
      const index = existing.findIndex((member) => member.userId === userId);
      if (index < 0) {
        return json(route, 404, { code: 404, message: "org membership not found" });
      }
      const target = existing[index];
      const ownerCount = existing.filter((member) => member.role === "owner").length;
      if (target.role === "owner" && role !== "owner" && ownerCount === 1) {
        state.lastOwnerProtectionHits += 1;
        return json(route, 409, { code: 409, message: "cannot demote last owner" });
      }

      const updated: MockOrgMembership = { ...target, role };
      const next = [...existing];
      next[index] = updated;
      orgMembersByOrg.set(orgId, next);
      return json(route, 200, updated);
    }

    const orgMemberDeleteRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/members\/([^/]+)$/);
    if (orgMemberDeleteRoute && method === "DELETE") {
      const [orgId, userId] = orgMemberDeleteRoute;
      const existing = orgMembersByOrg.get(orgId) ?? [];
      const target = existing.find((member) => member.userId === userId);
      if (!target) {
        return json(route, 404, { code: 404, message: "org membership not found" });
      }
      const ownerCount = existing.filter((member) => member.role === "owner").length;
      if (target.role === "owner" && ownerCount === 1) {
        state.lastOwnerProtectionHits += 1;
        return json(route, 409, { code: 409, message: "cannot remove last owner" });
      }
      orgMembersByOrg.set(
        orgId,
        existing.filter((member) => member.userId !== userId),
      );
      return route.fulfill({ status: 204 });
    }

    const teamsRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/teams$/);
    if (teamsRoute && method === "GET") {
      const [orgId] = teamsRoute;
      return json(route, 200, { items: teamsByOrg.get(orgId) ?? [] });
    }

    if (teamsRoute && method === "POST") {
      const [orgId] = teamsRoute;
      const payload = request.postDataJSON() as { name?: string; slug?: string };
      const name = payload.name?.trim() ?? "";
      const slug = payload.slug?.trim() ?? "";
      if (!name || !slug) {
        return json(route, 400, { code: 400, message: "name and slug are required" });
      }
      const existing = teamsByOrg.get(orgId) ?? [];
      if (existing.some((team) => team.slug === slug)) {
        return json(route, 409, { code: 409, message: "team slug already exists" });
      }
      const created: MockTeam = {
        id: `team-${existing.length + 1}`,
        orgId,
        name,
        slug,
        createdAt: NOW_ISO,
        updatedAt: NOW_ISO,
      };
      teamsByOrg.set(orgId, [...existing, created]);
      teamMembersByTeam.set(created.id, []);
      state.createTeamCalls += 1;
      return json(route, 201, created);
    }

    const teamRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/teams\/([^/]+)$/);
    if (teamRoute && method === "GET") {
      const [orgId, teamId] = teamRoute;
      const team = (teamsByOrg.get(orgId) ?? []).find((candidate) => candidate.id === teamId);
      if (!team) return json(route, 404, { code: 404, message: "team not found" });
      return json(route, 200, team);
    }

    if (teamRoute && method === "PUT") {
      const [orgId, teamId] = teamRoute;
      const payload = request.postDataJSON() as { name?: string; slug?: string };
      const name = payload.name?.trim() ?? "";
      const slug = payload.slug?.trim() ?? "";
      if (!name || !slug) {
        return json(route, 400, { code: 400, message: "team name and slug are required" });
      }
      const existing = teamsByOrg.get(orgId) ?? [];
      const index = existing.findIndex((team) => team.id === teamId);
      if (index < 0) return json(route, 404, { code: 404, message: "team not found" });
      const updated: MockTeam = { ...existing[index], name, slug, updatedAt: NOW_ISO };
      const next = [...existing];
      next[index] = updated;
      teamsByOrg.set(orgId, next);
      return json(route, 200, updated);
    }

    if (teamRoute && method === "DELETE") {
      const [orgId, teamId] = teamRoute;
      teamsByOrg.set(
        orgId,
        (teamsByOrg.get(orgId) ?? []).filter((team) => team.id !== teamId),
      );
      teamMembersByTeam.delete(teamId);
      return route.fulfill({ status: 204 });
    }

    const orgTenantsRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/tenants$/);
    if (orgTenantsRoute && method === "GET") {
      const [orgId] = orgTenantsRoute;
      return json(route, 200, { items: tenantsByOrg.get(orgId) ?? [] });
    }

    if (orgTenantsRoute && method === "POST") {
      const [orgId] = orgTenantsRoute;
      const payload = request.postDataJSON() as { tenantId?: string };
      const tenantId = payload.tenantId?.trim() ?? "";
      if (!tenantId) {
        return json(route, 400, { code: 400, message: "tenantId is required" });
      }
      const existing = tenantsByOrg.get(orgId) ?? [];
      const alreadyAssigned = existing.some((tenant) => tenant.id === tenantId);
      const next = alreadyAssigned ? existing : [...existing, makeTenant(tenantId, orgId)];
      tenantsByOrg.set(orgId, next);
      state.assignTenantCalls += 1;
      return json(route, 200, { status: "assigned" });
    }

    const orgTenantDeleteRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/tenants\/([^/]+)$/);
    if (orgTenantDeleteRoute && method === "DELETE") {
      const [orgId, tenantId] = orgTenantDeleteRoute;
      tenantsByOrg.set(
        orgId,
        (tenantsByOrg.get(orgId) ?? []).filter((tenant) => tenant.id !== tenantId),
      );
      return route.fulfill({ status: 204 });
    }

    const orgUsageRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/usage$/);
    if (orgUsageRoute && method === "GET") {
      const [orgId] = orgUsageRoute;
      const from = url.searchParams.get("from");
      const to = url.searchParams.get("to");
      state.lastUsageQuery = {
        period: url.searchParams.get("period"),
        from,
        to,
      };
      if ((from && !to) || (!from && to)) {
        return json(route, 400, {
          code: 400,
          message: "both from and to are required when either is provided",
        });
      }
      const period = (url.searchParams.get("period") ?? "month") as "day" | "week" | "month";
      return json(route, 200, {
        orgId,
        tenantCount: (tenantsByOrg.get(orgId) ?? []).length,
        period,
        data: [
          {
            date: "2026-03-01",
            apiRequests: 123,
            storageBytesUsed: 2048,
            bandwidthBytes: 1024,
            functionInvocations: 42,
          },
        ],
        totals: {
          apiRequests: 123,
          storageBytesUsed: 2048,
          bandwidthBytes: 1024,
          functionInvocations: 42,
        },
      });
    }

    const orgAuditRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)\/audit$/);
    if (orgAuditRoute && method === "GET") {
      const [orgId] = orgAuditRoute;
      const limit = Number(url.searchParams.get("limit") ?? "50");
      const offset = Number(url.searchParams.get("offset") ?? "0");
      const actionFilter = url.searchParams.get("action");
      const resultFilter = url.searchParams.get("result");
      const actorFilter = url.searchParams.get("actor_id");
      state.lastAuditQuery = {
        limit,
        offset,
        from: url.searchParams.get("from"),
        to: url.searchParams.get("to"),
        action: actionFilter,
        result: resultFilter,
        actorId: actorFilter,
      };
      const source = auditByOrg.get(orgId) ?? [];
      const filtered = source.filter((event) => {
        if (actionFilter && event.action !== actionFilter) return false;
        if (resultFilter && event.result !== resultFilter) return false;
        if (actorFilter && event.actorId !== actorFilter) return false;
        return true;
      });
      const items = filtered.slice(offset, offset + limit);
      return json(route, 200, {
        items,
        count: filtered.length,
        limit,
        offset,
      });
    }

    const orgRoute = parsePath(path, /^\/api\/admin\/orgs\/([^/]+)$/);
    if (orgRoute && method === "GET") {
      const [orgId] = orgRoute;
      const org = orgsById.get(orgId);
      if (!org) return json(route, 404, { code: 404, message: "org not found" });
      return json(route, 200, buildOrgDetail(org, orgsById, teamsByOrg, tenantsByOrg));
    }

    if (orgRoute && method === "PUT") {
      const [orgId] = orgRoute;
      const org = orgsById.get(orgId);
      if (!org) return json(route, 404, { code: 404, message: "org not found" });

      const payload = request.postDataJSON() as {
        name?: string;
        slug?: string;
        parentOrgId?: string;
      };
      const nextName = payload.name?.trim() || org.name;
      const nextSlug = payload.slug?.trim() || org.slug;
      if (!isValidSlug(nextSlug)) {
        return json(route, 400, { code: 400, message: "invalid slug format" });
      }
      if (
        nextSlug !== org.slug &&
        Array.from(orgsById.values()).some((candidate) => candidate.id !== orgId && candidate.slug === nextSlug)
      ) {
        return json(route, 409, { code: 409, message: "organization slug already exists" });
      }

      const trimmedParent = payload.parentOrgId?.trim();
      if (trimmedParent === orgId || trimmedParent === "org-cycle") {
        return json(route, 409, { code: 409, message: "circular parent org hierarchy" });
      }

      const updated: MockOrganization = {
        ...org,
        name: nextName,
        slug: nextSlug,
        parentOrgId: trimmedParent === "" ? null : (trimmedParent ?? org.parentOrgId),
        updatedAt: NOW_ISO,
      };
      orgsById.set(orgId, updated);
      return json(route, 200, updated);
    }

    if (orgRoute && method === "DELETE") {
      const [orgId] = orgRoute;
      const confirm = url.searchParams.get("confirm");
      if (confirm !== "true") {
        return json(route, 400, { code: 400, message: "confirm=true is required" });
      }
      state.deleteConfirmTrueCalls += 1;
      orgsById.delete(orgId);
      orgMembersByOrg.delete(orgId);
      teamsByOrg.delete(orgId);
      tenantsByOrg.delete(orgId);
      auditByOrg.delete(orgId);
      return route.fulfill({ status: 204 });
    }

    return json(route, 500, {
      message: `Unhandled mocked API route: ${method} ${path}`,
    });
  });

  return state;
}

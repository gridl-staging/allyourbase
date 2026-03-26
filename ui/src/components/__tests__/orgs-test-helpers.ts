/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/__tests__/orgs-test-helpers.ts.
 */
import { beforeEach, vi } from "vitest";
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
} from "../../api_orgs";
import type {
  OrgAuditResponse,
  OrgDetailResponse,
  OrgListResponse,
  OrgMembership,
  OrgMembershipListResponse,
  OrgUsageSummary,
  Organization,
  Team,
  TeamMembership,
  TeamMemberListResponse,
  TeamListResponse,
} from "../../types/organizations";
import type { Tenant } from "../../types/tenants";

vi.mock("../../api_orgs", () => ({
  fetchOrgList: vi.fn(),
  createOrg: vi.fn(),
  getOrg: vi.fn(),
  updateOrg: vi.fn(),
  deleteOrg: vi.fn(),
  fetchOrgMembers: vi.fn(),
  addOrgMember: vi.fn(),
  updateOrgMemberRole: vi.fn(),
  removeOrgMember: vi.fn(),
  assignTenantToOrg: vi.fn(),
  fetchOrgTenants: vi.fn(),
  unassignTenantFromOrg: vi.fn(),
  createTeam: vi.fn(),
  fetchTeams: vi.fn(),
  getTeam: vi.fn(),
  updateTeam: vi.fn(),
  deleteTeam: vi.fn(),
  fetchTeamMembers: vi.fn(),
  addTeamMember: vi.fn(),
  updateTeamMemberRole: vi.fn(),
  removeTeamMember: vi.fn(),
  fetchOrgUsage: vi.fn(),
  fetchOrgAudit: vi.fn(),
}));

export const mockFetchOrgList = vi.mocked(fetchOrgList);
export const mockCreateOrg = vi.mocked(createOrg);
export const mockGetOrg = vi.mocked(getOrg);
export const mockUpdateOrg = vi.mocked(updateOrg);
export const mockDeleteOrg = vi.mocked(deleteOrg);
export const mockFetchOrgMembers = vi.mocked(fetchOrgMembers);
export const mockAddOrgMember = vi.mocked(addOrgMember);
export const mockUpdateOrgMemberRole = vi.mocked(updateOrgMemberRole);
export const mockRemoveOrgMember = vi.mocked(removeOrgMember);
export const mockAssignTenantToOrg = vi.mocked(assignTenantToOrg);
export const mockFetchOrgTenants = vi.mocked(fetchOrgTenants);
export const mockUnassignTenantFromOrg = vi.mocked(unassignTenantFromOrg);
export const mockCreateTeam = vi.mocked(createTeam);
export const mockFetchTeams = vi.mocked(fetchTeams);
export const mockGetTeam = vi.mocked(getTeam);
export const mockUpdateTeam = vi.mocked(updateTeam);
export const mockDeleteTeam = vi.mocked(deleteTeam);
export const mockFetchTeamMembers = vi.mocked(fetchTeamMembers);
export const mockAddTeamMember = vi.mocked(addTeamMember);
export const mockUpdateTeamMemberRole = vi.mocked(updateTeamMemberRole);
export const mockRemoveTeamMember = vi.mocked(removeTeamMember);
export const mockFetchOrgUsage = vi.mocked(fetchOrgUsage);
export const mockFetchOrgAudit = vi.mocked(fetchOrgAudit);

export function makeOrg(overrides: Partial<Organization> = {}): Organization {
  return {
    id: "org-1",
    name: "Acme Inc",
    slug: "acme-inc",
    parentOrgId: null,
    planTier: "pro",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
    ...overrides,
  };
}

export function makeOrgDetail(overrides: Partial<OrgDetailResponse> = {}): OrgDetailResponse {
  return {
    ...makeOrg(),
    childOrgCount: 1,
    teamCount: 2,
    tenantCount: 3,
    ...overrides,
  };
}

export function makeOrgListResponse(overrides: Partial<OrgListResponse> = {}): OrgListResponse {
  return {
    items: [
      makeOrg(),
      makeOrg({ id: "org-2", name: "Beta Corp", slug: "beta-corp" }),
    ],
    ...overrides,
  };
}

export function makeOrgMembership(overrides: Partial<OrgMembership> = {}): OrgMembership {
  return {
    id: "om-1",
    orgId: "org-1",
    userId: "u-1",
    role: "owner",
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export function makeOrgMemberList(overrides: Partial<OrgMembershipListResponse> = {}): OrgMembershipListResponse {
  return {
    items: [
      makeOrgMembership(),
      makeOrgMembership({ id: "om-2", userId: "u-2", role: "admin" }),
    ],
    ...overrides,
  };
}

export function makeTeam(overrides: Partial<Team> = {}): Team {
  return {
    id: "team-1",
    orgId: "org-1",
    name: "Engineering",
    slug: "engineering",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
    ...overrides,
  };
}

export function makeTeamList(overrides: Partial<TeamListResponse> = {}): TeamListResponse {
  return {
    items: [
      makeTeam(),
      makeTeam({ id: "team-2", name: "Design", slug: "design" }),
    ],
    ...overrides,
  };
}

export function makeTeamMembership(overrides: Partial<TeamMembership> = {}): TeamMembership {
  return {
    id: "tm-1",
    teamId: "team-1",
    userId: "u-1",
    role: "lead",
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export function makeTeamMemberList(overrides: Partial<TeamMemberListResponse> = {}): TeamMemberListResponse {
  return {
    items: [
      makeTeamMembership(),
      makeTeamMembership({ id: "tm-2", userId: "u-2", role: "member" }),
    ],
    ...overrides,
  };
}

/**
 * TODO: Document makeTenantForOrg.
 */
export function makeTenantForOrg(overrides: Partial<Tenant> = {}): Tenant {
  return {
    id: "t-1",
    name: "Acme Tenant",
    slug: "acme-tenant",
    isolationMode: "shared",
    planTier: "pro",
    region: "us-east-1",
    orgId: "org-1",
    orgMetadata: null,
    state: "active",
    idempotencyKey: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
    ...overrides,
  };
}

/**
 * TODO: Document makeUsageSummary.
 */
export function makeUsageSummary(overrides: Partial<OrgUsageSummary> = {}): OrgUsageSummary {
  return {
    orgId: "org-1",
    tenantCount: 3,
    period: "month",
    data: [
      {
        date: "2026-01-01",
        apiRequests: 100,
        storageBytesUsed: 500,
        bandwidthBytes: 200,
        functionInvocations: 50,
      },
    ],
    totals: {
      apiRequests: 100,
      storageBytesUsed: 500,
      bandwidthBytes: 200,
      functionInvocations: 50,
    },
    ...overrides,
  };
}

/**
 * TODO: Document makeAuditResponse.
 */
export function makeAuditResponse(overrides: Partial<OrgAuditResponse> = {}): OrgAuditResponse {
  return {
    items: [
      {
        id: "a-1",
        tenantId: "org-1",
        actorId: "u-1",
        action: "org.created",
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
 * TODO: Document setupOrgTestMocks.
 */
export function setupOrgTestMocks() {
  mockFetchOrgList.mockResolvedValue(makeOrgListResponse());
  mockGetOrg.mockResolvedValue(makeOrgDetail());
  mockCreateOrg.mockResolvedValue(makeOrg({ id: "org-3", name: "Gamma", slug: "gamma" }));
  mockUpdateOrg.mockResolvedValue(makeOrg({ name: "Acme Updated" }));
  mockDeleteOrg.mockResolvedValue(undefined);
  mockFetchOrgMembers.mockResolvedValue(makeOrgMemberList());
  mockAddOrgMember.mockResolvedValue(
    makeOrgMembership({ id: "om-3", userId: "u-3", role: "viewer" }),
  );
  mockUpdateOrgMemberRole.mockResolvedValue(
    makeOrgMembership({ id: "om-2", userId: "u-2", role: "viewer" }),
  );
  mockRemoveOrgMember.mockResolvedValue(undefined);
  mockAssignTenantToOrg.mockResolvedValue({ status: "assigned" });
  mockFetchOrgTenants.mockResolvedValue({ items: [makeTenantForOrg()] });
  mockUnassignTenantFromOrg.mockResolvedValue(undefined);
  mockFetchTeams.mockResolvedValue(makeTeamList());
  mockCreateTeam.mockResolvedValue(makeTeam({ id: "team-3", name: "QA", slug: "qa" }));
  mockGetTeam.mockResolvedValue(makeTeam());
  mockUpdateTeam.mockResolvedValue(makeTeam({ name: "Engineering Updated" }));
  mockDeleteTeam.mockResolvedValue(undefined);
  mockFetchTeamMembers.mockResolvedValue(makeTeamMemberList());
  mockAddTeamMember.mockResolvedValue(
    makeTeamMembership({ id: "tm-3", userId: "u-3", role: "member" }),
  );
  mockUpdateTeamMemberRole.mockResolvedValue(
    makeTeamMembership({ id: "tm-2", userId: "u-2", role: "lead" }),
  );
  mockRemoveTeamMember.mockResolvedValue(undefined);
  mockFetchOrgUsage.mockResolvedValue(makeUsageSummary());
  mockFetchOrgAudit.mockResolvedValue(makeAuditResponse());
}

beforeEach(() => {
  vi.clearAllMocks();
  setupOrgTestMocks();
});

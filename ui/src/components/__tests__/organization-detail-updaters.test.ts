import { describe, expect, it } from "vitest";
import type { OrgDetailState } from "../organizations-hooks";
import type { OrgDetailResponse, OrgMembership, Team, TeamMembership } from "../../types/organizations";
import type { Tenant } from "../../types/tenants";
import {
  applyCreatedOrgMember,
  applyCreatedTeam,
  applyCreatedTeamMember,
  applyDeletedOrgMember,
  applyDeletedTeam,
  applyDeletedTeamMember,
  applyTenantAssignment,
  applyTenantUnassignment,
  applyUpdatedOrgMemberRole,
  applyUpdatedTeam,
  applyUpdatedTeamMemberRole,
} from "../organization-detail-updaters";

function makeOrgDetail(overrides: Partial<OrgDetailResponse> = {}): OrgDetailResponse {
  return {
    id: "org-1",
    name: "Acme",
    slug: "acme",
    parentOrgId: null,
    planTier: "free",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    childOrgCount: 0,
    teamCount: 1,
    tenantCount: 1,
    ...overrides,
  };
}

function makeTeam(overrides: Partial<Team> = {}): Team {
  return {
    id: "team-1",
    orgId: "org-1",
    name: "Engineering",
    slug: "engineering",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeTeamMember(overrides: Partial<TeamMembership> = {}): TeamMembership {
  return {
    id: "tm-1",
    teamId: "team-1",
    userId: "user-1",
    role: "member",
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeTenant(overrides: Partial<Tenant> = {}): Tenant {
  return {
    id: "tenant-1",
    name: "Primary",
    slug: "primary",
    isolationMode: "shared",
    planTier: "free",
    region: "us-east-1",
    orgId: "org-1",
    orgMetadata: null,
    state: "active",
    idempotencyKey: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeState(overrides: Partial<OrgDetailState> = {}): OrgDetailState {
  const teams = [makeTeam()];
  const teamMembersByTeamId: Record<string, TeamMembership[]> = {
    "team-1": [makeTeamMember()],
  };
  const members: OrgMembership[] = [
    {
      id: "om-1",
      orgId: "org-1",
      userId: "user-1",
      role: "owner",
      createdAt: "2026-01-01T00:00:00Z",
    },
  ];

  return {
    org: makeOrgDetail(),
    members,
    teams,
    teamMembersByTeamId,
    tenants: [makeTenant()],
    usage: null,
    auditItems: [],
    auditCount: 0,
    isLoading: false,
    error: null,
    ...overrides,
  };
}

describe("organization detail updaters", () => {
  it("appends a new org member and clears error", () => {
    const state = makeState({ error: "stale error" });
    const newMember: OrgMembership = {
      id: "om-2",
      orgId: "org-1",
      userId: "user-2",
      role: "admin",
      createdAt: "2026-01-02T00:00:00Z",
    };

    const next = applyCreatedOrgMember(state, newMember);

    expect(next.members).toHaveLength(2);
    expect(next.members[1]?.userId).toBe("user-2");
    expect(next.members[1]?.role).toBe("admin");
    expect(next.error).toBeNull();
  });

  it("removes an org member by userId", () => {
    const state = makeState({
      members: [
        { id: "om-1", orgId: "org-1", userId: "user-1", role: "owner", createdAt: "2026-01-01T00:00:00Z" },
        { id: "om-2", orgId: "org-1", userId: "user-2", role: "member", createdAt: "2026-01-01T00:00:00Z" },
      ],
    });

    const next = applyDeletedOrgMember(state, "user-2");

    expect(next.members).toHaveLength(1);
    expect(next.members[0]?.userId).toBe("user-1");
    expect(next.error).toBeNull();
  });

  it("updates an org member role in place without reordering", () => {
    const state = makeState({
      members: [
        { id: "om-1", orgId: "org-1", userId: "user-1", role: "owner", createdAt: "2026-01-01T00:00:00Z" },
        { id: "om-2", orgId: "org-1", userId: "user-2", role: "member", createdAt: "2026-01-01T00:00:00Z" },
      ],
    });

    const next = applyUpdatedOrgMemberRole(state, "user-2", "admin");

    expect(next.members[0]?.role).toBe("owner");
    expect(next.members[1]?.role).toBe("admin");
    expect(next.error).toBeNull();
  });

  it("replaces a team in the list after update", () => {
    const originalTeam = makeTeam({ name: "Engineering", slug: "engineering" });
    const updatedTeam = makeTeam({ name: "Platform", slug: "platform" });
    const state = makeState({ teams: [originalTeam] });

    const next = applyUpdatedTeam(state, "team-1", updatedTeam);

    expect(next.teams).toHaveLength(1);
    expect(next.teams[0]?.name).toBe("Platform");
    expect(next.teams[0]?.slug).toBe("platform");
    expect(next.error).toBeNull();
  });

  it("appends a team member to the selected team cache", () => {
    const state = makeState();
    const newMember = makeTeamMember({ id: "tm-2", userId: "user-2", role: "lead" });

    const next = applyCreatedTeamMember(state, "team-1", newMember);

    expect(next.teamMembersByTeamId["team-1"]).toHaveLength(2);
    expect(next.teamMembersByTeamId["team-1"][1]?.userId).toBe("user-2");
    expect(next.teamMembersByTeamId["team-1"][1]?.role).toBe("lead");
    expect(next.error).toBeNull();
  });

  it("creates team member cache entry for a team with no prior members", () => {
    const state = makeState({ teamMembersByTeamId: {} });
    const newMember = makeTeamMember({ id: "tm-1", teamId: "team-1", userId: "user-1" });

    const next = applyCreatedTeamMember(state, "team-1", newMember);

    expect(next.teamMembersByTeamId["team-1"]).toHaveLength(1);
  });

  it("removes a team member by userId from the cache", () => {
    const state = makeState({
      teamMembersByTeamId: {
        "team-1": [
          makeTeamMember({ userId: "user-1" }),
          makeTeamMember({ id: "tm-2", userId: "user-2" }),
        ],
      },
    });

    const next = applyDeletedTeamMember(state, "team-1", "user-2");

    expect(next.teamMembersByTeamId["team-1"]).toHaveLength(1);
    expect(next.teamMembersByTeamId["team-1"][0]?.userId).toBe("user-1");
    expect(next.error).toBeNull();
  });

  it("handles deleting team member from a team with no cached members", () => {
    const state = makeState({ teamMembersByTeamId: {} });

    const next = applyDeletedTeamMember(state, "team-x", "user-1");

    expect(next.teamMembersByTeamId["team-x"]).toEqual([]);
  });

  it("adds a created team and increments team count", () => {
    const state = makeState();

    const next = applyCreatedTeam(state, makeTeam({ id: "team-2", name: "Design", slug: "design" }));

    expect(next.teams).toHaveLength(2);
    expect(next.teams.map((team) => team.id)).toEqual(["team-1", "team-2"]);
    expect(next.org?.teamCount).toBe(2);
  });

  it("deletes a team, prunes member cache, and decrements count", () => {
    const state = makeState({
      org: makeOrgDetail({ teamCount: 2 }),
      teams: [makeTeam(), makeTeam({ id: "team-2", name: "Design", slug: "design" })],
      teamMembersByTeamId: {
        "team-1": [makeTeamMember()],
        "team-2": [makeTeamMember({ id: "tm-2", teamId: "team-2" })],
      },
    });

    const next = applyDeletedTeam(state, "team-2");

    expect(next.teams.map((team) => team.id)).toEqual(["team-1"]);
    expect(next.teamMembersByTeamId["team-2"]).toBeUndefined();
    expect(next.org?.teamCount).toBe(1);
  });

  it("updates one team member role in the selected team", () => {
    const state = makeState({
      teamMembersByTeamId: {
        "team-1": [makeTeamMember({ userId: "user-1", role: "member" })],
        "team-2": [makeTeamMember({ id: "tm-2", teamId: "team-2", userId: "user-1", role: "member" })],
      },
    });

    const next = applyUpdatedTeamMemberRole(state, "team-1", "user-1", "lead");

    expect(next.teamMembersByTeamId["team-1"][0]?.role).toBe("lead");
    expect(next.teamMembersByTeamId["team-2"][0]?.role).toBe("member");
  });

  it("replaces tenant list after assignment and syncs tenant count", () => {
    const state = makeState({ org: makeOrgDetail({ tenantCount: 1 }), tenants: [makeTenant()] });

    const next = applyTenantAssignment(state, [makeTenant(), makeTenant({ id: "tenant-2", slug: "second" })]);

    expect(next.tenants).toHaveLength(2);
    expect(next.org?.tenantCount).toBe(2);
  });

  it("unassigns tenant and never drops tenant count below zero", () => {
    const state = makeState({ org: makeOrgDetail({ tenantCount: 0 }), tenants: [makeTenant()] });

    const next = applyTenantUnassignment(state, "tenant-1");

    expect(next.tenants).toHaveLength(0);
    expect(next.org?.tenantCount).toBe(0);
  });
});

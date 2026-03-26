import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  makeOrgDetail,
  makeOrgListResponse,
  makeTeam,
  makeTeamList,
  makeTeamMemberList,
  makeTeamMembership,
  mockAddOrgMember,
  mockAddTeamMember,
  mockAssignTenantToOrg,
  mockCreateTeam,
  mockDeleteTeam,
  mockDeleteOrg,
  mockFetchOrgAudit,
  mockFetchOrgList,
  mockFetchTeams,
  mockFetchOrgUsage,
  mockFetchTeamMembers,
  mockCreateOrg,
  mockGetTeam,
  mockGetOrg,
  mockRemoveOrgMember,
  mockUnassignTenantFromOrg,
  mockUpdateTeam,
  mockUpdateOrg,
  mockUpdateOrgMemberRole,
  mockUpdateTeamMemberRole,
  mockRemoveTeamMember,
} from "./orgs-test-helpers";
import { renderWithProviders } from "../../test-utils";
import { Organizations } from "../Organizations";

describe("Organizations", () => {
  it("renders loading state while org list is pending", () => {
    mockFetchOrgList.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<Organizations />);
    expect(screen.getByText(/loading organizations/i)).toBeInTheDocument();
  });

  it("renders org list with names after load", async () => {
    renderWithProviders(<Organizations />);
    await expect(screen.findByText("Acme Inc")).resolves.toBeInTheDocument();
    expect(screen.getByText("Beta Corp")).toBeInTheDocument();
  });

  it("renders empty state when no organizations exist", async () => {
    mockFetchOrgList.mockResolvedValueOnce(makeOrgListResponse({ items: [] }));
    renderWithProviders(<Organizations />);
    await expect(screen.findByText(/no organizations found/i)).resolves.toBeInTheDocument();
  });

  it("renders error state when list fetch fails", async () => {
    mockFetchOrgList.mockRejectedValueOnce(new Error("server error"));
    renderWithProviders(<Organizations />);
    await expect(screen.findByText(/failed to load organizations/i)).resolves.toBeInTheDocument();
  });

  it("selects org and loads detail panel with info tab", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    const infoSection = await screen.findByTestId("org-info-section");
    expect(within(infoSection).getByText("acme-inc")).toBeInTheDocument();
    expect(within(infoSection).getByText("pro")).toBeInTheDocument();
  });

  it("shows enriched counts in detail header", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    expect(screen.getByText(/1 child org/i)).toBeInTheDocument();
    expect(screen.getByText(/2 teams/i)).toBeInTheDocument();
    expect(screen.getByText(/3 tenants/i)).toBeInTheDocument();
  });

  it("updates org name from the info panel", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.clear(screen.getByLabelText("Organization Name"));
    await user.type(screen.getByLabelText("Organization Name"), "Acme Updated");
    await user.click(screen.getByRole("button", { name: "Save Info" }));

    await waitFor(() =>
      expect(mockUpdateOrg).toHaveBeenCalledWith("org-1", expect.objectContaining({ name: "Acme Updated" })),
    );
  });

  it("blocks org-info save when organization name is empty", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");

    await user.clear(screen.getByLabelText("Organization Name"));
    await user.click(screen.getByRole("button", { name: "Save Info" }));

    expect(screen.getByText(/organization name is required/i)).toBeInTheDocument();
    expect(mockUpdateOrg).not.toHaveBeenCalled();
  });

  it("creates an organization from the list panel form", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");

    await user.type(screen.getByLabelText("Create Org Name"), "Gamma");
    await user.type(screen.getByLabelText("Create Org Slug"), "gamma");
    await user.selectOptions(screen.getByLabelText("Create Org Plan Tier"), "pro");
    await user.type(screen.getByLabelText("Create Org Parent ID"), "org-parent");
    await user.click(screen.getByRole("button", { name: "Create Org" }));

    await waitFor(() =>
      expect(mockCreateOrg).toHaveBeenCalledWith({
        name: "Gamma",
        slug: "gamma",
        planTier: "pro",
        parentOrgId: "org-parent",
      }),
    );
  });

  it("validates create-org required fields before submitting", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");

    await user.click(screen.getByRole("button", { name: "Create Org" }));

    expect(screen.getByText(/organization name and slug are required/i)).toBeInTheDocument();
    expect(mockCreateOrg).not.toHaveBeenCalled();
  });

  it("surfaces slug-format validation errors from org creation", async () => {
    mockCreateOrg.mockRejectedValueOnce(new Error("invalid slug format"));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");

    await user.type(screen.getByLabelText("Create Org Name"), "Bad Slug Org");
    await user.type(screen.getByLabelText("Create Org Slug"), "bad slug");
    await user.click(screen.getByRole("button", { name: "Create Org" }));

    await expect(screen.findByText(/invalid slug format/i)).resolves.toBeInTheDocument();
  });

  it("surfaces duplicate-slug conflicts from org creation", async () => {
    mockCreateOrg.mockRejectedValueOnce(new Error("organization slug already exists"));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");

    await user.type(screen.getByLabelText("Create Org Name"), "Acme Clone");
    await user.type(screen.getByLabelText("Create Org Slug"), "acme-inc");
    await user.click(screen.getByRole("button", { name: "Create Org" }));

    await expect(screen.findByText(/organization slug already exists/i)).resolves.toBeInTheDocument();
  });

  it("surfaces circular-parent conflicts from org creation", async () => {
    mockCreateOrg.mockRejectedValueOnce(new Error("circular parent org hierarchy"));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");

    await user.type(screen.getByLabelText("Create Org Name"), "Org With Cycle");
    await user.type(screen.getByLabelText("Create Org Slug"), "org-with-cycle");
    await user.type(screen.getByLabelText("Create Org Parent ID"), "org-cycle");
    await user.click(screen.getByRole("button", { name: "Create Org" }));

    await expect(screen.findByText(/circular parent org hierarchy/i)).resolves.toBeInTheDocument();
  });

  it("sends empty parentOrgId when clearing parent linkage", async () => {
    mockGetOrg.mockResolvedValueOnce(makeOrgDetail({ parentOrgId: "org-parent" }));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");

    await user.clear(screen.getByLabelText("Parent Org ID"));
    await user.click(screen.getByRole("button", { name: "Save Info" }));

    await waitFor(() =>
      expect(mockUpdateOrg).toHaveBeenCalledWith(
        "org-1",
        expect.objectContaining({ parentOrgId: "" }),
      ),
    );
  });

  it("switches to Members tab and shows member list", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.click(screen.getByText("Members"));
    await expect(screen.findByText("u-1")).resolves.toBeInTheDocument();
    expect(screen.getByTestId("org-member-role-u-1")).toHaveTextContent("owner");
    expect(screen.getByText("u-2")).toBeInTheDocument();
    expect(screen.getByTestId("org-member-role-u-2")).toHaveTextContent("admin");
  });

  it("adds a member from the members panel", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Members"));
    await screen.findByTestId("org-members-section");

    await user.type(screen.getByLabelText("New Member User ID"), "u-3");
    await user.selectOptions(screen.getByLabelText("New Member Role"), "viewer");
    await user.click(screen.getByRole("button", { name: "Add Member" }));

    await waitFor(() => expect(mockAddOrgMember).toHaveBeenCalledWith("org-1", "u-3", "viewer"));
    expect(screen.getByText("u-3")).toBeInTheDocument();
  });

  it("blocks org-member creation when user id is blank", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Members"));
    await screen.findByTestId("org-members-section");

    await user.click(screen.getByRole("button", { name: "Add Member" }));

    expect(screen.getByText(/member user id is required/i)).toBeInTheDocument();
    expect(mockAddOrgMember).not.toHaveBeenCalled();
  });

  it("removes a member from the members panel", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Members"));
    await screen.findByTestId("org-member-row-u-2");

    await user.click(
      within(screen.getByTestId("org-member-row-u-2")).getByRole("button", { name: "Remove" }),
    );

    await waitFor(() => expect(mockRemoveOrgMember).toHaveBeenCalledWith("org-1", "u-2"));
    expect(screen.queryByTestId("org-member-row-u-2")).not.toBeInTheDocument();
  });

  it("surfaces remove-last-owner protection when deleting the final owner", async () => {
    mockRemoveOrgMember.mockRejectedValueOnce(new Error("cannot remove last owner"));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Members"));
    const ownerRow = await screen.findByTestId("org-member-row-u-1");

    await user.click(within(ownerRow).getByRole("button", { name: "Remove" }));

    await expect(screen.findByText(/cannot remove last owner/i)).resolves.toBeInTheDocument();
    expect(screen.getByTestId("org-member-row-u-1")).toBeInTheDocument();
  });

  it("updates member role via Members tab", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Members"));
    const memberRow = await screen.findByTestId("org-member-row-u-2");

    await user.selectOptions(within(memberRow).getByLabelText("Role for u-2"), "viewer");
    await user.click(within(memberRow).getByRole("button", { name: "Update Role" }));

    await waitFor(() => expect(mockUpdateOrgMemberRole).toHaveBeenCalledWith("org-1", "u-2", "viewer"));
  });

  it("switches to Teams tab and shows team list", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.click(screen.getByText("Teams"));
    await expect(screen.findByText("Engineering")).resolves.toBeInTheDocument();
    expect(screen.getByText("Design")).toBeInTheDocument();
  });

  it("creates a team from the Teams tab", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");

    await user.type(screen.getByLabelText("Team Name"), "QA");
    await user.type(screen.getByLabelText("Team Slug"), "qa");
    await user.click(screen.getByRole("button", { name: "Create Team" }));

    await waitFor(() =>
      expect(mockCreateTeam).toHaveBeenCalledWith("org-1", { name: "QA", slug: "qa" }),
    );
  });

  it("blocks team creation when name or slug is missing", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");

    await user.click(screen.getByRole("button", { name: "Create Team" }));

    expect(screen.getByText(/team name and slug are required/i)).toBeInTheDocument();
    expect(mockCreateTeam).not.toHaveBeenCalled();
  });

  it("selects a team and shows team members", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");
    await user.click(screen.getByText("Engineering"));

    await expect(screen.findByTestId("team-members-section")).resolves.toBeInTheDocument();
    expect(mockGetTeam).toHaveBeenCalledWith("org-1", "team-1");
    expect(screen.getByTestId("team-member-role-u-1")).toHaveTextContent("lead");
    expect(screen.getByTestId("team-member-role-u-2")).toHaveTextContent("member");
  });

  it("adds a team member from the selected team panel", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");
    await user.click(screen.getByText("Engineering"));
    await screen.findByTestId("team-members-section");

    await user.type(screen.getByLabelText("Team Member User ID"), "u-3");
    await user.selectOptions(screen.getByLabelText("Team Member Role"), "member");
    await user.click(screen.getByRole("button", { name: "Add Member" }));

    await waitFor(() =>
      expect(mockAddTeamMember).toHaveBeenCalledWith("org-1", "team-1", "u-3", "member"),
    );
    expect(screen.getByTestId("team-member-row-u-3")).toBeInTheDocument();
  });

  it("blocks team-member creation when user id is blank", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");
    await user.click(screen.getByText("Engineering"));
    await screen.findByTestId("team-members-section");

    await user.click(screen.getByRole("button", { name: "Add Member" }));

    expect(screen.getByText(/member user id is required/i)).toBeInTheDocument();
    expect(mockAddTeamMember).not.toHaveBeenCalled();
  });

  it("surfaces org-member prerequisite conflicts when adding a team member", async () => {
    mockAddTeamMember.mockRejectedValueOnce(new Error("user must be an org member before joining a team"));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");
    await user.click(screen.getByText("Engineering"));
    await screen.findByTestId("team-members-section");

    await user.type(screen.getByLabelText("Team Member User ID"), "u-99");
    await user.click(screen.getByRole("button", { name: "Add Member" }));

    await expect(
      screen.findByText(/user must be an org member before joining a team/i),
    ).resolves.toBeInTheDocument();
  });

  it("updates a team member role from the selected team panel", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");
    await user.click(screen.getByText("Engineering"));
    const memberRow = await screen.findByTestId("team-member-row-u-2");

    await user.selectOptions(within(memberRow).getByLabelText("Team role for u-2"), "lead");
    await user.click(within(memberRow).getByRole("button", { name: "Update Role" }));

    await waitFor(() =>
      expect(mockUpdateTeamMemberRole).toHaveBeenCalledWith("org-1", "team-1", "u-2", "lead"),
    );
  });

  it("removes a team member from the selected team panel", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");
    await user.click(screen.getByText("Engineering"));
    const memberRow = await screen.findByTestId("team-member-row-u-2");

    await user.click(within(memberRow).getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(mockRemoveTeamMember).toHaveBeenCalledWith("org-1", "team-1", "u-2"));
    expect(screen.queryByTestId("team-member-row-u-2")).not.toBeInTheDocument();
  });

  it("keeps team-member role drafts scoped to the selected team", async () => {
    mockGetTeam
      .mockResolvedValueOnce(makeTeam())
      .mockResolvedValueOnce(makeTeam({ id: "team-2", name: "Design", slug: "design" }));
    mockFetchTeamMembers
      .mockResolvedValueOnce(makeTeamMemberList())
      .mockResolvedValueOnce(
        makeTeamMemberList({
          items: [makeTeamMembership({ id: "tm-9", teamId: "team-2", userId: "u-1", role: "lead" })],
        }),
      );
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");

    await user.click(screen.getByText("Engineering"));
    const engineeringMemberRow = await screen.findByTestId("team-member-row-u-1");
    await user.selectOptions(within(engineeringMemberRow).getByLabelText("Team role for u-1"), "member");

    await user.click(screen.getByText("Design"));
    const designMemberRow = await screen.findByTestId("team-member-row-u-1");
    expect(within(designMemberRow).getByLabelText("Team role for u-1")).toHaveValue("lead");
  });

  it("updates a selected team from the Teams tab", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");

    await user.click(screen.getByText("Engineering"));
    await screen.findByTestId("team-members-section");
    await user.clear(screen.getByLabelText("Selected Team Name"));
    await user.type(screen.getByLabelText("Selected Team Name"), "Platform");
    await user.clear(screen.getByLabelText("Selected Team Slug"));
    await user.type(screen.getByLabelText("Selected Team Slug"), "platform");
    await user.click(screen.getByRole("button", { name: "Save Team" }));

    await waitFor(() =>
      expect(mockUpdateTeam).toHaveBeenCalledWith("org-1", "team-1", {
        name: "Platform",
        slug: "platform",
      }),
    );
  });

  it("blocks selected-team save when required fields are empty", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");

    await user.click(screen.getByText("Engineering"));
    await screen.findByTestId("team-members-section");
    await user.clear(screen.getByLabelText("Selected Team Name"));
    await user.click(screen.getByRole("button", { name: "Save Team" }));

    expect(screen.getByText(/team name and slug are required/i)).toBeInTheDocument();
    expect(mockUpdateTeam).not.toHaveBeenCalled();
  });

  it("deletes a selected team from the Teams tab", async () => {
    mockFetchTeams
      .mockResolvedValueOnce(makeTeamList())
      .mockResolvedValueOnce(makeTeamList())
      .mockResolvedValueOnce(
        makeTeamList({ items: [makeTeam({ id: "team-2", name: "Design", slug: "design" })] }),
      );
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));
    await screen.findByText("Engineering");

    await user.click(screen.getByText("Engineering"));
    await screen.findByTestId("team-members-section");
    await user.click(screen.getByRole("button", { name: "Delete Team" }));

    await waitFor(() => expect(mockDeleteTeam).toHaveBeenCalledWith("org-1", "team-1"));
    await waitFor(() => expect(screen.queryByText("Engineering")).not.toBeInTheDocument());
  });

  it("switches to Tenants tab and shows assigned tenants", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.click(screen.getByText("Tenants"));
    await expect(screen.findByText("Acme Tenant")).resolves.toBeInTheDocument();
  });

  it("assigns a tenant from the Tenants tab", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Tenants"));
    await screen.findByText("Acme Tenant");

    await user.type(screen.getByLabelText("Tenant ID"), "t-99");
    await user.click(screen.getByRole("button", { name: "Assign Tenant" }));

    await waitFor(() => expect(mockAssignTenantToOrg).toHaveBeenCalledWith("org-1", "t-99"));
  });

  it("blocks tenant assignment when tenant id is blank", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Tenants"));
    await screen.findByText("Acme Tenant");

    await user.click(screen.getByRole("button", { name: "Assign Tenant" }));

    expect(screen.getByText(/tenant id is required/i)).toBeInTheDocument();
    expect(mockAssignTenantToOrg).not.toHaveBeenCalled();
  });

  it("unassigns a tenant from the Tenants tab", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Tenants"));
    await screen.findByText("Acme Tenant");

    await user.click(screen.getByRole("button", { name: "Unassign" }));

    await waitFor(() => expect(mockUnassignTenantFromOrg).toHaveBeenCalledWith("org-1", "t-1"));
    expect(screen.queryByText("Acme Tenant")).not.toBeInTheDocument();
  });

  it("switches to Usage tab and renders usage data", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.click(screen.getByText("Usage"));
    const usageSection = await screen.findByTestId("org-usage-section");
    expect(within(usageSection).getByText(/API Requests/)).toBeInTheDocument();
  });

  it("applies usage period and custom date-range filters", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.click(screen.getByText("Usage"));
    await screen.findByTestId("org-usage-section");

    await user.selectOptions(screen.getByLabelText("Period"), "week");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));
    await waitFor(() =>
      expect(mockFetchOrgUsage).toHaveBeenLastCalledWith("org-1", {
        period: "week",
        from: null,
        to: null,
      }),
    );

    await user.type(screen.getByLabelText("From"), "2026-01-01");
    await user.type(screen.getByLabelText("To"), "2026-01-15");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));
    await waitFor(() =>
      expect(mockFetchOrgUsage).toHaveBeenLastCalledWith("org-1", {
        period: "week",
        from: "2026-01-01",
        to: "2026-01-15",
      }),
    );
  });

  it("resets usage filters back to default query values", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Usage"));
    const usageSection = await screen.findByTestId("org-usage-section");

    await user.selectOptions(within(usageSection).getByLabelText("Period"), "week");
    await user.type(within(usageSection).getByLabelText("From"), "2026-02-01");
    await user.type(within(usageSection).getByLabelText("To"), "2026-02-10");
    await user.click(within(usageSection).getByRole("button", { name: "Apply Filters" }));
    await waitFor(() =>
      expect(mockFetchOrgUsage).toHaveBeenLastCalledWith("org-1", {
        period: "week",
        from: "2026-02-01",
        to: "2026-02-10",
      }),
    );

    const refreshedUsageSection = await screen.findByTestId("org-usage-section");
    await user.click(within(refreshedUsageSection).getByRole("button", { name: "Reset" }));
    await waitFor(() =>
      expect(mockFetchOrgUsage).toHaveBeenLastCalledWith("org-1", {
        period: "month",
        from: null,
        to: null,
      }),
    );
  });

  it("switches to Audit tab and shows audit events", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.click(screen.getByText("Audit"));
    await expect(screen.findByText("org.created")).resolves.toBeInTheDocument();
    expect(screen.getByText("success")).toBeInTheDocument();
  });

  it("serializes audit filter params into fetchOrgAudit queries", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Audit"));
    await screen.findByTestId("org-audit-section");

    await user.type(screen.getByLabelText("From"), "2026-01-01");
    await user.type(screen.getByLabelText("To"), "2026-01-15");
    await user.type(screen.getByLabelText("Action"), "org.updated");
    await user.type(screen.getByLabelText("Result"), "success");
    await user.type(screen.getByLabelText("Actor ID"), "u-9");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() =>
      expect(mockFetchOrgAudit).toHaveBeenLastCalledWith(
        "org-1",
        expect.objectContaining({
          limit: 50,
          offset: 0,
          from: "2026-01-01",
          to: "2026-01-15",
          action: "org.updated",
          result: "success",
          actorId: "u-9",
        }),
      ),
    );
  });

  it("resets audit filters back to default query values", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Audit"));
    const auditSection = await screen.findByTestId("org-audit-section");

    await user.type(within(auditSection).getByLabelText("From"), "2026-02-01");
    await user.type(within(auditSection).getByLabelText("To"), "2026-02-10");
    await user.type(within(auditSection).getByLabelText("Action"), "org.deleted");
    await user.type(within(auditSection).getByLabelText("Result"), "failure");
    await user.type(within(auditSection).getByLabelText("Actor ID"), "u-44");
    await user.click(within(auditSection).getByRole("button", { name: "Apply Filters" }));
    await waitFor(() =>
      expect(mockFetchOrgAudit).toHaveBeenLastCalledWith(
        "org-1",
        expect.objectContaining({
          limit: 50,
          offset: 0,
          from: "2026-02-01",
          to: "2026-02-10",
          action: "org.deleted",
          result: "failure",
          actorId: "u-44",
        }),
      ),
    );

    const refreshedAuditSection = await screen.findByTestId("org-audit-section");
    await user.click(within(refreshedAuditSection).getByRole("button", { name: "Reset" }));
    await waitFor(() =>
      expect(mockFetchOrgAudit).toHaveBeenLastCalledWith("org-1", {
        limit: 50,
        offset: 0,
        from: undefined,
        to: undefined,
        action: undefined,
        result: undefined,
        actorId: undefined,
      }),
    );
  });

  it("deletes org with confirm=true", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(mockDeleteOrg).toHaveBeenCalledWith("org-1"));
  });

  it("surfaces delete failures without clearing the selected org", async () => {
    mockDeleteOrg.mockRejectedValueOnce(new Error("org still has assigned tenants"));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await screen.findByTestId("org-info-section");

    await user.click(screen.getByRole("button", { name: "Delete" }));

    await expect(screen.findByText(/org still has assigned tenants/i)).resolves.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Acme Inc" })).toBeInTheDocument();
  });

  it("surfaces last-owner error on member role update", async () => {
    mockUpdateOrgMemberRole.mockRejectedValueOnce(new Error("cannot demote last owner"));
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Members"));
    const memberRow = await screen.findByTestId("org-member-row-u-1");

    await user.selectOptions(within(memberRow).getByLabelText("Role for u-1"), "member");
    await user.click(within(memberRow).getByRole("button", { name: "Update Role" }));

    await expect(screen.findByText(/cannot demote last owner/i)).resolves.toBeInTheDocument();
  });
});

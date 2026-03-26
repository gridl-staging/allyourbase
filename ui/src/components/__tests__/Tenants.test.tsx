import { describe, expect, it } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  CREATE_TENANT_ISOLATION_MODE_OPTIONS,
  CREATE_TENANT_PLAN_TIER_OPTIONS,
} from "../../types/tenants";
import type { Tenant, TenantMembership } from "../../types/tenants";
import {
  CREATE_TENANT_CANONICAL_SELECTIONS,
  CREATE_TENANT_DIALOG_DEFAULTS,
  getCreateTenantDialogControls,
  makeApiError,
  makeBreakerState,
  makeExpectedCreateTenantPayload,
  makeListResponse,
  makeTenant,
  mockAddTenantMember,
  mockCreateTenant,
  mockDeleteTenant,
  mockEnableMaintenance,
  mockFetchBreakerState,
  mockFetchTenantAudit,
  mockFetchTenantList,
  mockFetchTenantMembers,
  mockGetTenant,
  mockListUsersSearchResults,
  openCreateTenantDialog,
  mockRemoveTenantMember,
  mockSuspendTenant,
  mockUpdateTenant,
  mockUpdateTenantMemberRole,
} from "./tenants-test-helpers";
import { renderWithProviders } from "../../test-utils";
import { Tenants } from "../Tenants";

describe("Tenants", () => {
  it("renders loading state while tenant list is pending", () => {
    mockFetchTenantList.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<Tenants />);
    expect(screen.getByText(/loading tenants/i)).toBeInTheDocument();
  });

  it("renders tenant list with names and state badges after load", async () => {
    renderWithProviders(<Tenants />);
    await expect(screen.findByText("Acme")).resolves.toBeInTheDocument();
    expect(screen.getByText("Beta Corp")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
    expect(screen.getByText("suspended")).toBeInTheDocument();
  });

  it("renders empty state when no tenants exist", async () => {
    mockFetchTenantList.mockResolvedValueOnce(makeListResponse({ items: [], totalItems: 0, totalPages: 0 }));
    renderWithProviders(<Tenants />);
    await expect(screen.findByText(/no tenants found/i)).resolves.toBeInTheDocument();
  });

  it("renders error state when list fetch fails", async () => {
    mockFetchTenantList.mockRejectedValueOnce(new Error("server error"));
    renderWithProviders(<Tenants />);
    await expect(screen.findByText(/failed to load tenants/i)).resolves.toBeInTheDocument();
  });

  it("renders pagination showing page info", async () => {
    mockFetchTenantList.mockResolvedValueOnce(makeListResponse({ page: 1, totalPages: 3 }));
    renderWithProviders(<Tenants />);
    await expect(screen.findByText("Page 1 of 3")).resolves.toBeInTheDocument();
  });

  it("selects tenant and loads detail panel with info tab", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    const infoSection = await screen.findByTestId("tenant-info-section");
    expect(within(infoSection).getByText("acme")).toBeInTheDocument();
    expect(within(infoSection).getByText("shared")).toBeInTheDocument();
    expect(within(infoSection).getByText("pro")).toBeInTheDocument();
  });

  it("clears stale tenant detail while a newly selected tenant is loading", async () => {
    let resolveTenant: ((tenant: Tenant) => void) | null = null;

    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await screen.findByRole("heading", { name: "Acme" });

    mockGetTenant.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveTenant = resolve;
      }),
    );

    await user.click(screen.getByText("Beta Corp"));

    await expect(screen.findByText("Loading tenant details…")).resolves.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Acme" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Suspend" })).not.toBeInTheDocument();

    resolveTenant?.(makeTenant({ id: "t-2", name: "Beta Corp", slug: "beta-corp", state: "suspended" }));
    await expect(screen.findByRole("heading", { name: "Beta Corp" })).resolves.toBeInTheDocument();
  });

  it("updates tenant info from the info panel", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    await user.click(screen.getByText("Acme"));
    await screen.findByTestId("tenant-info-section");
    await user.clear(screen.getByLabelText("Tenant Name"));
    await user.type(screen.getByLabelText("Tenant Name"), "Acme Updated");
    fireEvent.change(screen.getByLabelText("Org Metadata"), { target: { value: "{\"tier\":\"gold\"}" } });
    await user.click(screen.getByRole("button", { name: "Save Info" }));

    await waitFor(() =>
      expect(mockUpdateTenant).toHaveBeenCalledWith("t-1", {
        name: "Acme Updated",
        orgMetadata: { tier: "gold" },
      }),
    );
    expect(screen.getByRole("heading", { name: "Acme Updated" })).toBeInTheDocument();
  });

  it("shows Suspend button for active tenant and Resume for suspended tenant", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    // Select active tenant — should show Suspend
    await user.click(screen.getByText("Acme"));
    await expect(screen.findByRole("button", { name: "Suspend" })).resolves.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Resume" })).not.toBeInTheDocument();

    // Select suspended tenant — should show Resume
    mockGetTenant.mockResolvedValueOnce(makeTenant({ id: "t-2", state: "suspended", name: "Beta Corp" }));
    await user.click(screen.getByText("Beta Corp"));
    await expect(screen.findByRole("button", { name: "Resume" })).resolves.toBeInTheDocument();
  });

  it("executes suspend action and refreshes tenant state", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await screen.findByRole("button", { name: "Suspend" });
    await user.click(screen.getByRole("button", { name: "Suspend" }));
    expect(mockSuspendTenant).toHaveBeenCalledWith("t-1");
  });

  it("keeps the deleted tenant selected with its deleting lifecycle state", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await screen.findByRole("button", { name: "Delete" });

    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(mockDeleteTenant).toHaveBeenCalledWith("t-1");
      expect(screen.getByRole("heading", { name: "Acme" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Suspend" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Resume" })).not.toBeInTheDocument();
    });
  });

  it("switches to Members tab and shows member list", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await screen.findByRole("button", { name: "Suspend" });
    await user.click(screen.getByText("Members"));
    await expect(screen.findByText("u-1")).resolves.toBeInTheDocument();
    expect(screen.getByTestId("tenant-member-role-u-1")).toHaveTextContent("owner");
    expect(screen.getByText("u-2")).toBeInTheDocument();
    expect(screen.getByTestId("tenant-member-role-u-2")).toHaveTextContent("admin");
  });

  it("adds a member from the members panel", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    await user.click(screen.getByText("Acme"));
    await user.click(screen.getByText("Members"));
    await screen.findByTestId("tenant-members-section");

    await user.type(screen.getByLabelText("New Member User ID"), "u-3");
    await user.selectOptions(screen.getByLabelText("New Member Role"), "viewer");
    await user.click(screen.getByRole("button", { name: "Add Member" }));

    await waitFor(() => expect(mockAddTenantMember).toHaveBeenCalledWith("t-1", "u-3", "viewer"));
    expect(screen.getByText("u-3")).toBeInTheDocument();
    expect(screen.getByTestId("tenant-member-role-u-3")).toHaveTextContent("viewer");
  });

  it("switches to Maintenance tab and shows breaker status", async () => {
    mockFetchBreakerState.mockResolvedValueOnce(makeBreakerState({ state: "open", consecutiveFailures: 5 }));
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await screen.findByRole("button", { name: "Suspend" });
    await user.click(screen.getByText("Maintenance"));
    const section = await screen.findByTestId("tenant-maintenance-section");
    expect(within(section).getByText(/State:.*open/)).toBeInTheDocument();
    expect(within(section).getByText(/Consecutive Failures:.*5/)).toBeInTheDocument();
  });

  it("toggles maintenance on via Maintenance tab", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await screen.findByRole("button", { name: "Suspend" });
    await user.click(screen.getByText("Maintenance"));
    await screen.findByRole("button", { name: "Enable Maintenance" });
    await user.click(screen.getByRole("button", { name: "Enable Maintenance" }));
    expect(mockEnableMaintenance).toHaveBeenCalledWith("t-1", expect.any(String));
  });

  it("switches to Audit tab and shows audit events", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await screen.findByRole("button", { name: "Suspend" });
    await user.click(screen.getByText("Audit"));
    await expect(screen.findByText("tenant.created")).resolves.toBeInTheDocument();
    expect(screen.getByText("success")).toBeInTheDocument();
  });

  it("validates required create-tenant fields and slug format before submit", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    await user.click(screen.getByRole("button", { name: "Create Tenant" }));
    await user.click(screen.getByRole("button", { name: "Create" }));

    await expect(screen.findByText("Tenant name is required")).resolves.toBeInTheDocument();
    expect(screen.getByText("Slug is required")).toBeInTheDocument();
    expect(mockCreateTenant).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText("Tenant Name"), "Gamma");
    await user.type(screen.getByLabelText("Slug"), "Invalid Slug!");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await expect(
      screen.findByText("Slug must match: lowercase letters, numbers, and hyphens"),
    ).resolves.toBeInTheDocument();
    expect(mockCreateTenant).not.toHaveBeenCalled();
  });

  it("allows ownerless create and trims typed payload fields before submit", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    const { nameInput, slugInput, ownerUserIdInput, isolationModeSelect, planTierSelect, regionInput, createButton } =
      await openCreateTenantDialog(user);
    expect(ownerUserIdInput).toHaveAttribute("aria-autocomplete", "list");
    expect(isolationModeSelect.tagName).toBe("SELECT");
    expect(planTierSelect.tagName).toBe("SELECT");
    expect(regionInput.tagName).toBe("INPUT");

    await user.type(nameInput, "  Gamma Corp  ");
    await user.type(slugInput, "  gamma-corp  ");
    await user.clear(regionInput);
    await user.type(regionInput, "  us-west-2  ");
    await user.click(createButton);

    await waitFor(() =>
      expect(mockCreateTenant).toHaveBeenCalledWith(
        makeExpectedCreateTenantPayload({
          name: "Gamma Corp",
          slug: "gamma-corp",
          region: "us-west-2",
        }),
      ),
    );
    expect(screen.queryByText("Owner user ID is required")).not.toBeInTheDocument();
  });

  it("submits a raw owner UUID exactly as typed without requiring combobox selection", async () => {
    const rawOwnerUserId = "34d15a79-4950-4f30-a9c7-a8510d44f9ee";
    mockListUsersSearchResults();
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    const { nameInput, slugInput, ownerUserIdInput, createButton } = await openCreateTenantDialog(user);
    await user.type(nameInput, "Raw Owner Tenant");
    await user.type(slugInput, "raw-owner-tenant");
    await user.type(ownerUserIdInput, `  ${rawOwnerUserId}  `);
    await user.click(createButton);

    await waitFor(() =>
      expect(mockCreateTenant).toHaveBeenCalledWith(
        makeExpectedCreateTenantPayload({
          name: "Raw Owner Tenant",
          slug: "raw-owner-tenant",
          ownerUserId: rawOwnerUserId,
        }),
      ),
    );
  });

  it("renders isolation and plan options from shared constants and keeps region free text", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    const { ownerUserIdInput, isolationModeSelect, planTierSelect, regionInput } = await openCreateTenantDialog(user);

    expect(screen.getByText("Owner User ID (optional)")).toBeInTheDocument();
    expect(ownerUserIdInput).toHaveAttribute("placeholder", "Search by email or paste a user ID");
    expect(isolationModeSelect.tagName).toBe("SELECT");
    expect(planTierSelect.tagName).toBe("SELECT");
    expect(regionInput.tagName).toBe("INPUT");

    expect(
      within(isolationModeSelect).getAllByRole("option").map((option) => (option as HTMLOptionElement).value),
    ).toEqual(CREATE_TENANT_ISOLATION_MODE_OPTIONS.map((option) => option.value));

    expect(
      within(planTierSelect).getAllByRole("option").map((option) => (option as HTMLOptionElement).value),
    ).toEqual(CREATE_TENANT_PLAN_TIER_OPTIONS.map((option) => option.value));
  });

  it("clears create submit errors on edit, closes on success, and reopens with default values", async () => {
    mockCreateTenant.mockRejectedValueOnce(makeApiError(409, "tenant slug already exists"));
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    let { nameInput, slugInput, ownerUserIdInput, isolationModeSelect, planTierSelect, regionInput } =
      await openCreateTenantDialog(user);
    const { createButton } = getCreateTenantDialogControls();
    await user.type(nameInput, "Gamma");
    await user.type(slugInput, "gamma");

    await user.click(createButton);
    await expect(screen.findByText("A tenant with this slug already exists.")).resolves.toBeInTheDocument();

    await user.type(nameInput, " LLC");
    await waitFor(() =>
      expect(screen.queryByText("A tenant with this slug already exists.")).not.toBeInTheDocument(),
    );

    await user.click(createButton);

    await waitFor(() => expect(mockCreateTenant).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Create" })).not.toBeInTheDocument());
    expect(mockCreateTenant).toHaveBeenLastCalledWith(
      makeExpectedCreateTenantPayload({
        name: "Gamma LLC",
        slug: "gamma",
      }),
    );

    await openCreateTenantDialog(user);
    ({ nameInput, slugInput, ownerUserIdInput, isolationModeSelect, planTierSelect, regionInput } =
      getCreateTenantDialogControls());
    expect(nameInput).toHaveValue(CREATE_TENANT_DIALOG_DEFAULTS.name);
    expect(slugInput).toHaveValue(CREATE_TENANT_DIALOG_DEFAULTS.slug);
    expect(ownerUserIdInput).toHaveValue(CREATE_TENANT_DIALOG_DEFAULTS.ownerUserId);
    expect(isolationModeSelect).toHaveValue(CREATE_TENANT_DIALOG_DEFAULTS.isolationMode);
    expect(planTierSelect).toHaveValue(CREATE_TENANT_DIALOG_DEFAULTS.planTier);
    expect(regionInput).toHaveValue(CREATE_TENANT_DIALOG_DEFAULTS.region);
  });

  it("submits canonical isolation mode and plan tier values selected from shared options", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    const selectedIsolationMode = CREATE_TENANT_CANONICAL_SELECTIONS.isolationMode;
    const selectedPlanTier = CREATE_TENANT_CANONICAL_SELECTIONS.planTier;

    const { nameInput, slugInput, isolationModeSelect, planTierSelect, createButton } = await openCreateTenantDialog(
      user,
    );
    await user.type(nameInput, "Canonical Tenant");
    await user.type(slugInput, "canonical-tenant");
    await user.selectOptions(isolationModeSelect, selectedIsolationMode);
    await user.selectOptions(planTierSelect, selectedPlanTier);
    await user.click(createButton);

    await waitFor(() =>
      expect(mockCreateTenant).toHaveBeenCalledWith(
        makeExpectedCreateTenantPayload({
          name: "Canonical Tenant",
          slug: "canonical-tenant",
          isolationMode: selectedIsolationMode,
          planTier: selectedPlanTier,
        }),
      ),
    );
  });

  it("surfaces duplicate slug errors from create tenant endpoint", async () => {
    mockCreateTenant.mockRejectedValueOnce(makeApiError(409, "tenant slug already exists"));
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    const { nameInput, slugInput, ownerUserIdInput, createButton } = await openCreateTenantDialog(user);
    await user.type(nameInput, "Gamma");
    await user.type(slugInput, "gamma");
    await user.type(ownerUserIdInput, "u-3");
    await user.click(createButton);

    await expect(screen.findByText("A tenant with this slug already exists.")).resolves.toBeInTheDocument();
    expect(mockCreateTenant).toHaveBeenCalledWith(
      makeExpectedCreateTenantPayload({ name: "Gamma", slug: "gamma", ownerUserId: "u-3" }),
    );
  });

  it("redacts unexpected create tenant backend errors from the dialog", async () => {
    mockCreateTenant.mockRejectedValueOnce(
      makeApiError(500, "pq: duplicate key value violates unique constraint \"tenants_slug_key\""),
    );
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    const { nameInput, slugInput, createButton } = await openCreateTenantDialog(user);
    await user.type(nameInput, "Gamma");
    await user.type(slugInput, "gamma");
    await user.click(createButton);

    await expect(
      screen.findByText("Unable to create tenant. Verify the values and try again."),
    ).resolves.toBeInTheDocument();
    expect(screen.queryByText(/duplicate key value/i)).not.toBeInTheDocument();
  });

  it("optimistically refreshes member role while update call is in-flight", async () => {
    let resolveUpdate: ((value: TenantMembership) => void) | null = null;
    mockUpdateTenantMemberRole.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveUpdate = resolve;
      }),
    );

    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await user.click(screen.getByText("Members"));

    const membersSection = await screen.findByTestId("tenant-members-section");
    const memberRow = within(membersSection).getByTestId("tenant-member-row-u-2");
    expect(within(memberRow).getByTestId("tenant-member-role-u-2")).toHaveTextContent("admin");

    await user.selectOptions(within(memberRow).getByLabelText("Role for u-2"), "viewer");
    await user.click(within(memberRow).getByRole("button", { name: "Update Role" }));

    expect(within(memberRow).getByTestId("tenant-member-role-u-2")).toHaveTextContent("viewer");
    expect(mockUpdateTenantMemberRole).toHaveBeenCalledWith("t-1", "u-2", "viewer");

    resolveUpdate?.({
      id: "m-2",
      tenantId: "t-1",
      userId: "u-2",
      role: "viewer",
      createdAt: "2026-01-02T00:00:00Z",
    });
    await expect(within(memberRow).findByTestId("tenant-member-role-u-2")).resolves.toHaveTextContent(
      "viewer",
    );
  });

  it("clears unsaved member-role drafts when switching to another tenant", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    await user.click(screen.getByText("Acme"));
    await user.click(screen.getByText("Members"));
    const acmeRow = await screen.findByTestId("tenant-member-row-u-2");
    await user.selectOptions(within(acmeRow).getByLabelText("Role for u-2"), "viewer");
    expect(within(acmeRow).getByLabelText("Role for u-2")).toHaveValue("viewer");

    mockGetTenant.mockResolvedValueOnce(makeTenant({ id: "t-2", name: "Beta Corp", slug: "beta-corp" }));
    mockFetchTenantMembers.mockResolvedValueOnce({
      items: [
        {
          id: "m-9",
          tenantId: "t-2",
          userId: "u-2",
          role: "owner",
          createdAt: "2026-01-03T00:00:00Z",
        },
      ],
    });

    await user.click(screen.getByText("Beta Corp"));
    await user.click(screen.getByText("Members"));

    const betaRow = await screen.findByTestId("tenant-member-row-u-2");
    expect(within(betaRow).getByTestId("tenant-member-role-u-2")).toHaveTextContent("owner");
    expect(within(betaRow).getByLabelText("Role for u-2")).toHaveValue("owner");
  });

  it("removes a member from the members panel", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");

    await user.click(screen.getByText("Acme"));
    await user.click(screen.getByText("Members"));
    await screen.findByTestId("tenant-member-row-u-2");

    await user.click(within(screen.getByTestId("tenant-member-row-u-2")).getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(mockRemoveTenantMember).toHaveBeenCalledWith("t-1", "u-2"));
    expect(screen.queryByTestId("tenant-member-row-u-2")).not.toBeInTheDocument();
  });

  it("serializes audit filter params into fetchTenantAudit queries", async () => {
    renderWithProviders(<Tenants />);
    const user = userEvent.setup();
    await screen.findByText("Acme");
    await user.click(screen.getByText("Acme"));
    await user.click(screen.getByText("Audit"));
    await screen.findByTestId("tenant-audit-section");

    await user.type(screen.getByLabelText("From"), "2026-01-01");
    await user.type(screen.getByLabelText("To"), "2026-01-15");
    await user.type(screen.getByLabelText("Action"), "tenant.suspended");
    await user.type(screen.getByLabelText("Result"), "success");
    await user.type(screen.getByLabelText("Actor ID"), "u-9");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() =>
      expect(mockFetchTenantAudit).toHaveBeenLastCalledWith(
        "t-1",
        expect.objectContaining({
          limit: 50,
          offset: 0,
          from: "2026-01-01",
          to: "2026-01-15",
          action: "tenant.suspended",
          result: "success",
          actorId: "u-9",
        }),
      ),
    );
  });
});

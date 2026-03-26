import { vi, describe, it, expect, beforeEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import userEvent from "@testing-library/user-event";
import { RlsPolicies } from "../RlsPolicies";
import {
  listRlsPolicies,
  getRlsStatus,
  createRlsPolicy,
  deleteRlsPolicy,
  enableRls,
  disableRls,
} from "../../api";
import type { SchemaCache, RlsTableStatus } from "../../types";
import { makePolicy } from "./rls-test-fixtures";

vi.mock("../../api", () => ({
  listRlsPolicies: vi.fn(),
  getRlsStatus: vi.fn(),
  createRlsPolicy: vi.fn(),
  deleteRlsPolicy: vi.fn(),
  enableRls: vi.fn(),
  disableRls: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

const mockListPolicies = vi.mocked(listRlsPolicies);
const mockGetStatus = vi.mocked(getRlsStatus);
const mockCreatePolicy = vi.mocked(createRlsPolicy);
const mockDeletePolicy = vi.mocked(deleteRlsPolicy);
const mockEnableRls = vi.mocked(enableRls);
const mockDisableRls = vi.mocked(disableRls);

function makeSchema(tableNames: string[] = ["posts", "comments"]): SchemaCache {
  const tables: SchemaCache["tables"] = {};
  for (const name of tableNames) {
    tables[`public.${name}`] = {
      schema: "public",
      name,
      kind: "table",
      columns: [
        { name: "id", position: 1, type: "uuid", nullable: false, isPrimaryKey: true, jsonType: "string" },
        { name: "user_id", position: 2, type: "uuid", nullable: false, isPrimaryKey: false, jsonType: "string" },
      ],
      primaryKey: ["id"],
    };
  }
  return { tables, schemas: ["public"], builtAt: "2026-02-10T12:00:00Z" };
}

function makeStatus(overrides: Partial<RlsTableStatus> = {}): RlsTableStatus {
  return { rlsEnabled: true, forceRls: false, ...overrides };
}

describe("RlsPolicies", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading state", () => {
    mockListPolicies.mockReturnValue(new Promise(() => {}));
    mockGetStatus.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);
    expect(screen.getByText("Loading policies...")).toBeInTheDocument();
  });

  it("shows table list in sidebar", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts", "comments"])} />);

    await waitFor(() => {
      expect(screen.getByText("posts")).toBeInTheDocument();
      expect(screen.getByText("comments")).toBeInTheDocument();
    });
  });

  it("shows RLS enabled badge when RLS is on", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus({ rlsEnabled: true }));
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("RLS Enabled")).toBeInTheDocument();
    });
  });

  it("shows RLS disabled badge when RLS is off", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus({ rlsEnabled: false }));
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("RLS Disabled")).toBeInTheDocument();
    });
  });

  it("renders policy list with details", async () => {
    const policies = [
      makePolicy({ policyName: "owner_access", command: "ALL" }),
      makePolicy({ policyName: "public_read", command: "SELECT", roles: ["PUBLIC"] }),
    ];
    mockListPolicies.mockResolvedValueOnce(policies);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
      expect(screen.getByText("public_read")).toBeInTheDocument();
      expect(screen.getByText("ALL")).toBeInTheDocument();
      expect(screen.getByText("SELECT")).toBeInTheDocument();
    });
  });

  it("shows empty state when no policies", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
      expect(screen.getByText("Create your first policy")).toBeInTheDocument();
    });
  });

  it("shows error state with retry", async () => {
    mockListPolicies.mockRejectedValueOnce(new Error("connection refused"));
    mockGetStatus.mockRejectedValueOnce(new Error("connection refused"));
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("opens create policy modal on Add Policy click", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));

    expect(screen.getByText("Create RLS Policy")).toBeInTheDocument();
    expect(screen.getByLabelText("Policy name")).toBeInTheDocument();
    expect(screen.getByLabelText("Command")).toBeInTheDocument();
  });

  it("create policy closes and resets modal, then refreshes policy data for selected table", async () => {
    const createdPolicy = makePolicy({
      tableName: "comments",
      policyName: "test_policy",
      command: "SELECT",
      usingExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
      withCheckExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
    });
    let commentsFetchCount = 0;
    mockListPolicies.mockImplementation(async (tableName) => {
      if (tableName !== "public.comments") {
        return [];
      }
      commentsFetchCount += 1;
      if (commentsFetchCount >= 2) {
        return [createdPolicy];
      }
      return [];
    });
    mockGetStatus.mockResolvedValue(makeStatus());
    mockCreatePolicy.mockResolvedValueOnce({ message: "policy created" });
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts", "comments"])} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "comments" }));
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "comments" })).toBeInTheDocument();
    });

    await user.click(screen.getByText("Add Policy"));

    await user.type(screen.getByLabelText("Policy name"), "test_policy");
    await user.selectOptions(screen.getByLabelText("Command"), "SELECT");
    await user.type(
      screen.getByLabelText("USING expression"),
      "(user_id = current_setting('ayb.user_id', true)::uuid)",
    );
    await user.type(
      screen.getByLabelText("WITH CHECK expression"),
      "(user_id = current_setting('ayb.user_id', true)::uuid)",
    );

    await user.click(screen.getByText("Create Policy"));

    await waitFor(() => {
      expect(mockCreatePolicy).toHaveBeenCalledWith(
        expect.objectContaining({
          table: "comments",
          name: "test_policy",
          command: "SELECT",
          using: "(user_id = current_setting('ayb.user_id', true)::uuid)",
          withCheck: "(user_id = current_setting('ayb.user_id', true)::uuid)",
        }),
      );
    });

    await waitFor(() => {
      expect(screen.queryByText("Create RLS Policy")).not.toBeInTheDocument();
      const createdPolicyCard = screen.getByText("test_policy").closest("div.border.rounded-lg");
      expect(createdPolicyCard).not.toBeNull();
      const createdPolicyDetails = within(createdPolicyCard as HTMLElement);
      expect(createdPolicyDetails.getByText("SELECT")).toBeInTheDocument();
      expect(
        createdPolicyDetails.getAllByText("(user_id = current_setting('ayb.user_id', true)::uuid)"),
      ).toHaveLength(2);
    });

    await user.click(screen.getByText("Add Policy"));
    const policyNameInput = screen.getByLabelText("Policy name") as HTMLInputElement;
    const usingExpressionInput = screen.getByLabelText("USING expression") as HTMLTextAreaElement;
    const withCheckExpressionInput = screen.getByLabelText("WITH CHECK expression") as HTMLTextAreaElement;
    expect(policyNameInput.value).toBe("");
    expect(usingExpressionInput.value).toBe("");
    expect(withCheckExpressionInput.value).toBe("");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      const listPolicyCallsForComments = mockListPolicies.mock.calls.filter(
        ([table]) => table === "public.comments",
      );
      const statusCallsForComments = mockGetStatus.mock.calls.filter(
        ([table]) => table === "public.comments",
      );
      expect(listPolicyCallsForComments.length).toBeGreaterThanOrEqual(2);
      expect(statusCallsForComments.length).toBeGreaterThanOrEqual(2);
    });
  });

  it("create policy rejection shows toast, keeps modal open, and resets submitting state", async () => {
    mockListPolicies.mockResolvedValue([]);
    mockGetStatus.mockResolvedValue(makeStatus());
    mockCreatePolicy.mockRejectedValueOnce(new Error("create failed"));
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));
    await user.type(screen.getByLabelText("Policy name"), "test_policy");

    const createButton = screen.getByRole("button", { name: "Create Policy" });
    await user.click(createButton);

    await waitFor(() => {
      expect(mockCreatePolicy).toHaveBeenCalledWith(
        expect.objectContaining({
          table: "posts",
          name: "test_policy",
        }),
      );
    });

    await waitFor(() => {
      expect(screen.getByText("create failed")).toBeInTheDocument();
      expect(screen.getByText("Create RLS Policy")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Create Policy" })).toBeEnabled();
    });
  });

  it("shows policy templates in create modal", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));

    expect(screen.getByText("Owner only")).toBeInTheDocument();
    expect(screen.getByText("Public read, owner write")).toBeInTheDocument();
    expect(screen.getByText("Role-based access")).toBeInTheDocument();
    expect(screen.getByText("Tenant isolation")).toBeInTheDocument();
  });

  it("template and placeholder auth-context copy uses ayb session keys", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));
    await user.click(screen.getByText("Owner only"));

    const usingInput = screen.getByLabelText("USING expression") as HTMLTextAreaElement;
    const withCheckInput = screen.getByLabelText("WITH CHECK expression") as HTMLTextAreaElement;
    expect(usingInput).toHaveAttribute("placeholder", expect.stringContaining("ayb.user_id"));
    expect(withCheckInput).toHaveAttribute("placeholder", expect.stringContaining("ayb.user_id"));
    expect(usingInput.value).toContain("ayb.user_id");
    expect(withCheckInput.value).toContain("ayb.user_id");

    await user.click(screen.getByText("Role-based access"));
    expect(usingInput.value).toContain("ayb.user_role");
    expect(withCheckInput.value).toContain("ayb.user_role");

    await user.click(screen.getByText("Tenant isolation"));
    expect(usingInput.value).toContain("ayb.tenant_id");
    expect(withCheckInput.value).toContain("ayb.tenant_id");

    const commandSelect = screen.getByLabelText("Command") as HTMLSelectElement;
    expect(commandSelect.value).toBe("ALL");
  });

  it("opens delete confirmation when delete button clicked", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy()]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete policy"));

    expect(screen.getByText("Delete Policy")).toBeInTheDocument();
    expect(screen.getByText(/permanently drop the policy/)).toBeInTheDocument();
  });

  it("confirming delete calls deleteRlsPolicy", async () => {
    mockListPolicies.mockResolvedValue([makePolicy()]);
    mockGetStatus.mockResolvedValue(makeStatus());
    mockDeletePolicy.mockResolvedValueOnce(undefined);
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete policy"));

    const dialog = screen
      .getByText("Delete Policy")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Delete" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockDeletePolicy).toHaveBeenCalledWith("public.posts", "owner_access");
    });
  });

  it("delete rejection shows toast, keeps dialog open, and re-enables delete action", async () => {
    mockListPolicies.mockResolvedValue([makePolicy()]);
    mockGetStatus.mockResolvedValue(makeStatus());
    mockDeletePolicy.mockRejectedValueOnce(new Error("delete failed"));
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete policy"));

    const dialog = screen.getByText("Delete Policy").closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Delete" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockDeletePolicy).toHaveBeenCalledWith("public.posts", "owner_access");
      expect(screen.getByText("delete failed")).toBeInTheDocument();
      expect(screen.getByText("Delete Policy")).toBeInTheDocument();
      expect(within(dialog).getByRole("button", { name: "Delete" })).toBeEnabled();
    });
  });

  it("cancel on delete dialog closes it", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy()]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete policy"));
    expect(screen.getByText("Delete Policy")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Delete Policy")).not.toBeInTheDocument();
  });

  it("toggle RLS calls enableRls when disabled and refetches policies and status", async () => {
    mockListPolicies
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([
        makePolicy({
          policyName: "after_enable",
          command: "SELECT",
          usingExpr: "(tenant_id = current_setting('ayb.tenant_id', true)::uuid)",
        }),
      ]);
    mockGetStatus
      .mockResolvedValueOnce(makeStatus({ rlsEnabled: false }))
      .mockResolvedValueOnce(makeStatus({ rlsEnabled: true }));
    mockEnableRls.mockResolvedValueOnce({ message: "enabled" });
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Enable RLS")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Enable RLS"));

    await waitFor(() => {
      expect(mockEnableRls).toHaveBeenCalledWith("public.posts");
      expect(screen.getByText("RLS Enabled")).toBeInTheDocument();
      expect(screen.getByText("after_enable")).toBeInTheDocument();
      const listPolicyCallsForPosts = mockListPolicies.mock.calls.filter(
        ([table]) => table === "public.posts",
      );
      const statusCallsForPosts = mockGetStatus.mock.calls.filter(
        ([table]) => table === "public.posts",
      );
      expect(listPolicyCallsForPosts.length).toBeGreaterThanOrEqual(2);
      expect(statusCallsForPosts.length).toBeGreaterThanOrEqual(2);
    });
  });

  it("toggle RLS calls disableRls when enabled", async () => {
    mockListPolicies.mockResolvedValue([]);
    mockGetStatus.mockResolvedValue(makeStatus({ rlsEnabled: true }));
    mockDisableRls.mockResolvedValueOnce({ message: "disabled" });
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Disable RLS")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Disable RLS"));

    await waitFor(() => {
      expect(mockDisableRls).toHaveBeenCalledWith("public.posts");
    });
  });

  it("toggle disable rejection shows error toast and re-enables toggle button without success toast", async () => {
    mockListPolicies.mockResolvedValue([]);
    mockGetStatus.mockResolvedValue(makeStatus({ rlsEnabled: true }));
    mockDisableRls.mockRejectedValueOnce(new Error("toggle failed"));
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Disable RLS")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const toggleButton = screen.getByRole("button", { name: "Disable RLS" });
    await user.click(toggleButton);

    await waitFor(() => {
      expect(mockDisableRls).toHaveBeenCalledWith("public.posts");
      expect(screen.getByText("toggle failed")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Disable RLS" })).toBeEnabled();
    });
    expect(screen.queryByText("RLS disabled on posts")).not.toBeInTheDocument();
  });

  it("shows SQL preview when View SQL clicked", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy()]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("View SQL"));

    expect(screen.getByText("SQL Preview")).toBeInTheDocument();
    expect(screen.getByText(/CREATE POLICY "owner_access"/)).toBeInTheDocument();
  });

  it("shows USING and WITH CHECK expressions", async () => {
    mockListPolicies.mockResolvedValueOnce([
      makePolicy({
        usingExpr: "(user_id = 1)",
        withCheckExpr: "(tenant_id = 2)",
      }),
    ]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("(user_id = 1)")).toBeInTheDocument();
      expect(screen.getByText("(tenant_id = 2)")).toBeInTheDocument();
    });
  });

  it("shows roles for policy", async () => {
    mockListPolicies.mockResolvedValueOnce([
      makePolicy({ roles: ["authenticated", "admin"] }),
    ]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("Roles: authenticated, admin")).toBeInTheDocument();
    });
  });

  it("shows PERMISSIVE badge", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy({ permissive: "PERMISSIVE" })]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("PERMISSIVE")).toBeInTheDocument();
    });
  });

  it("shows RESTRICTIVE badge", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy({ permissive: "RESTRICTIVE" })]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("RESTRICTIVE")).toBeInTheDocument();
    });
  });

  it("filters tables to only show kind=table (no views)", async () => {
    const schema = makeSchema(["posts"]);
    schema.tables["public.my_view"] = {
      schema: "public",
      name: "my_view",
      kind: "view",
      columns: [],
      primaryKey: [],
    };
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={schema} />);

    await waitFor(() => {
      expect(screen.getByText("posts")).toBeInTheDocument();
    });
    // Views should not appear in the table list
    expect(screen.queryByText("my_view")).not.toBeInTheDocument();
  });

  it("retry button refetches data after error", async () => {
    mockListPolicies.mockRejectedValueOnce(new Error("db down"));
    mockGetStatus.mockRejectedValueOnce(new Error("db down"));
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("db down")).toBeInTheDocument();
    });

    // Retry should fetch again
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    const user = userEvent.setup();
    await user.click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
    });
  });

  it("create modal closes on cancel", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));
    expect(screen.getByText("Create RLS Policy")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Create RLS Policy")).not.toBeInTheDocument();
  });

  it("close SQL preview modal", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy()]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("View SQL"));
    expect(screen.getByText("SQL Preview")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByText("SQL Preview")).not.toBeInTheDocument();
  });

  it("SQL preview includes full CREATE POLICY statement", async () => {
    mockListPolicies.mockResolvedValueOnce([
      makePolicy({
        policyName: "owner_access",
        command: "ALL",
        permissive: "PERMISSIVE",
        roles: ["authenticated"],
        usingExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
        withCheckExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
      }),
    ]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("View SQL"));

    // Verify the SQL preview modal opened
    expect(screen.getByText("SQL Preview")).toBeInTheDocument();

    // Verify full SQL content including policy name, schema.table, command, roles, and expressions
    const preEl = screen.getByText(/CREATE POLICY "owner_access"/);
    expect(preEl).toBeInTheDocument();
    const sqlText = preEl.textContent ?? "";
    expect(sqlText).toContain('"public"."posts"');
    expect(sqlText).toContain("FOR ALL");
    expect(sqlText).toContain("authenticated");
    expect(sqlText).toContain("USING");
    expect(sqlText).toContain("WITH CHECK");
    expect(sqlText).toContain("current_setting");
  });

  it("shows no policies message with create button for empty table", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
      expect(screen.getByText("Create your first policy")).toBeInTheDocument();
    });
  });

  it("create your first policy button opens modal", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Create your first policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first policy"));
    expect(screen.getByText("Create RLS Policy")).toBeInTheDocument();
  });

  it("policy with null usingExpr shows no expression", async () => {
    mockListPolicies.mockResolvedValueOnce([
      makePolicy({ usingExpr: null, withCheckExpr: null }),
    ]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });
  });

  it("multiple policies render correctly", async () => {
    mockListPolicies.mockResolvedValueOnce([
      makePolicy({ policyName: "policy_1", command: "SELECT" }),
      makePolicy({ policyName: "policy_2", command: "INSERT" }),
      makePolicy({ policyName: "policy_3", command: "UPDATE" }),
    ]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("policy_1")).toBeInTheDocument();
      expect(screen.getByText("policy_2")).toBeInTheDocument();
      expect(screen.getByText("policy_3")).toBeInTheDocument();
      expect(screen.getByText("SELECT")).toBeInTheDocument();
      expect(screen.getByText("INSERT")).toBeInTheDocument();
      expect(screen.getByText("UPDATE")).toBeInTheDocument();
    });
  });

  it("create policy form has required fields", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));

    expect(screen.getByLabelText("Policy name")).toBeInTheDocument();
    expect(screen.getByLabelText("Command")).toBeInTheDocument();
    expect(screen.getByLabelText("USING expression")).toBeInTheDocument();
    expect(screen.getByLabelText("WITH CHECK expression")).toBeInTheDocument();
  });

  it("create policy sends schema for non-public tables", async () => {
    const schema = makeSchema([]);
    schema.tables["myapp.tasks"] = {
      schema: "myapp",
      name: "tasks",
      kind: "table",
      columns: [
        { name: "id", position: 1, type: "uuid", nullable: false, isPrimaryKey: true, jsonType: "string" },
      ],
      primaryKey: ["id"],
    };
    mockListPolicies.mockResolvedValue([]);
    mockGetStatus.mockResolvedValue(makeStatus());
    mockCreatePolicy.mockResolvedValueOnce({ message: "policy created" });
    renderWithProviders(<RlsPolicies schema={schema} />);

    await waitFor(() => {
      // Table should appear in sidebar
      expect(screen.getAllByText("tasks").length).toBeGreaterThanOrEqual(1);
    });

    const user = userEvent.setup();
    // Click on the first "tasks" element (sidebar button)
    await user.click(screen.getAllByText("tasks")[0]);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Add Policy"));
    await user.type(screen.getByLabelText("Policy name"), "test_policy");
    await user.click(screen.getByText("Create Policy"));

    await waitFor(() => {
      expect(mockCreatePolicy).toHaveBeenCalledWith(
        expect.objectContaining({
          table: "tasks",
          schema: "myapp",
        }),
      );
    });
  });
});

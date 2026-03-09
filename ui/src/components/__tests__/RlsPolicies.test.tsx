import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
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
import type { SchemaCache, RlsPolicy, RlsTableStatus } from "../../types";

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

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
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

function makePolicy(overrides: Partial<RlsPolicy> = {}): RlsPolicy {
  return {
    tableSchema: "public",
    tableName: "posts",
    policyName: "owner_access",
    command: "ALL",
    permissive: "PERMISSIVE",
    roles: ["authenticated"],
    usingExpr: "(user_id = current_setting('app.user_id')::uuid)",
    withCheckExpr: "(user_id = current_setting('app.user_id')::uuid)",
    ...overrides,
  };
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
    render(<RlsPolicies schema={makeSchema()} />);
    expect(screen.getByText("Loading policies...")).toBeInTheDocument();
  });

  it("shows table list in sidebar", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema(["posts", "comments"])} />);

    await waitFor(() => {
      expect(screen.getByText("posts")).toBeInTheDocument();
      expect(screen.getByText("comments")).toBeInTheDocument();
    });
  });

  it("shows RLS enabled badge when RLS is on", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus({ rlsEnabled: true }));
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("RLS Enabled")).toBeInTheDocument();
    });
  });

  it("shows RLS disabled badge when RLS is off", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus({ rlsEnabled: false }));
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
      expect(screen.getByText("Create your first policy")).toBeInTheDocument();
    });
  });

  it("shows error state with retry", async () => {
    mockListPolicies.mockRejectedValueOnce(new Error("connection refused"));
    mockGetStatus.mockRejectedValueOnce(new Error("connection refused"));
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("opens create policy modal on Add Policy click", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));

    expect(screen.getByText("Create RLS Policy")).toBeInTheDocument();
    expect(screen.getByLabelText("Policy name")).toBeInTheDocument();
    expect(screen.getByLabelText("Command")).toBeInTheDocument();
  });

  it("create policy submits and closes modal", async () => {
    mockListPolicies.mockResolvedValue([]);
    mockGetStatus.mockResolvedValue(makeStatus());
    mockCreatePolicy.mockResolvedValueOnce({ message: "policy created" });
    render(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));

    await user.type(screen.getByLabelText("Policy name"), "test_policy");
    await user.selectOptions(screen.getByLabelText("Command"), "SELECT");

    await user.click(screen.getByText("Create Policy"));

    await waitFor(() => {
      expect(mockCreatePolicy).toHaveBeenCalledWith(
        expect.objectContaining({
          table: "posts",
          name: "test_policy",
          command: "SELECT",
        }),
      );
    });
  });

  it("shows policy templates in create modal", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

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

  it("template populates form fields", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Add Policy"));
    await user.click(screen.getByText("Owner only"));

    // Verify USING expression was populated
    const usingInput = screen.getByLabelText("USING expression") as HTMLTextAreaElement;
    expect(usingInput.value).toContain("current_setting");
    expect(usingInput.value).toContain("user_id");

    // Verify WITH CHECK expression was also populated
    const withCheckInput = screen.getByLabelText("WITH CHECK expression") as HTMLTextAreaElement;
    expect(withCheckInput.value).toContain("current_setting");

    // Verify command was set to ALL
    const commandSelect = screen.getByLabelText("Command") as HTMLSelectElement;
    expect(commandSelect.value).toBe("ALL");
  });

  it("opens delete confirmation when delete button clicked", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy()]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
      expect(mockDeletePolicy).toHaveBeenCalledWith("posts", "owner_access");
    });
  });

  it("cancel on delete dialog closes it", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy()]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("owner_access")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete policy"));
    expect(screen.getByText("Delete Policy")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Delete Policy")).not.toBeInTheDocument();
  });

  it("toggle RLS calls enableRls when disabled", async () => {
    mockListPolicies.mockResolvedValue([]);
    mockGetStatus.mockResolvedValue(makeStatus({ rlsEnabled: false }));
    mockEnableRls.mockResolvedValueOnce({ message: "enabled" });
    render(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Enable RLS")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Enable RLS"));

    await waitFor(() => {
      expect(mockEnableRls).toHaveBeenCalledWith("posts");
    });
  });

  it("toggle RLS calls disableRls when enabled", async () => {
    mockListPolicies.mockResolvedValue([]);
    mockGetStatus.mockResolvedValue(makeStatus({ rlsEnabled: true }));
    mockDisableRls.mockResolvedValueOnce({ message: "disabled" });
    render(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("Disable RLS")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Disable RLS"));

    await waitFor(() => {
      expect(mockDisableRls).toHaveBeenCalledWith("posts");
    });
  });

  it("shows SQL preview when View SQL clicked", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy()]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("Roles: authenticated, admin")).toBeInTheDocument();
    });
  });

  it("shows PERMISSIVE badge", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy({ permissive: "PERMISSIVE" })]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("PERMISSIVE")).toBeInTheDocument();
    });
  });

  it("shows RESTRICTIVE badge", async () => {
    mockListPolicies.mockResolvedValueOnce([makePolicy({ permissive: "RESTRICTIVE" })]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={schema} />);

    await waitFor(() => {
      expect(screen.getByText("posts")).toBeInTheDocument();
    });
    // Views should not appear in the table list
    expect(screen.queryByText("my_view")).not.toBeInTheDocument();
  });

  it("retry button refetches data after error", async () => {
    mockListPolicies.mockRejectedValueOnce(new Error("db down"));
    mockGetStatus.mockRejectedValueOnce(new Error("db down"));
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
        usingExpr: "(user_id = current_setting('app.user_id')::uuid)",
        withCheckExpr: "(user_id = current_setting('app.user_id')::uuid)",
      }),
    ]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema(["posts"])} />);

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
      expect(screen.getByText("Create your first policy")).toBeInTheDocument();
    });
  });

  it("create your first policy button opens modal", async () => {
    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    render(<RlsPolicies schema={makeSchema(["posts"])} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={makeSchema()} />);

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
    render(<RlsPolicies schema={schema} />);

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

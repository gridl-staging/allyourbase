import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders } from "../../test-utils";
import { RlsPolicies } from "../RlsPolicies";
import {
  deleteRlsPolicy,
  getRlsStatus,
  listRlsPolicies,
} from "../../api";
import type { SchemaCache } from "../../types";
import { makePolicy } from "./rls-test-fixtures";

vi.mock("../../api", async () => {
  const actual = await vi.importActual<typeof import("../../api")>("../../api");
  return {
    ...actual,
    listRlsPolicies: vi.fn(),
    getRlsStatus: vi.fn(),
    createRlsPolicy: vi.fn(),
    deleteRlsPolicy: vi.fn(),
    enableRls: vi.fn(),
    disableRls: vi.fn(),
  };
});

const mockDeletePolicy = vi.mocked(deleteRlsPolicy);
const mockGetStatus = vi.mocked(getRlsStatus);
const mockListPolicies = vi.mocked(listRlsPolicies);

function makeSchemaWithPrivatePosts(): SchemaCache {
  return {
    tables: {
      "private.posts": {
        schema: "private",
        name: "posts",
        kind: "table",
        columns: [],
        primaryKey: [],
      },
      "public.posts": {
        schema: "public",
        name: "posts",
        kind: "table",
        columns: [],
        primaryKey: [],
      },
    },
    schemas: ["private", "public"],
    builtAt: "2026-03-20T00:00:00Z",
  };
}

describe("RlsPolicies schema-qualified table actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("uses schema-qualified identifiers when managing a non-public table", async () => {
    mockListPolicies.mockImplementation(async (table) =>
      table === "private.posts"
        ? [makePolicy({ tableSchema: "private", tableName: "posts", policyName: "private_access" })]
        : [],
    );
    mockGetStatus.mockResolvedValue({ rlsEnabled: true, forceRls: false });
    mockDeletePolicy.mockResolvedValueOnce(undefined);

    renderWithProviders(<RlsPolicies schema={makeSchemaWithPrivatePosts()} />);

    await waitFor(() => {
      expect(screen.getByText("Add Policy")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await waitFor(() => {
      expect(mockListPolicies).toHaveBeenCalledWith("private.posts");
      expect(mockGetStatus).toHaveBeenCalledWith("private.posts");
      expect(screen.getByText("private_access")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Delete policy"));
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(mockDeletePolicy).toHaveBeenCalledWith("private.posts", "private_access");
    });
  });
});

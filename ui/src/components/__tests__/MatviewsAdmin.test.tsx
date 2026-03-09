import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MatviewsAdmin } from "../MatviewsAdmin";
import {
  listMatviews,
  registerMatview,
  updateMatview,
  deleteMatview,
  refreshMatview,
} from "../../api";
import type {
  MatviewListResponse,
  MatviewRegistration,
  MatviewRefreshResult,
  SchemaCache,
} from "../../types";

vi.mock("../../api", () => ({
  listMatviews: vi.fn(),
  registerMatview: vi.fn(),
  updateMatview: vi.fn(),
  deleteMatview: vi.fn(),
  refreshMatview: vi.fn(),
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListMatviews = vi.mocked(listMatviews);
const mockRegisterMatview = vi.mocked(registerMatview);
const mockUpdateMatview = vi.mocked(updateMatview);
const mockDeleteMatview = vi.mocked(deleteMatview);
const mockRefreshMatview = vi.mocked(refreshMatview);

function makeRegistration(overrides: Partial<MatviewRegistration> = {}): MatviewRegistration {
  return {
    id: "mv1",
    schemaName: "public",
    viewName: "leaderboard",
    refreshMode: "standard",
    lastRefreshAt: null,
    lastRefreshDurationMs: null,
    lastRefreshStatus: null,
    lastRefreshError: null,
    createdAt: "2026-02-22T09:00:00Z",
    updatedAt: "2026-02-22T09:00:00Z",
    ...overrides,
  };
}

function makeListResponse(items: MatviewRegistration[]): MatviewListResponse {
  return { items, count: items.length };
}

function makeRefreshResult(reg: MatviewRegistration, durationMs = 42): MatviewRefreshResult {
  return { registration: reg, durationMs };
}

const minimalSchema: SchemaCache = {
  tables: {
    "public.leaderboard": {
      schema: "public",
      name: "leaderboard",
      kind: "materialized_view",
      columns: [],
      primaryKey: [],
    },
    "public.stats_daily": {
      schema: "public",
      name: "stats_daily",
      kind: "materialized_view",
      columns: [],
      primaryKey: [],
    },
    "public.users": {
      schema: "public",
      name: "users",
      kind: "table",
      columns: [],
      primaryKey: ["id"],
    },
  },
  schemas: ["public"],
  builtAt: "2026-02-22T10:00:00Z",
};

describe("MatviewsAdmin", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListMatviews.mockResolvedValue(makeListResponse([]));
    mockRegisterMatview.mockResolvedValue(makeRegistration());
    mockUpdateMatview.mockResolvedValue(makeRegistration());
    mockDeleteMatview.mockResolvedValue(undefined);
    mockRefreshMatview.mockResolvedValue(
      makeRefreshResult(makeRegistration({ lastRefreshStatus: "success" })),
    );
  });

  it("shows loading state", () => {
    mockListMatviews.mockReturnValue(new Promise(() => {}));
    render(<MatviewsAdmin schema={minimalSchema} />);
    expect(screen.getByText("Loading materialized views...")).toBeInTheDocument();
  });

  it("shows error state with retry", async () => {
    mockListMatviews.mockRejectedValueOnce(new Error("connection refused"));
    render(<MatviewsAdmin schema={minimalSchema} />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
    });

    mockListMatviews.mockResolvedValueOnce(makeListResponse([]));
    await userEvent.setup().click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("No materialized views registered")).toBeInTheDocument();
    });
  });

  it("renders empty state", async () => {
    render(<MatviewsAdmin schema={minimalSchema} />);

    await waitFor(() => {
      expect(screen.getByText("No materialized views registered")).toBeInTheDocument();
    });
  });

  it("renders matview table with status columns", async () => {
    mockListMatviews.mockResolvedValueOnce(
      makeListResponse([
        makeRegistration({
          id: "mv1",
          viewName: "leaderboard",
          refreshMode: "standard",
          lastRefreshAt: "2026-02-22T10:00:00Z",
          lastRefreshDurationMs: 152,
          lastRefreshStatus: "success",
        }),
        makeRegistration({
          id: "mv2",
          viewName: "stats_daily",
          refreshMode: "concurrent",
          lastRefreshStatus: "error",
          lastRefreshError: "unique index missing",
        }),
      ]),
    );

    render(<MatviewsAdmin schema={minimalSchema} />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Materialized Views" })).toBeInTheDocument();
      expect(screen.getByText("leaderboard")).toBeInTheDocument();
      expect(screen.getByText("stats_daily")).toBeInTheDocument();
      expect(screen.getByText("standard")).toBeInTheDocument();
      expect(screen.getByText("concurrent")).toBeInTheDocument();
      expect(screen.getByText("success")).toBeInTheDocument();
      expect(screen.getByText("error")).toBeInTheDocument();
      expect(screen.getByText("152ms")).toBeInTheDocument();
      expect(screen.getByText("unique index missing")).toBeInTheDocument();
    });
  });

  it("triggers refresh for a matview", async () => {
    const reg = makeRegistration({ id: "mv-refresh" });
    mockListMatviews.mockResolvedValueOnce(makeListResponse([reg]));
    mockRefreshMatview.mockResolvedValueOnce(
      makeRefreshResult(makeRegistration({ id: "mv-refresh", lastRefreshStatus: "success" }), 55),
    );

    render(<MatviewsAdmin schema={minimalSchema} />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Refresh matview mv-refresh")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Refresh matview mv-refresh"));

    await waitFor(() => {
      expect(mockRefreshMatview).toHaveBeenCalledWith("mv-refresh");
    });
  });

  it("opens register modal with matview dropdown from schema cache", async () => {
    render(<MatviewsAdmin schema={minimalSchema} />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Register Matview" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Register Matview" }));

    await waitFor(() => {
      expect(screen.getByText("Register Materialized View")).toBeInTheDocument();
    });

    // Should show matviews from schema cache in the dropdown, but NOT regular tables
    const viewSelect = screen.getByLabelText("View");
    const options = within(viewSelect).getAllByRole("option");
    const optionTexts = options.map((o) => o.textContent);
    expect(optionTexts).toContain("public.leaderboard");
    expect(optionTexts).toContain("public.stats_daily");
    expect(optionTexts).not.toContain("public.users");
  });

  it("registers a matview via modal", async () => {
    render(<MatviewsAdmin schema={minimalSchema} />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Register Matview" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Register Matview" }));
    await user.selectOptions(screen.getByLabelText("View"), "public.leaderboard");
    await user.selectOptions(screen.getByLabelText("Refresh Mode"), "concurrent");
    await user.click(screen.getByRole("button", { name: "Register" }));

    await waitFor(() => {
      expect(mockRegisterMatview).toHaveBeenCalledWith({
        schema: "public",
        viewName: "leaderboard",
        refreshMode: "concurrent",
      });
    });
  });

  it("updates refresh mode via edit action", async () => {
    mockListMatviews.mockResolvedValueOnce(
      makeListResponse([makeRegistration({ id: "mv-edit", refreshMode: "standard" })]),
    );

    render(<MatviewsAdmin schema={minimalSchema} />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Edit matview mv-edit")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Edit matview mv-edit"));

    await waitFor(() => {
      expect(screen.getByText("Edit Refresh Mode")).toBeInTheDocument();
    });

    await user.selectOptions(screen.getByLabelText("Refresh Mode"), "concurrent");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateMatview).toHaveBeenCalledWith("mv-edit", {
        refreshMode: "concurrent",
      });
    });
  });

  it("unregisters a matview via delete action", async () => {
    mockListMatviews.mockResolvedValueOnce(
      makeListResponse([makeRegistration({ id: "mv-del", viewName: "leaderboard" })]),
    );

    render(<MatviewsAdmin schema={minimalSchema} />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Delete matview mv-del")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Delete matview mv-del"));

    await waitFor(() => {
      expect(screen.getByText("Unregister materialized view?")).toBeInTheDocument();
      // "leaderboard" appears in both the table row and the modal confirmation
      expect(screen.getAllByText("leaderboard").length).toBeGreaterThanOrEqual(2);
    });

    await user.click(screen.getByRole("button", { name: "Unregister" }));

    await waitFor(() => {
      expect(mockDeleteMatview).toHaveBeenCalledWith("mv-del");
    });
  });

  it("shows never-refreshed state as dash", async () => {
    mockListMatviews.mockResolvedValueOnce(
      makeListResponse([
        makeRegistration({
          lastRefreshAt: null,
          lastRefreshStatus: null,
          lastRefreshDurationMs: null,
        }),
      ]),
    );

    render(<MatviewsAdmin schema={minimalSchema} />);

    await waitFor(() => {
      // For never-refreshed matviews, status/duration/time should show "-"
      const cells = screen.getAllByText("-");
      expect(cells.length).toBeGreaterThanOrEqual(2);
    });
  });

  it("displays refresh error preview truncated", async () => {
    const longError = "a]".repeat(60);
    mockListMatviews.mockResolvedValueOnce(
      makeListResponse([
        makeRegistration({
          lastRefreshStatus: "error",
          lastRefreshError: longError,
        }),
      ]),
    );

    render(<MatviewsAdmin schema={minimalSchema} />);

    await waitFor(() => {
      // Should truncate long errors
      const errorCells = screen.getAllByText(/\.\.\.$/);
      expect(errorCells.length).toBeGreaterThanOrEqual(1);
    });
  });
});

import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MockApiError } from "../../test-utils";

vi.mock("../../api", () => ({
  getRows: vi.fn(),
  createRecord: vi.fn(),
  updateRecord: vi.fn(),
  deleteRecord: vi.fn(),
  batchRecords: vi.fn(),
  ApiError: MockApiError,
}));

// Mock RecordForm to avoid testing its internals here.
vi.mock("../RecordForm", () => ({
  RecordForm: ({
    mode,
    onSubmit,
    onClose,
  }: {
    mode: string;
    onSubmit: (d: Record<string, unknown>) => Promise<void>;
    onClose: () => void;
  }) => (
    <div data-testid={`record-form-${mode}`}>
      <button onClick={() => onSubmit({ title: "from-form" })}>
        mock-submit
      </button>
      <button onClick={onClose}>mock-close</button>
    </div>
  ),
}));

import { getRows, createRecord, deleteRecord, batchRecords } from "../../api";
import { TableBrowser } from "../TableBrowser";
import type { Table } from "../../types";

const mockGetRows = vi.mocked(getRows);
const mockCreateRecord = vi.mocked(createRecord);
const mockDeleteRecord = vi.mocked(deleteRecord);
const mockBatchRecords = vi.mocked(batchRecords);

function makeTable(overrides: Partial<Table> = {}): Table {
  return {
    schema: "public",
    name: "posts",
    kind: "table",
    columns: [
      {
        name: "id",
        position: 1,
        type: "uuid",
        nullable: false,
        isPrimaryKey: true,
        jsonType: "string",
      },
      {
        name: "title",
        position: 2,
        type: "text",
        nullable: false,
        isPrimaryKey: false,
        jsonType: "string",
      },
    ],
    primaryKey: ["id"],
    ...overrides,
  };
}

const emptyResponse = {
  items: [],
  page: 1,
  perPage: 20,
  totalItems: 0,
  totalPages: 0,
};

const oneRowResponse = {
  items: [{ id: "abc-123", title: "Hello" }],
  page: 1,
  perPage: 20,
  totalItems: 1,
  totalPages: 1,
};

const twoRowResponse = {
  items: [
    { id: "abc-123", title: "Hello" },
    { id: "def-456", title: "World" },
  ],
  page: 1,
  perPage: 20,
  totalItems: 2,
  totalPages: 1,
};

const multiPageResponse = {
  items: Array.from({ length: 20 }, (_, i) => ({
    id: String(i),
    title: `Post ${i}`,
  })),
  page: 1,
  perPage: 20,
  totalItems: 45,
  totalPages: 3,
};

describe("TableBrowser", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders column headers", async () => {
    mockGetRows.mockResolvedValueOnce(emptyResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("id")).toBeInTheDocument();
      expect(screen.getByText("title")).toBeInTheDocument();
    });
  });

  it("fetches and displays rows", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
      expect(screen.getByText("abc-123")).toBeInTheDocument();
    });
    expect(mockGetRows).toHaveBeenCalledWith("posts", {
      page: 1,
      perPage: 20,
      sort: undefined,
      filter: undefined,
      search: undefined,
      expand: undefined,
    });
  });

  it("shows empty state when no rows", async () => {
    mockGetRows.mockResolvedValueOnce(emptyResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("No rows found")).toBeInTheDocument();
    });
  });

  it("shows error on fetch failure", async () => {
    const { ApiError } = await import("../../api");
    mockGetRows.mockRejectedValueOnce(
      new ApiError(400, "invalid filter syntax"),
    );
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("invalid filter syntax")).toBeInTheDocument();
    });
  });

  it("shows generic error for non-API errors", async () => {
    mockGetRows.mockRejectedValueOnce(new Error("network"));
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Failed to load data")).toBeInTheDocument();
    });
  });

  it("shows PK badge on primary key columns", async () => {
    mockGetRows.mockResolvedValueOnce(emptyResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("PK")).toBeInTheDocument();
    });
  });

  it("renders null cells as italic null", async () => {
    mockGetRows.mockResolvedValueOnce({
      items: [{ id: "1", title: null }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("null")).toBeInTheDocument();
    });
  });

  it("renders boolean cells with color", async () => {
    const table = makeTable({
      columns: [
        {
          name: "id",
          position: 1,
          type: "uuid",
          nullable: false,
          isPrimaryKey: true,
          jsonType: "string",
        },
        {
          name: "active",
          position: 2,
          type: "boolean",
          nullable: false,
          isPrimaryKey: false,
          jsonType: "boolean",
        },
      ],
    });
    mockGetRows.mockResolvedValueOnce({
      items: [{ id: "1", active: true }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    render(<TableBrowser table={table} />);

    await waitFor(() => {
      expect(screen.getByText("true")).toBeInTheDocument();
    });
  });

  it("renders object cells as JSON", async () => {
    const table = makeTable({
      columns: [
        {
          name: "id",
          position: 1,
          type: "uuid",
          nullable: false,
          isPrimaryKey: true,
          jsonType: "string",
        },
        {
          name: "meta",
          position: 2,
          type: "jsonb",
          nullable: true,
          isPrimaryKey: false,
          jsonType: "object",
        },
      ],
    });
    mockGetRows.mockResolvedValueOnce({
      items: [{ id: "1", meta: { key: "val" } }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    render(<TableBrowser table={table} />);

    await waitFor(() => {
      expect(screen.getByText('{"key":"val"}')).toBeInTheDocument();
    });
  });

  // --- Pagination ---

  it("shows total row count", async () => {
    mockGetRows.mockResolvedValueOnce(multiPageResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("45 rows")).toBeInTheDocument();
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });
  });

  it("singular row text for 1 row", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("1 row")).toBeInTheDocument();
    });
  });

  it("navigates to next page", async () => {
    mockGetRows.mockResolvedValueOnce(multiPageResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });

    mockGetRows.mockResolvedValueOnce({ ...multiPageResponse, page: 2 });
    const user = userEvent.setup();
    const paginationBar = screen.getByText("1 / 3").parentElement!;
    const navButtons = within(paginationBar).getAllByRole("button");
    await user.click(navButtons[1]); // second is "next"

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ page: 2 }),
      );
    });
  });

  // --- Sorting ---

  it("toggles sort ascending then descending on column click", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    // Click "title" column header.
    const titleHeader = screen.getByText("title").closest("th")!;
    await user.click(titleHeader);

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ sort: "+title" }),
      );
    });

    // Click again for descending.
    await user.click(titleHeader);

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ sort: "-title" }),
      );
    });
  });

  // --- Filtering ---

  it("applies filter on Enter key", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const filterInput = screen.getByPlaceholderText(/Filter/);
    await user.type(filterInput, "status='active'{Enter}");

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ filter: "status='active'" }),
      );
    });
  });

  it("applies filter on Apply button click", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const filterInput = screen.getByPlaceholderText(/Filter/);
    await user.type(filterInput, "age>10");
    await user.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ filter: "age>10" }),
      );
    });
  });

  // --- New button / views ---

  it("shows New button for writable tables with PK", async () => {
    mockGetRows.mockResolvedValueOnce(emptyResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /New/ })).toBeInTheDocument();
    });
  });

  it("hides New button for views", async () => {
    mockGetRows.mockResolvedValueOnce(emptyResponse);
    render(<TableBrowser table={makeTable({ kind: "view" })} />);

    await waitFor(() => {
      expect(screen.getByText("No rows found")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /New/ })).not.toBeInTheDocument();
  });

  it("opens create form when New clicked", async () => {
    mockGetRows.mockResolvedValue(emptyResponse);
    const user = userEvent.setup();
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /New/ })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /New/ }));

    expect(screen.getByTestId("record-form-create")).toBeInTheDocument();
  });

  it("create form submit calls createRecord and refreshes", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    mockCreateRecord.mockResolvedValue({ id: "new", title: "from-form" });
    const user = userEvent.setup();
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /New/ }));
    await user.click(screen.getByRole("button", { name: "mock-submit" }));

    await waitFor(() => {
      expect(mockCreateRecord).toHaveBeenCalledWith("posts", {
        title: "from-form",
      });
    });
  });

  // --- Row detail ---

  it("clicking a row opens detail drawer", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    // Click the table row (the tr element).
    const row = screen.getByText("Hello").closest("tr")!;
    await user.click(row);

    expect(screen.getByText("Row Detail")).toBeInTheDocument();
    // abc-123 now appears in both the table and the drawer.
    expect(screen.getAllByText("abc-123").length).toBeGreaterThanOrEqual(2);
  });

  // --- Delete ---

  it("delete confirmation shows table name and PK", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    // Open detail drawer, then click delete.
    const user = userEvent.setup();
    const row = screen.getByText("Hello").closest("tr")!;
    await user.click(row);
    // The drawer has Edit and Delete title buttons.
    await user.click(screen.getAllByTitle("Delete")[0]);

    expect(screen.getByText("Delete record?")).toBeInTheDocument();
    expect(screen.getByText("posts", { selector: "strong" })).toBeInTheDocument();
    expect(screen.getByText("id=abc-123")).toBeInTheDocument();
  });

  it("confirming delete calls deleteRecord and refreshes", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    mockDeleteRecord.mockResolvedValue(undefined);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const row = screen.getByText("Hello").closest("tr")!;
    await user.click(row);
    await user.click(screen.getAllByTitle("Delete")[0]);

    // The confirmation dialog has a "Delete" button — scope to the dialog.
    const dialog = screen.getByText("Delete record?").closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Delete" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockDeleteRecord).toHaveBeenCalledWith("posts", "abc-123");
    });
  });

  it("cancel on delete confirmation closes modal", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const row = screen.getByText("Hello").closest("tr")!;
    await user.click(row);
    await user.click(screen.getAllByTitle("Delete")[0]);
    expect(screen.getByText("Delete record?")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Delete record?")).not.toBeInTheDocument();
  });

  // --- Full-text search ---

  it("renders search input", async () => {
    mockGetRows.mockResolvedValueOnce(emptyResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(
        screen.getByRole("textbox", { name: "Full-text search" }),
      ).toBeInTheDocument();
    });
  });

  it("applies search on Enter key", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const searchInput = screen.getByRole("textbox", { name: "Full-text search" });
    await user.type(searchInput, "postgres database{Enter}");

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ search: "postgres database" }),
      );
    });
  });

  it("clears search with X button", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const searchInput = screen.getByRole("textbox", { name: "Full-text search" });
    await user.type(searchInput, "test query{Enter}");

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ search: "test query" }),
      );
    });

    // Click the clear search button.
    await user.click(screen.getByRole("button", { name: "Clear search" }));

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ search: undefined }),
      );
    });
  });

  it("search and filter can be used together", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const searchInput = screen.getByRole("textbox", { name: "Full-text search" });
    await user.type(searchInput, "hello");

    const filterInput = screen.getByPlaceholderText(/Filter/);
    await user.type(filterInput, "status='active'");

    await user.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({
          search: "hello",
          filter: "status='active'",
        }),
      );
    });
  });

  // --- Export ---

  it("shows Export button when data is loaded", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    expect(screen.getByRole("button", { name: "Export" })).toBeInTheDocument();
  });

  it("hides Export button when no data", async () => {
    mockGetRows.mockResolvedValueOnce(emptyResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("No rows found")).toBeInTheDocument();
    });

    expect(screen.queryByRole("button", { name: "Export" })).not.toBeInTheDocument();
  });

  it("shows CSV and JSON options in Export menu", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Export" }));

    expect(screen.getByText("CSV")).toBeInTheDocument();
    expect(screen.getByText("JSON")).toBeInTheDocument();
  });

  it("CSV export triggers blob download", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const createObjectURL = vi.fn(() => "blob:test");
    const revokeObjectURL = vi.fn();
    (globalThis as Record<string, unknown>).URL = { createObjectURL, revokeObjectURL };

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Export" }));
    await user.click(screen.getByText("CSV"));

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    const blob = (createObjectURL.mock.calls as unknown[][])[0][0] as Blob;
    expect(blob.type).toBe("text/csv");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:test");
  });

  it("JSON export triggers blob download", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const createObjectURL = vi.fn(() => "blob:test");
    const revokeObjectURL = vi.fn();
    (globalThis as Record<string, unknown>).URL = { createObjectURL, revokeObjectURL };

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Export" }));
    await user.click(screen.getByText("JSON"));

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    const blob = (createObjectURL.mock.calls as unknown[][])[0][0] as Blob;
    expect(blob.type).toBe("application/json");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:test");
  });

  // --- Batch selection ---

  it("shows select-all checkbox for writable tables with PK", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    expect(screen.getByRole("checkbox", { name: "Select all" })).toBeInTheDocument();
  });

  it("hides checkboxes for views", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable({ kind: "view" })} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    expect(screen.queryByRole("checkbox", { name: "Select all" })).not.toBeInTheDocument();
  });

  it("shows per-row checkboxes", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    expect(screen.getByRole("checkbox", { name: "Select row abc-123" })).toBeInTheDocument();
  });

  it("selecting a row shows Delete Selected button", async () => {
    mockGetRows.mockResolvedValueOnce(twoRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "Select row abc-123" }));

    expect(screen.getByRole("button", { name: "Delete selected" })).toBeInTheDocument();
    expect(screen.getByText("Delete (1)")).toBeInTheDocument();
  });

  it("select-all selects all rows on current page", async () => {
    mockGetRows.mockResolvedValueOnce(twoRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "Select all" }));

    expect(screen.getByText("Delete (2)")).toBeInTheDocument();
  });

  it("select-all deselects when all are selected", async () => {
    mockGetRows.mockResolvedValueOnce(twoRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    // Select all
    await user.click(screen.getByRole("checkbox", { name: "Select all" }));
    expect(screen.getByText("Delete (2)")).toBeInTheDocument();

    // Deselect all
    await user.click(screen.getByRole("checkbox", { name: "Select all" }));
    expect(screen.queryByRole("button", { name: "Delete selected" })).not.toBeInTheDocument();
  });

  it("batch delete confirmation shows count and table name", async () => {
    mockGetRows.mockResolvedValueOnce(twoRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "Select all" }));
    await user.click(screen.getByRole("button", { name: "Delete selected" }));

    expect(screen.getByText("Delete 2 records?")).toBeInTheDocument();
    expect(screen.getByText("posts", { selector: "strong" })).toBeInTheDocument();
  });

  it("confirming batch delete calls batchRecords and clears selection", async () => {
    mockGetRows.mockResolvedValue(twoRowResponse);
    mockBatchRecords.mockResolvedValue([
      { index: 0, status: 204 },
      { index: 1, status: 204 },
    ]);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "Select all" }));
    await user.click(screen.getByRole("button", { name: "Delete selected" }));

    const dialog = screen.getByText("Delete 2 records?").closest("div.fixed")! as HTMLElement;
    await user.click(within(dialog).getByRole("button", { name: "Delete 2" }));

    await waitFor(() => {
      expect(mockBatchRecords).toHaveBeenCalledWith("posts", [
        { method: "delete", id: "abc-123" },
        { method: "delete", id: "def-456" },
      ]);
    });
  });

  it("cancelling batch delete keeps selection", async () => {
    mockGetRows.mockResolvedValueOnce(twoRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "Select all" }));
    await user.click(screen.getByRole("button", { name: "Delete selected" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    // Selection still visible
    expect(screen.getByText("Delete (2)")).toBeInTheDocument();
  });

  // --- FK Expand ---

  it("shows Expand button when table has many-to-one relationships", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    const table = makeTable({
      relationships: [
        {
          name: "posts_author_fkey",
          type: "many-to-one",
          fromSchema: "public",
          fromTable: "posts",
          fromColumns: ["author_id"],
          toSchema: "public",
          toTable: "users",
          toColumns: ["id"],
          fieldName: "author",
        },
      ],
    });
    render(<TableBrowser table={table} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    expect(screen.getByRole("button", { name: "Expand" })).toBeInTheDocument();
  });

  it("hides Expand button when no relationships", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    render(<TableBrowser table={makeTable()} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    expect(screen.queryByRole("button", { name: "Expand" })).not.toBeInTheDocument();
  });

  it("clicking Expand shows relation checkboxes", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    const table = makeTable({
      relationships: [
        {
          name: "posts_author_fkey",
          type: "many-to-one",
          fromSchema: "public",
          fromTable: "posts",
          fromColumns: ["author_id"],
          toSchema: "public",
          toTable: "users",
          toColumns: ["id"],
          fieldName: "author",
        },
      ],
    });
    render(<TableBrowser table={table} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Expand" }));

    expect(screen.getByText("author")).toBeInTheDocument();
    expect(screen.getByText("users")).toBeInTheDocument();
  });

  it("toggling expand relation passes expand param to getRows", async () => {
    mockGetRows.mockResolvedValue(oneRowResponse);
    const table = makeTable({
      relationships: [
        {
          name: "posts_author_fkey",
          type: "many-to-one",
          fromSchema: "public",
          fromTable: "posts",
          fromColumns: ["author_id"],
          toSchema: "public",
          toTable: "users",
          toColumns: ["id"],
          fieldName: "author",
        },
      ],
    });
    render(<TableBrowser table={table} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Expand" }));
    await user.click(screen.getByText("author"));

    await waitFor(() => {
      expect(mockGetRows).toHaveBeenCalledWith(
        "posts",
        expect.objectContaining({ expand: "author" }),
      );
    });
  });

  it("shows expanded data in column when relation is expanded", async () => {
    const expandedResponse = {
      items: [
        {
          id: "abc-123",
          title: "Hello",
          author_id: "user-1",
          expand: { author: { id: "user-1", email: "alice@example.com" } },
        },
      ],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    };
    mockGetRows
      .mockResolvedValueOnce(oneRowResponse) // initial load
      .mockResolvedValueOnce(expandedResponse); // after expand toggled

    const table = makeTable({
      relationships: [
        {
          name: "posts_author_fkey",
          type: "many-to-one",
          fromSchema: "public",
          fromTable: "posts",
          fromColumns: ["author_id"],
          toSchema: "public",
          toTable: "users",
          toColumns: ["id"],
          fieldName: "author",
        },
      ],
    });
    render(<TableBrowser table={table} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Expand" }));
    await user.click(screen.getByText("author"));

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });
  });

  it("does not show Expand for one-to-many only relationships", async () => {
    mockGetRows.mockResolvedValueOnce(oneRowResponse);
    const table = makeTable({
      relationships: [
        {
          name: "comments_post_fkey",
          type: "one-to-many",
          fromSchema: "public",
          fromTable: "posts",
          fromColumns: ["id"],
          toSchema: "public",
          toTable: "comments",
          toColumns: ["post_id"],
          fieldName: "comments",
        },
      ],
    });
    render(<TableBrowser table={table} />);

    await waitFor(() => {
      expect(screen.getByText("Hello")).toBeInTheDocument();
    });

    expect(screen.queryByRole("button", { name: "Expand" })).not.toBeInTheDocument();
  });
});

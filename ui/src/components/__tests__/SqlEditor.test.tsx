import { render, screen, waitFor, act, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MockApiError } from "../../test-utils";

// Mock CodeMirror as a controlled textarea so we can test the component logic
// without fighting with CodeMirror's complex DOM in jsdom.
let cmOnChange: ((value: string) => void) | undefined;
vi.mock("@uiw/react-codemirror", () => ({
  default: (props: {
    value: string;
    onChange: (v: string) => void;
    extensions?: unknown[];
    placeholder?: string;
  }) => {
    cmOnChange = props.onChange;
    return (
      <textarea
        data-testid="cm-editor"
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        placeholder={props.placeholder}
        aria-label="SQL query"
      />
    );
  },
}));

vi.mock("@codemirror/lang-sql", () => ({
  sql: () => [],
  PostgreSQL: {},
}));

vi.mock("@codemirror/view", () => ({
  keymap: { of: () => [] },
  EditorView: { contentAttributes: { of: () => [] } },
}));

vi.mock("../../api", () => ({
  executeSQL: vi.fn(),
  ApiError: MockApiError,
}));

import { SqlEditor, resultToCSV, resultToJSON } from "../SqlEditor";
import { executeSQL } from "../../api";
const mockExecuteSQL = vi.mocked(executeSQL);

const mockWriteText = vi.fn().mockResolvedValue(undefined);

describe("resultToCSV", () => {
  it("converts result to CSV", () => {
    const csv = resultToCSV({
      columns: ["id", "name"],
      rows: [
        [1, "alice"],
        [2, "bob"],
      ],
      rowCount: 2,
      durationMs: 1,
    });
    expect(csv).toBe("id,name\n1,alice\n2,bob");
  });

  it("handles commas and quotes in values", () => {
    const csv = resultToCSV({
      columns: ["val"],
      rows: [['hello, "world"']],
      rowCount: 1,
      durationMs: 1,
    });
    expect(csv).toBe('val\n"hello, ""world"""');
  });

  it("handles null values", () => {
    const csv = resultToCSV({
      columns: ["a"],
      rows: [[null]],
      rowCount: 1,
      durationMs: 1,
    });
    expect(csv).toBe("a\n");
  });
});

describe("resultToJSON", () => {
  it("converts result to JSON array of objects", () => {
    const json = resultToJSON({
      columns: ["id", "name"],
      rows: [[1, "alice"]],
      rowCount: 1,
      durationMs: 1,
    });
    expect(JSON.parse(json)).toEqual([{ id: 1, name: "alice" }]);
  });

  it("handles null values", () => {
    const json = resultToJSON({
      columns: ["a"],
      rows: [[null]],
      rowCount: 1,
      durationMs: 1,
    });
    expect(JSON.parse(json)).toEqual([{ a: null }]);
  });
});

describe("SqlEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    cmOnChange = undefined;
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: mockWriteText },
      writable: true,
      configurable: true,
    });
  });

  it("renders with default query", () => {
    render(<SqlEditor />);

    const editor = screen.getByTestId("cm-editor");
    expect(editor).toBeInTheDocument();
    expect(editor).toHaveValue("SELECT 1 AS hello;");
    expect(screen.getByRole("button", { name: /Execute/ })).toBeInTheDocument();
  });

  it("restores query from localStorage", () => {
    localStorage.setItem("ayb_sql_query", "SELECT * FROM users;");
    render(<SqlEditor />);

    expect(screen.getByTestId("cm-editor")).toHaveValue("SELECT * FROM users;");
  });

  it("executes query and displays results", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["id", "name"],
      rows: [
        [1, "alice"],
        [2, "bob"],
      ],
      rowCount: 2,
      durationMs: 5,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText("id")).toBeInTheDocument();
      expect(screen.getByText("name")).toBeInTheDocument();
      expect(screen.getByText("alice")).toBeInTheDocument();
      expect(screen.getByText("bob")).toBeInTheDocument();
      expect(screen.getByText(/2 rows in 5ms/)).toBeInTheDocument();
    });
  });

  it("shows error on query failure", async () => {
    const { ApiError } = await import("../../api");
    mockExecuteSQL.mockRejectedValueOnce(
      new ApiError(400, 'relation "nonexistent" does not exist'),
    );
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(
        screen.getByText('relation "nonexistent" does not exist'),
      ).toBeInTheDocument();
    });
  });

  it("renders null cells as italic null", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["name"],
      rows: [[null]],
      rowCount: 1,
      durationMs: 1,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText("null")).toBeInTheDocument();
    });
  });

  it("shows placeholder when no results", () => {
    render(<SqlEditor />);
    expect(screen.getByText("Run a query to see results")).toBeInTheDocument();
  });

  it("shows success message for DDL without columns", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: [],
      rows: [],
      rowCount: 0,
      durationMs: 2,
    });
    const user = userEvent.setup();

    // Set query to a CREATE statement so classifyQuery returns "ddl"
    localStorage.setItem("ayb_sql_query", "CREATE TABLE foo (id int);");
    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(
        screen.getByText(/Statement executed successfully/),
      ).toBeInTheDocument();
    });
  });

  it("shows affected rows for DML without columns", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: [],
      rows: [],
      rowCount: 3,
      durationMs: 2,
    });
    const user = userEvent.setup();

    // Set query to an INSERT so classifyQuery returns "dml"
    localStorage.setItem(
      "ayb_sql_query",
      "INSERT INTO foo VALUES (1),(2),(3);",
    );
    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText(/3 rows affected/)).toBeInTheDocument();
    });
  });

  it("saves query to localStorage on successful execution", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["x"],
      rows: [[1]],
      rowCount: 1,
      durationMs: 1,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    const editor = screen.getByTestId("cm-editor");
    await user.clear(editor);
    await user.type(editor, "SELECT 42;");
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(localStorage.getItem("ayb_sql_query")).toBe("SELECT 42;");
    });
  });

  it("does not execute empty query", async () => {
    render(<SqlEditor />);
    // Simulate clearing the editor via onChange
    act(() => {
      cmOnChange?.("");
    });

    // Button should be disabled when query is empty.
    expect(screen.getByRole("button", { name: /Execute/ })).toBeDisabled();
    expect(mockExecuteSQL).not.toHaveBeenCalled();
  });

  it("renders JSON object cells as stringified JSON", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["data"],
      rows: [[{ key: "value" }]],
      rowCount: 1,
      durationMs: 1,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText('{"key":"value"}')).toBeInTheDocument();
    });
  });

  it("singular row text for 1 row", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["x"],
      rows: [[1]],
      rowCount: 1,
      durationMs: 10,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText(/1 row in 10ms/)).toBeInTheDocument();
    });
  });

  it("shows copy CSV and copy JSON buttons when results are displayed", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["id", "name"],
      rows: [[1, "alice"]],
      rowCount: 1,
      durationMs: 1,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByTitle("Copy as CSV")).toBeInTheDocument();
      expect(screen.getByTitle("Copy as JSON")).toBeInTheDocument();
    });
  });

  it("clicking copy CSV shows feedback", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["id", "name"],
      rows: [
        [1, "alice"],
        [2, "bob"],
      ],
      rowCount: 2,
      durationMs: 1,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByTitle("Copy as CSV")).toBeInTheDocument();
    });

    await act(async () => {
      fireEvent.click(screen.getByTitle("Copy as CSV"));
    });

    expect(screen.getByText("CSV copied!")).toBeInTheDocument();
  });

  it("clicking copy JSON shows feedback", async () => {
    mockExecuteSQL.mockResolvedValueOnce({
      columns: ["id", "name"],
      rows: [[1, "alice"]],
      rowCount: 1,
      durationMs: 1,
    });
    const user = userEvent.setup();

    render(<SqlEditor />);
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByTitle("Copy as JSON")).toBeInTheDocument();
    });

    await act(async () => {
      fireEvent.click(screen.getByTitle("Copy as JSON"));
    });

    expect(screen.getByText("JSON copied!")).toBeInTheDocument();
  });
});

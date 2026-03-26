import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

describe("SqlEditor CodeMirror integration", () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.resetModules();
  });

  it("does not import @codemirror/view directly", async () => {
    vi.doMock("@codemirror/view", () => {
      throw new Error("SqlEditor must not import @codemirror/view directly");
    });

    vi.doMock("@codemirror/lang-sql", () => ({
      sql: () => [],
      PostgreSQL: {},
    }));

    vi.doMock("@uiw/react-codemirror", () => ({
      default: (props: { value: string; onChange: (value: string) => void }) => (
        <textarea
          data-testid="cm-editor"
          aria-label="SQL query"
          value={props.value}
          onChange={(event) => props.onChange(event.target.value)}
        />
      ),
      keymap: { of: () => [] },
      EditorView: { contentAttributes: { of: () => [] } },
    }));

    vi.doMock("../../api", () => ({
      executeSQL: vi.fn(),
      ApiError: class extends Error {},
    }));

    const module = await import("../SqlEditor");
    render(<module.SqlEditor />);

    expect(screen.getByLabelText("SQL query")).toBeInTheDocument();
  });
});

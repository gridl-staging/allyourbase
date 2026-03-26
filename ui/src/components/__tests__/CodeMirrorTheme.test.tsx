import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { SqlEditor } from "../SqlEditor";
import { FunctionCreate } from "../edge-functions/FunctionCreate";
import { FunctionEditor } from "../edge-functions/FunctionEditor";
import { ThemeProvider, useTheme } from "../ThemeProvider";
import type { EdgeFunctionResponse } from "../../types";

vi.mock("@uiw/react-codemirror", () => ({
  __esModule: true,
  default: (props: { theme?: string; "data-testid"?: string }) => (
    <div
      data-testid={props["data-testid"] || "codemirror-editor"}
      data-theme={props.theme || ""}
    />
  ),
  keymap: { of: () => [] },
  EditorView: { contentAttributes: { of: () => [] } },
}));

vi.mock("@codemirror/lang-sql", () => ({
  sql: () => [],
  PostgreSQL: {},
}));

vi.mock("@codemirror/lang-javascript", () => ({
  javascript: () => [],
}));

const sampleFn: EdgeFunctionResponse = {
  id: "fn_1",
  name: "hello",
  source: "export default function handler() { return new Response('ok'); }",
  compiledJs: "",
  entryPoint: "handler",
  timeout: 5_000_000_000,
  public: true,
  envVars: {},
  lastInvokedAt: null,
  createdAt: "2026-02-01T00:00:00Z",
  updatedAt: "2026-02-01T00:00:00Z",
};

describe("CodeMirror theme wiring", () => {
  beforeEach(() => {
    document.documentElement.classList.remove("dark");
    cleanup();
  });

  afterEach(() => {
    cleanup();
  });

  it("uses light CodeMirror theme by default", () => {
    render(<SqlEditor />);
    expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
      "data-theme",
      "light",
    );

    cleanup();
    render(<FunctionCreate onBack={() => {}} addToast={() => {}} />);
    expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
      "data-theme",
      "light",
    );

    cleanup();
    render(
      <FunctionEditor
        fn={sampleFn}
        onFnUpdate={() => {}}
        onDelete={() => {}}
        addToast={() => {}}
      />,
    );
    expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
      "data-theme",
      "light",
    );
  });

  it("uses dark CodeMirror theme when document has dark class", () => {
    document.documentElement.classList.add("dark");

    render(<SqlEditor />);
    expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
      "data-theme",
      "dark",
    );

    cleanup();
    render(<FunctionCreate onBack={() => {}} addToast={() => {}} />);
    expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
      "data-theme",
      "dark",
    );

    cleanup();
    render(
      <FunctionEditor
        fn={sampleFn}
        onFnUpdate={() => {}}
        onDelete={() => {}}
        addToast={() => {}}
      />,
    );
    expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
      "data-theme",
      "dark",
    );
  });

  it("updates CodeMirror theme when app theme toggles at runtime", () => {
    function ThemeToggleHarness() {
      const { toggleTheme } = useTheme();
      return (
        <>
          <button onClick={toggleTheme}>toggle-theme</button>
          <SqlEditor />
        </>
      );
    }

    render(
      <ThemeProvider>
        <ThemeToggleHarness />
      </ThemeProvider>,
    );

    expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
      "data-theme",
      "light",
    );

    fireEvent.click(screen.getByText("toggle-theme"));

    return waitFor(() => {
      expect(screen.getByTestId("codemirror-editor")).toHaveAttribute(
        "data-theme",
        "dark",
      );
    });
  });
});

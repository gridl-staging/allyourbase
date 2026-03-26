import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FunctionEditor } from "../edge-functions/FunctionEditor";
import { updateEdgeFunction, deleteEdgeFunction } from "../../api";
import type { EdgeFunctionResponse } from "../../types";

vi.mock("../../api", () => ({
  updateEdgeFunction: vi.fn(),
  deleteEdgeFunction: vi.fn(),
}));

vi.mock("@uiw/react-codemirror", () => ({
  __esModule: true,
  default: ({ value, onChange, "data-testid": testId }: {
    value: string;
    onChange?: (val: string) => void;
    "data-testid"?: string;
  }) => (
    <textarea
      data-testid={testId || "codemirror-editor"}
      value={value}
      onChange={(e) => onChange?.(e.target.value)}
    />
  ),
}));

vi.mock("@codemirror/lang-javascript", () => ({
  javascript: () => [],
}));

const mockUpdate = vi.mocked(updateEdgeFunction);
const mockDelete = vi.mocked(deleteEdgeFunction);

function makeFn(overrides: Partial<EdgeFunctionResponse> = {}): EdgeFunctionResponse {
  return {
    id: "ef_1",
    name: "test-fn",
    entryPoint: "handler",
    timeout: 5000000000,
    public: true,
    source: "export default function handler() {}",
    compiledJs: "",
    lastInvokedAt: null,
    createdAt: "2026-02-01T00:00:00Z",
    updatedAt: "2026-02-01T00:00:00Z",
    ...overrides,
  };
}

describe("FunctionEditor", () => {
  const addToast = vi.fn();
  const onFnUpdate = vi.fn();
  const onDelete = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not show dirty indicator when no changes", () => {
    render(
      <FunctionEditor fn={makeFn()} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    expect(screen.queryByTestId("dirty-indicator")).not.toBeInTheDocument();
  });

  it("shows dirty indicator when source changes", async () => {
    const user = userEvent.setup();
    render(
      <FunctionEditor fn={makeFn()} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    const editor = screen.getByTestId("codemirror-editor");
    await user.clear(editor);
    await user.type(editor, "new code");
    expect(screen.getByTestId("dirty-indicator")).toHaveTextContent("Unsaved changes");
  });

  it("shows dirty indicator when timeout changes", async () => {
    const user = userEvent.setup();
    render(
      <FunctionEditor fn={makeFn()} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    const timeout = screen.getByTestId("editor-timeout");
    await user.clear(timeout);
    await user.type(timeout, "10000");
    expect(screen.getByTestId("dirty-indicator")).toBeInTheDocument();
  });

  it("reverts changes on revert click", async () => {
    const user = userEvent.setup();
    const fn = makeFn();
    render(
      <FunctionEditor fn={fn} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    const editor = screen.getByTestId("codemirror-editor");
    await user.clear(editor);
    await user.type(editor, "modified");
    expect(screen.getByTestId("dirty-indicator")).toBeInTheDocument();

    await user.click(screen.getByTestId("revert-btn"));
    expect(screen.queryByTestId("dirty-indicator")).not.toBeInTheDocument();
    expect(screen.getByTestId("codemirror-editor")).toHaveValue(fn.source);
  });

  it("clears dirty state after successful save", async () => {
    const user = userEvent.setup();
    const fn = makeFn();
    const updatedFn = makeFn({ source: "modified" });
    mockUpdate.mockResolvedValueOnce(updatedFn);

    const { rerender } = render(
      <FunctionEditor fn={fn} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    const editor = screen.getByTestId("codemirror-editor");
    await user.clear(editor);
    await user.type(editor, "modified");
    expect(screen.getByTestId("dirty-indicator")).toBeInTheDocument();

    await user.click(screen.getByTestId("editor-save"));
    await waitFor(() => expect(onFnUpdate).toHaveBeenCalledWith(updatedFn));

    // Rerender with new fn to simulate parent state update
    rerender(
      <FunctionEditor fn={updatedFn} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    expect(screen.queryByTestId("dirty-indicator")).not.toBeInTheDocument();
  });

  it("shows inline deploy error for compile failures", async () => {
    const user = userEvent.setup();
    mockUpdate.mockRejectedValueOnce(new Error("compile error: unexpected token at line 3"));

    render(
      <FunctionEditor fn={makeFn()} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    await user.click(screen.getByTestId("editor-save"));

    await waitFor(() => {
      expect(screen.getByTestId("deploy-error")).toHaveTextContent("compile error: unexpected token at line 3");
    });
    expect(addToast).toHaveBeenCalledWith("error", "compile error: unexpected token at line 3");
  });

  it("clears deploy error when source changes", async () => {
    const user = userEvent.setup();
    mockUpdate.mockRejectedValueOnce(new Error("compile error: bad syntax"));

    render(
      <FunctionEditor fn={makeFn()} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    await user.click(screen.getByTestId("editor-save"));
    await waitFor(() => expect(screen.getByTestId("deploy-error")).toBeInTheDocument());

    const editor = screen.getByTestId("codemirror-editor");
    await user.clear(editor);
    await user.type(editor, "fixed code");
    expect(screen.queryByTestId("deploy-error")).not.toBeInTheDocument();
  });

  it("blocks save when env vars have errors", async () => {
    const user = userEvent.setup();
    const fn = makeFn({ envVars: { A: "1" } });
    render(
      <FunctionEditor fn={fn} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );

    // Add a duplicate env var by clicking "+ Add Variable" then typing the same key
    await user.click(screen.getByTestId("add-env-var"));
    const newKey = screen.getByTestId("env-key-1");
    await user.type(newKey, "A");

    await user.click(screen.getByTestId("editor-save"));
    expect(addToast).toHaveBeenCalledWith("error", "Fix environment variable errors before saving");
    expect(mockUpdate).not.toHaveBeenCalled();
  });

  it("handles delete with confirmation", async () => {
    const user = userEvent.setup();
    mockDelete.mockResolvedValueOnce(undefined);

    render(
      <FunctionEditor fn={makeFn()} onFnUpdate={onFnUpdate} onDelete={onDelete} addToast={addToast} />,
    );
    await user.click(screen.getByTestId("editor-delete"));
    await user.click(screen.getByTestId("editor-confirm-delete"));

    await waitFor(() => expect(onDelete).toHaveBeenCalled());
  });
});

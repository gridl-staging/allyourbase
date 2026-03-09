import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FunctionBrowser } from "../FunctionBrowser";
import { callRpc } from "../../api";
import type { SchemaFunction } from "../../types";

vi.mock("../../api", () => ({
  callRpc: vi.fn(),
}));

const mockCallRpc = vi.mocked(callRpc);

const sampleFunctions: Record<string, SchemaFunction> = {
  "public.add_numbers": {
    schema: "public",
    name: "add_numbers",
    comment: "Adds two integers",
    parameters: [
      { name: "a", type: "integer", position: 1 },
      { name: "b", type: "integer", position: 2 },
    ],
    returnType: "integer",
    returnsSet: false,
    isVoid: false,
  },
  "public.cleanup_old_data": {
    schema: "public",
    name: "cleanup_old_data",
    parameters: null,
    returnType: "void",
    returnsSet: false,
    isVoid: true,
  },
  "public.get_active_users": {
    schema: "public",
    name: "get_active_users",
    parameters: [{ name: "min_age", type: "integer", position: 1 }],
    returnType: "record",
    returnsSet: true,
    isVoid: false,
  },
  "stats.unnamed_param_fn": {
    schema: "stats",
    name: "unnamed_param_fn",
    parameters: [{ name: "", type: "integer", position: 1 }],
    returnType: "integer",
    returnsSet: false,
    isVoid: false,
  },
};

describe("FunctionBrowser", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders empty state when no functions", () => {
    render(<FunctionBrowser functions={{}} />);
    expect(
      screen.getByText("No functions found in the database."),
    ).toBeDefined();
  });

  it("renders function list with count", () => {
    render(<FunctionBrowser functions={sampleFunctions} />);
    expect(screen.getByText("Functions (4)")).toBeDefined();
  });

  it("shows function names", () => {
    render(<FunctionBrowser functions={sampleFunctions} />);
    expect(screen.getByText("add_numbers")).toBeDefined();
    expect(screen.getByText("cleanup_old_data")).toBeDefined();
    expect(screen.getByText("get_active_users")).toBeDefined();
    expect(screen.getByText("unnamed_param_fn")).toBeDefined();
  });

  it("shows return type info", () => {
    render(<FunctionBrowser functions={sampleFunctions} />);
    // "integer" appears twice (add_numbers + unnamed_param_fn)
    expect(screen.getAllByText("integer").length).toBe(2);
    expect(screen.getByText("void")).toBeDefined();
    expect(screen.getByText("SETOF record")).toBeDefined();
  });

  it("shows schema prefix for non-public schemas", () => {
    render(<FunctionBrowser functions={sampleFunctions} />);
    expect(screen.getByText("stats.")).toBeDefined();
  });

  it("expands function to show parameter inputs", async () => {
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("add_numbers"));

    expect(screen.getByText("Adds two integers")).toBeDefined();
    expect(screen.getByText("Parameters")).toBeDefined();
    // Parameters are rendered as labels with name + type
    const inputs = screen.getAllByRole("textbox");
    expect(inputs.length).toBe(2);
    expect(screen.getByRole("button", { name: /Execute/ })).toBeDefined();
  });

  it("shows warning for unnamed parameters", async () => {
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("unnamed_param_fn"));

    expect(
      screen.getByText(/unnamed parameters/),
    ).toBeDefined();
    // Should NOT show Execute button
    expect(screen.queryByRole("button", { name: /Execute/ })).toBeNull();
  });

  it("shows no parameter section for no-arg functions", async () => {
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("cleanup_old_data"));

    // Should show Execute button but no Parameters heading
    expect(screen.getByRole("button", { name: /Execute/ })).toBeDefined();
    expect(screen.queryByText("Parameters")).toBeNull();
  });

  it("executes function and displays result", async () => {
    mockCallRpc.mockResolvedValueOnce({ status: 200, data: 42 });
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("add_numbers"));

    const inputs = screen.getAllByRole("textbox");
    await user.type(inputs[0], "10");
    await user.type(inputs[1], "32");
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText("42")).toBeDefined();
    });

    expect(mockCallRpc).toHaveBeenCalledWith("add_numbers", { a: 10, b: 32 });
  });

  it("executes void function and shows void result", async () => {
    mockCallRpc.mockResolvedValueOnce({ status: 204, data: null });
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("cleanup_old_data"));
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText("(no return value)")).toBeDefined();
    });

    expect(mockCallRpc).toHaveBeenCalledWith("cleanup_old_data", {});
  });

  it("displays error when execution fails", async () => {
    mockCallRpc.mockRejectedValueOnce(new Error("function not found"));
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("add_numbers"));
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText("Error")).toBeDefined();
      expect(screen.getByText("function not found")).toBeDefined();
    });
  });

  it("collapses expanded function on second click", async () => {
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("add_numbers"));
    expect(screen.getByText("Adds two integers")).toBeDefined();

    await user.click(screen.getByText("add_numbers"));
    expect(screen.queryByText("Adds two integers")).toBeNull();
  });

  it("executes on Enter key in parameter input", async () => {
    mockCallRpc.mockResolvedValueOnce({ status: 200, data: 7 });
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("add_numbers"));

    const inputs = screen.getAllByRole("textbox");
    await user.type(inputs[0], "3");
    await user.type(inputs[1], "4{Enter}");

    await waitFor(() => {
      expect(screen.getByText("7")).toBeDefined();
    });

    expect(mockCallRpc).toHaveBeenCalledWith("add_numbers", { a: 3, b: 4 });
  });

  it("shows SETOF return type for set-returning functions", () => {
    render(<FunctionBrowser functions={sampleFunctions} />);
    expect(screen.getByText("SETOF record")).toBeDefined();
  });

  it("shows duration in result", async () => {
    mockCallRpc.mockResolvedValueOnce({ status: 200, data: [] });
    const user = userEvent.setup();
    render(<FunctionBrowser functions={sampleFunctions} />);

    await user.click(screen.getByText("get_active_users"));

    const inputs = screen.getAllByRole("textbox");
    await user.type(inputs[0], "21");
    await user.click(screen.getByRole("button", { name: /Execute/ }));

    await waitFor(() => {
      expect(screen.getByText("Result")).toBeDefined();
      // Duration is dynamic but should contain "ms"
      expect(screen.getByText(/\d+ms/)).toBeDefined();
    });
  });
});

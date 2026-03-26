import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InvokeTester } from "../edge-functions/InvokeTester";
import { invokeEdgeFunction, listEdgeFunctionLogs } from "../../api";
import type { EdgeFunctionResponse, EdgeFunctionInvokeResponse } from "../../types";

vi.mock("../../api", () => ({
  invokeEdgeFunction: vi.fn(),
  listEdgeFunctionLogs: vi.fn(),
}));

const mockInvoke = vi.mocked(invokeEdgeFunction);
const mockListLogs = vi.mocked(listEdgeFunctionLogs);

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

describe("InvokeTester", () => {
  const addToast = vi.fn();
  const onLogsUpdate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockListLogs.mockResolvedValue([]);
  });

  it("renders method selector, path, and send button", () => {
    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    expect(screen.getByTestId("invoke-method")).toBeInTheDocument();
    expect(screen.getByTestId("invoke-path")).toHaveValue("/test-fn");
    expect(screen.getByTestId("invoke-send")).toBeInTheDocument();
  });

  it("shows content-type selector for POST method", async () => {
    const user = userEvent.setup();
    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    await user.selectOptions(screen.getByTestId("invoke-method"), "POST");
    expect(screen.getByTestId("invoke-content-type")).toBeInTheDocument();
    expect(screen.getByTestId("invoke-body")).toBeInTheDocument();
  });

  it("hides body textarea for GET method", () => {
    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    expect(screen.queryByTestId("invoke-body")).not.toBeInTheDocument();
    expect(screen.queryByTestId("invoke-content-type")).not.toBeInTheDocument();
  });

  it("shows response with status code and duration after invoke", async () => {
    const user = userEvent.setup();
    const resp: EdgeFunctionInvokeResponse = {
      statusCode: 200,
      body: '{"ok":true}',
    };
    mockInvoke.mockResolvedValueOnce(resp);

    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    await user.click(screen.getByTestId("invoke-send"));

    await waitFor(() => {
      expect(screen.getByTestId("invoke-status-code")).toHaveTextContent("200");
      expect(screen.getByTestId("invoke-duration")).toBeInTheDocument();
      expect(screen.getByTestId("invoke-response-body")).toHaveTextContent('{"ok":true}');
    });
  });

  it("displays response headers when present", async () => {
    const user = userEvent.setup();
    const resp: EdgeFunctionInvokeResponse = {
      statusCode: 200,
      headers: { "X-Custom": ["val1"], "Content-Type": ["application/json"] },
      body: "test",
    };
    mockInvoke.mockResolvedValueOnce(resp);

    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    await user.click(screen.getByTestId("invoke-send"));

    await waitFor(() => {
      expect(screen.getByTestId("invoke-response-headers")).toBeInTheDocument();
      expect(screen.getByText("X-Custom:")).toBeInTheDocument();
    });
  });

  it("sends POST body with content-type header", async () => {
    const user = userEvent.setup();
    mockInvoke.mockResolvedValueOnce({ statusCode: 201, body: "created" });

    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    await user.selectOptions(screen.getByTestId("invoke-method"), "POST");

    const { fireEvent } = await import("@testing-library/react");
    fireEvent.change(screen.getByTestId("invoke-body"), { target: { value: '{"name":"x"}' } });

    await user.click(screen.getByTestId("invoke-send"));

    await waitFor(() => {
      expect(mockInvoke).toHaveBeenCalledWith(
        "ef_1",
        expect.objectContaining({
          method: "POST",
          body: '{"name":"x"}',
          headers: expect.objectContaining({ "Content-Type": ["application/json"] }),
        }),
      );
    });
  });

  it("shows error toast on invocation failure", async () => {
    const user = userEvent.setup();
    mockInvoke.mockRejectedValueOnce(new Error("timeout"));

    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    const sendBtn = screen.getByTestId("invoke-send");
    await user.click(sendBtn);

    await waitFor(() => {
      expect(mockInvoke).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(addToast).toHaveBeenCalledWith("error", "timeout");
    });
  });

  it("refreshes logs after successful invocation", async () => {
    const user = userEvent.setup();
    mockInvoke.mockResolvedValueOnce({ statusCode: 200, body: "ok" });

    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    await user.click(screen.getByTestId("invoke-send"));

    await waitFor(() => {
      expect(mockInvoke).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(onLogsUpdate).toHaveBeenCalled();
    });
  });

  it("adds and uses custom headers", async () => {
    const user = userEvent.setup();
    mockInvoke.mockResolvedValueOnce({ statusCode: 200, body: "ok" });

    render(<InvokeTester fn={makeFn()} onLogsUpdate={onLogsUpdate} addToast={addToast} />);
    await user.click(screen.getByTestId("invoke-add-header"));
    await user.type(screen.getByTestId("invoke-header-key-0"), "Authorization");
    await user.type(screen.getByTestId("invoke-header-value-0"), "Bearer tok");

    await user.click(screen.getByTestId("invoke-send"));

    await waitFor(() => {
      expect(mockInvoke).toHaveBeenCalledWith(
        "ef_1",
        expect.objectContaining({
          headers: expect.objectContaining({ Authorization: ["Bearer tok"] }),
        }),
      );
    });
  });
});

import { useState } from "react";
import { vi, describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FunctionLogs } from "../edge-functions/FunctionLogs";
import { listEdgeFunctionLogs } from "../../api";
import type { EdgeFunctionLogEntry } from "../../types";

vi.mock("../../api", () => ({
  listEdgeFunctionLogs: vi.fn(),
}));

const mockListLogs = vi.mocked(listEdgeFunctionLogs);

function makeLog(overrides: Partial<EdgeFunctionLogEntry> = {}): EdgeFunctionLogEntry {
  return {
    id: "log_1",
    functionId: "ef_1",
    invocationId: "inv_1",
    status: "success",
    durationMs: 42,
    requestMethod: "GET",
    requestPath: "/test",
    createdAt: "2026-02-01T12:00:00Z",
    ...overrides,
  };
}

describe("FunctionLogs", () => {
  const onLogsUpdate = vi.fn();
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;
  const initialFetchQuery = {
    page: 1,
    perPage: 20,
    status: undefined,
    trigger_type: undefined,
  };

  async function renderAndWaitForInitialFetch(ui: JSX.Element) {
    render(ui);
    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalledWith("ef_1", initialFetchQuery);
    });
    await waitFor(() => {
      expect(onLogsUpdate).toHaveBeenCalled();
    });
  }

  beforeEach(() => {
    vi.clearAllMocks();
    mockListLogs.mockResolvedValue([]);
    consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    const actWarnings = consoleErrorSpy.mock.calls
      .map((args) => args.map((arg) => String(arg)).join(" "))
      .filter((line) => line.includes("not wrapped in act"));
    expect(actWarnings).toHaveLength(0);
    consoleErrorSpy.mockRestore();
  });

  it("fetches latest logs on mount", async () => {
    const latest = [makeLog({ id: "fresh", requestPath: "/fresh" })];
    mockListLogs.mockResolvedValue(latest);

    render(<FunctionLogs functionId="ef_1" logs={[]} onLogsUpdate={onLogsUpdate} />);

    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalledWith("ef_1", initialFetchQuery);
    });
    await waitFor(() => {
      expect(onLogsUpdate).toHaveBeenCalledWith(latest);
    });
  });

  it("shows empty state when no logs", async () => {
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={[]} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByTestId("logs-empty")).toHaveTextContent("No execution logs yet.");
  });

  it("renders log rows with correct data", async () => {
    const logs = [
      makeLog({ id: "l1", durationMs: 10, requestMethod: "GET", requestPath: "/a" }),
      makeLog({ id: "l2", durationMs: 500, requestMethod: "POST", requestPath: "/b", status: "error" }),
    ];
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByText("10ms")).toBeInTheDocument();
    expect(screen.getByText("500ms")).toBeInTheDocument();
    expect(screen.getByText("/a")).toBeInTheDocument();
    expect(screen.getByText("/b")).toBeInTheDocument();
  });

  it("exposes method and path cells with stable row-scoped test IDs", async () => {
    const logs = [
      makeLog({ id: "l1", requestMethod: "POST", requestPath: "/db-event" }),
    ];
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );

    expect(screen.getByTestId("log-method-l1")).toHaveTextContent("POST");
    expect(screen.getByTestId("log-path-l1")).toHaveTextContent("/db-event");
  });

  it("displays trigger type badge when present", async () => {
    const logs = [
      makeLog({ id: "l1", triggerType: "http" }),
      makeLog({ id: "l2", triggerType: "cron" }),
    ];
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByTestId("log-trigger-l1")).toHaveTextContent("http");
    expect(screen.getByTestId("log-trigger-l2")).toHaveTextContent("cron");
  });

  it("shows dash when trigger type is absent", async () => {
    const logs = [makeLog({ id: "l1", triggerType: undefined })];
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.queryByTestId("log-trigger-l1")).not.toBeInTheDocument();
  });

  it("expands row to show stdout/error and trigger metadata", async () => {
    const user = userEvent.setup();
    const logs = [
      makeLog({
        id: "l1",
        stdout: "output text",
        error: "err text",
        triggerType: "db",
        triggerId: "trg_1",
        parentInvocationId: "inv_parent",
      }),
    ];
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.queryByText("output text")).not.toBeInTheDocument();

    await user.click(screen.getByTestId("log-row-l1"));
    expect(screen.getByText("output text")).toBeInTheDocument();
    expect(screen.getByText("err text")).toBeInTheDocument();
    expect(screen.getByText(/trg_1/)).toBeInTheDocument();
    expect(screen.getByText(/inv_parent/)).toBeInTheDocument();
  });

  it("renders status filter dropdown", async () => {
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={[makeLog()]} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByTestId("log-status-filter")).toBeInTheDocument();
  });

  it("renders trigger type filter dropdown", async () => {
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={[makeLog()]} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByTestId("log-trigger-filter")).toBeInTheDocument();
  });

  it("fetches filtered logs on status filter change", async () => {
    const user = userEvent.setup();
    const logs = [makeLog()];

    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );
    mockListLogs.mockClear();
    await user.selectOptions(screen.getByTestId("log-status-filter"), "error");

    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalledWith("ef_1", expect.objectContaining({ status: "error" }));
    });
  });

  it("fetches filtered logs on trigger filter change", async () => {
    const user = userEvent.setup();

    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={[makeLog()]} onLogsUpdate={onLogsUpdate} />,
    );
    mockListLogs.mockClear();
    await user.selectOptions(screen.getByTestId("log-trigger-filter"), "cron");

    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalledWith("ef_1", expect.objectContaining({ trigger_type: "cron" }));
    });
  });

  it("shows pagination controls", async () => {
    // 20 logs means there might be more
    const logs = Array.from({ length: 20 }, (_, i) => makeLog({ id: `l${i}` }));
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByTestId("log-prev-page")).toBeInTheDocument();
    expect(screen.getByTestId("log-next-page")).toBeInTheDocument();
    expect(screen.getByTestId("log-page-info")).toHaveTextContent("Page 1");
  });

  it("disables previous on first page", async () => {
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={[makeLog()]} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByTestId("log-prev-page")).toBeDisabled();
  });

  it("navigates to next page", async () => {
    const user = userEvent.setup();
    const logs = Array.from({ length: 20 }, (_, i) => makeLog({ id: `l${i}` }));
    mockListLogs
      .mockResolvedValueOnce(logs)
      .mockResolvedValueOnce(logs.slice(0, 5));

    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={onLogsUpdate} />,
    );
    mockListLogs.mockClear();
    await user.click(screen.getByTestId("log-next-page"));

    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalledWith("ef_1", expect.objectContaining({ page: 2 }));
    });
  });

  it("shows no-match message when filters return empty", async () => {
    const user = userEvent.setup();
    mockListLogs.mockResolvedValue([]);
    const onWrapperLogsUpdate = vi.fn();

    // Use a stateful wrapper so the component re-renders with empty logs
    function Wrapper() {
      const [logs, setLogs] = useState([makeLog()]);
      const handleLogsUpdate = (nextLogs: EdgeFunctionLogEntry[]) => {
        onWrapperLogsUpdate(nextLogs);
        setLogs(nextLogs);
      };
      return <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={handleLogsUpdate} />;
    }

    render(<Wrapper />);
    await waitFor(() => {
      expect(onWrapperLogsUpdate).toHaveBeenCalled();
    });
    mockListLogs.mockClear();
    await user.selectOptions(screen.getByTestId("log-status-filter"), "error");

    await waitFor(() => {
      expect(screen.getByTestId("logs-no-match")).toHaveTextContent(
        "No matching logs for the selected filters.",
      );
    });
  });

  it("shows Trigger column header", async () => {
    await renderAndWaitForInitialFetch(
      <FunctionLogs functionId="ef_1" logs={[makeLog()]} onLogsUpdate={onLogsUpdate} />,
    );
    expect(screen.getByText("Trigger")).toBeInTheDocument();
  });

  it("polls and renders logs that appear after initial empty fetch", async () => {
    const delayedLog = makeLog({
      id: "delayed-db-log",
      requestPath: "/db-event",
      triggerType: "db",
    });
    mockListLogs
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([delayedLog])
      .mockResolvedValue([delayedLog]);

    function Wrapper() {
      const [logs, setLogs] = useState<EdgeFunctionLogEntry[]>([]);
      return <FunctionLogs functionId="ef_1" logs={logs} onLogsUpdate={setLogs} />;
    }

    render(<Wrapper />);

    await waitFor(() => {
      expect(screen.getByTestId("logs-empty")).toBeInTheDocument();
    });
    expect(mockListLogs).toHaveBeenCalledTimes(1);

    await waitFor(() => {
      expect(screen.getByText("/db-event")).toBeInTheDocument();
    }, { timeout: 3500 });
    expect(mockListLogs.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("does not crash when API returns non-array data", async () => {
    // Simulate API returning undefined (e.g. network issue, malformed response)
    mockListLogs.mockResolvedValue(undefined as unknown as EdgeFunctionLogEntry[]);

    const { container } = render(
      <FunctionLogs functionId="ef_1" logs={[]} onLogsUpdate={onLogsUpdate} />,
    );

    // Wait for the fetch to complete
    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalled();
    });

    // Component should still be rendered (not crashed)
    expect(container.querySelector('[data-testid="function-logs"]')).toBeInTheDocument();
    // onLogsUpdate should NOT have been called with non-array data
    for (const call of onLogsUpdate.mock.calls) {
      expect(Array.isArray(call[0])).toBe(true);
    }
  });

  it("does not start a polling request while initial fetch is still in flight", async () => {
    mockListLogs.mockImplementation(() => new Promise(() => {}));

    render(<FunctionLogs functionId="ef_1" logs={[]} onLogsUpdate={onLogsUpdate} />);

    await waitFor(() => {
      expect(mockListLogs).toHaveBeenCalledTimes(1);
    });

    await new Promise((resolve) => setTimeout(resolve, 2300));
    expect(mockListLogs).toHaveBeenCalledTimes(1);
  });
});

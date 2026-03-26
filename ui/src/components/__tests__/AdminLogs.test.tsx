import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, fireEvent, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders } from "../../test-utils";
import { AdminLogs } from "../AdminLogs";
import type { AdminLogEntry, AdminLogsResult } from "../../types/logs";

vi.mock("../../api_logs", () => ({
  listAdminLogs: vi.fn(),
}));

import * as api from "../../api_logs";
const originalURL = globalThis.URL;

function makeLogsResult(overrides: Partial<AdminLogsResult> = {}): AdminLogsResult {
  return {
    entries: [
      {
        id: "e-1",
        time: "2026-03-15T10:00:00Z",
        parsedTimeMs: Date.parse("2026-03-15T10:00:00Z"),
        level: "info",
        levelLabel: "INFO",
        message: "boot complete",
        attrs: { path: "/api/admin/status", status: 200 },
        attrsText: '{"path":"/api/admin/status","status":200}',
        searchText: '2026-03-15t10:00:00z info boot complete {"path":"/api/admin/status","status":200}',
      },
      {
        id: "e-2",
        time: "2026-03-15T11:00:00Z",
        parsedTimeMs: Date.parse("2026-03-15T11:00:00Z"),
        level: "error",
        levelLabel: "ERROR",
        message: "request failed",
        attrs: { path: "/api/admin/sql", status: 500 },
        attrsText: '{"path":"/api/admin/sql","status":500}',
        searchText: '2026-03-15t11:00:00z error request failed {"path":"/api/admin/sql","status":500}',
      },
    ],
    bufferingEnabled: true,
    ...overrides,
  };
}

function toDateTimeLocalValue(timestamp: string): string {
  const date = new Date(timestamp);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function toCopiedLogText(entry: AdminLogEntry): string {
  return JSON.stringify(
    {
      id: entry.id,
      time: entry.time,
      level: entry.level,
      levelLabel: entry.levelLabel,
      message: entry.message,
      attrs: entry.attrs,
      attrsText: entry.attrsText,
    },
    null,
    2,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  (api.listAdminLogs as ReturnType<typeof vi.fn>).mockResolvedValue(makeLogsResult());
});

afterEach(() => {
  vi.useRealTimers();
  (globalThis as { URL: typeof URL }).URL = originalURL;
});

describe("AdminLogs", () => {
  it("renders loading state then log rows", async () => {
    let resolveResult: ((value: AdminLogsResult) => void) | undefined;
    (api.listAdminLogs as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      new Promise<AdminLogsResult>((resolve) => {
        resolveResult = resolve;
      }),
    );

    renderWithProviders(<AdminLogs />);

    expect(screen.getByText(/loading admin logs/i)).toBeInTheDocument();
    expect(screen.queryByText(/no log entries found/i)).not.toBeInTheDocument();
    resolveResult?.(makeLogsResult());

    await expect(screen.findByText("boot complete")).resolves.toBeInTheDocument();
    expect(screen.getByText("request failed")).toBeInTheDocument();
  });

  it("renders error state when loading fails", async () => {
    (api.listAdminLogs as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("network down"));

    renderWithProviders(<AdminLogs />);

    await expect(screen.findByText(/network down/i)).resolves.toBeInTheDocument();
    expect(screen.queryByText(/no log entries found/i)).not.toBeInTheDocument();
  });

  it("renders buffering-disabled message from backend response", async () => {
    (api.listAdminLogs as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      makeLogsResult({
        entries: [],
        bufferingEnabled: false,
        message: "log buffering not enabled",
      }),
    );

    renderWithProviders(<AdminLogs />);

    await expect(screen.findByText(/log buffering not enabled/i)).resolves.toBeInTheDocument();
    expect(screen.getByText(/no log entries found/i)).toBeInTheDocument();
  });

  it("applies local search/level/time filters without refetching", async () => {
    const user = userEvent.setup();
    renderWithProviders(<AdminLogs />);

    await expect(screen.findByText("boot complete")).resolves.toBeInTheDocument();
    expect(api.listAdminLogs).toHaveBeenCalledTimes(1);

    await user.type(screen.getByLabelText("Search logs"), "request");
    expect(screen.queryByText("boot complete")).not.toBeInTheDocument();
    expect(screen.getByText("request failed")).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Level"), "error");
    expect(screen.queryByText("boot complete")).not.toBeInTheDocument();

    const requestFailedTime = toDateTimeLocalValue("2026-03-15T11:00:00Z");
    const bootCompleteTime = toDateTimeLocalValue("2026-03-15T10:00:00Z");

    fireEvent.change(screen.getByLabelText("From"), {
      target: { value: requestFailedTime },
    });
    expect(screen.getByText("request failed")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("To"), {
      target: { value: bootCompleteTime },
    });
    expect(screen.queryByText("request failed")).not.toBeInTheDocument();
    expect(screen.getByText(/no log entries found/i)).toBeInTheDocument();

    expect(api.listAdminLogs).toHaveBeenCalledTimes(1);
  });

  it("supports nested attrs inspection with empty attrs fallback and searchText filtering", async () => {
    const user = userEvent.setup();
    const nestedEntry: AdminLogEntry = {
      id: "nested-1",
      time: "2026-03-15T12:00:00Z",
      parsedTimeMs: Date.parse("2026-03-15T12:00:00Z"),
      level: "info",
      levelLabel: "INFO",
      message: "nested payload",
      attrs: {
        request: {
          traceId: "trace-id-9",
          roles: ["admin", "auditor"],
        },
        retry: [1, 2],
      },
      attrsText:
        '{"request":{"roles":["admin","auditor"],"traceId":"trace-id-9"},"retry":[1,2]}',
      searchText:
        '2026-03-15t12:00:00z info nested payload {"request":{"roles":["admin","auditor"],"traceId":"trace-id-9"},"retry":[1,2]}',
    };
    const emptyAttrsEntry: AdminLogEntry = {
      id: "empty-1",
      time: "2026-03-15T09:00:00Z",
      parsedTimeMs: Date.parse("2026-03-15T09:00:00Z"),
      level: "warn",
      levelLabel: "WARN",
      message: "empty attrs entry",
      attrs: {},
      attrsText: "{}",
      searchText: "2026-03-15t09:00:00z warn empty attrs entry {}",
    };

    (api.listAdminLogs as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      makeLogsResult({
        entries: [nestedEntry, emptyAttrsEntry],
      }),
    );

    renderWithProviders(<AdminLogs />);

    await expect(screen.findByText("nested payload")).resolves.toBeInTheDocument();
    expect(screen.getByTestId("admin-log-attrs-summary-empty-1")).toHaveTextContent("-");

    await user.type(screen.getByLabelText("Search logs"), "trace-id-9");
    expect(screen.getByText("nested payload")).toBeInTheDocument();
    expect(screen.queryByText("empty attrs entry")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Inspect attrs nested-1" }));

    const inspectJson = screen.getByTestId("admin-log-attrs-json-nested-1");
    expect(inspectJson).toHaveTextContent('"traceId": "trace-id-9"');
    expect(inspectJson).toHaveTextContent('"roles": [');
  });

  it("copies a row and exports the current filtered view using normalized filtered order", async () => {
    const user = userEvent.setup();
    const blobSpy = vi.spyOn(globalThis, "Blob");
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      writable: true,
      configurable: true,
    });

    const createObjectURL = vi.fn(() => "blob:test");
    const revokeObjectURL = vi.fn();
    (globalThis as Record<string, unknown>).URL = { createObjectURL, revokeObjectURL };

    renderWithProviders(<AdminLogs />);

    await expect(screen.findByText("boot complete")).resolves.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Copy log e-2" }));
    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith(
      toCopiedLogText(makeLogsResult().entries[1] as AdminLogEntry),
    );

    await user.click(screen.getByRole("button", { name: /export filtered json/i }));

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    const firstBlobCall = (blobSpy.mock.calls as unknown[][])[0];
    const firstBlobParts = firstBlobCall[0] as unknown[];
    const firstBlobOptions = firstBlobCall[1] as { type?: string };
    expect(firstBlobOptions.type).toBe("application/json");
    const firstExported = JSON.parse(String(firstBlobParts[0])) as AdminLogEntry[];
    expect(firstExported.map((entry) => entry.id)).toEqual(["e-2", "e-1"]);
    expect(firstExported.map((entry) => entry.message)).toEqual(["request failed", "boot complete"]);

    await user.type(screen.getByLabelText("Search logs"), "request");
    expect(screen.getByText("request failed")).toBeInTheDocument();
    expect(screen.queryByText("boot complete")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /export filtered json/i }));

    expect(createObjectURL).toHaveBeenCalledTimes(2);
    const secondBlobCall = (blobSpy.mock.calls as unknown[][])[1];
    const secondBlobParts = secondBlobCall[0] as unknown[];
    const secondExported = JSON.parse(String(secondBlobParts[0])) as AdminLogEntry[];
    expect(secondExported.map((entry) => entry.id)).toEqual(["e-2"]);
    expect(secondExported.map((entry) => entry.message)).toEqual(["request failed"]);
    expect(revokeObjectURL).toHaveBeenCalledTimes(2);
    expect(revokeObjectURL).toHaveBeenNthCalledWith(1, "blob:test");
    expect(revokeObjectURL).toHaveBeenNthCalledWith(2, "blob:test");
    blobSpy.mockRestore();
  });

  it("shows an error toast when export creation fails", async () => {
    const user = userEvent.setup();
    const createObjectURL = vi.fn(() => {
      throw new Error("blob failure");
    });
    const revokeObjectURL = vi.fn();
    (globalThis as Record<string, unknown>).URL = { createObjectURL, revokeObjectURL };

    renderWithProviders(<AdminLogs />);

    await expect(screen.findByText("boot complete")).resolves.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /export filtered json/i }));

    await expect(screen.findByText("Failed to export logs")).resolves.toBeInTheDocument();
    expect(screen.queryByText("Exported 2 log row(s)")).not.toBeInTheDocument();
    expect(revokeObjectURL).not.toHaveBeenCalled();
  });

  it("pauses interval polling, allows one-shot refresh while paused, and resumes polling", async () => {
    vi.useFakeTimers();

    renderWithProviders(<AdminLogs pollMs={25} />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(api.listAdminLogs).toHaveBeenCalledTimes(1);
    expect(screen.getByText("boot complete")).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /pause auto-refresh/i }));
    });
    expect(screen.getByTestId("admin-logs-polling-status")).toHaveTextContent("Auto-refresh paused");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(75);
    });
    expect(api.listAdminLogs).toHaveBeenCalledTimes(1);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /^refresh$/i }));
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(api.listAdminLogs).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("button", { name: /resume auto-refresh/i })).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(50);
    });
    expect(api.listAdminLogs).toHaveBeenCalledTimes(2);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /resume auto-refresh/i }));
      await vi.advanceTimersByTimeAsync(25);
    });
    expect(api.listAdminLogs).toHaveBeenCalledTimes(3);
  });
});

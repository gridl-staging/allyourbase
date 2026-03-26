import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { RealtimeInspectorSnapshot } from "../../types";
import { RealtimeInspector } from "../RealtimeInspector";

vi.mock("../../api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api")>();
  return { ...actual, getRealtimeInspectorSnapshot: vi.fn() };
});

import { getRealtimeInspectorSnapshot } from "../../api";

const mockedGet = vi.mocked(getRealtimeInspectorSnapshot);

function makeSnapshot(): RealtimeInspectorSnapshot {
  return {
    version: "v1",
    timestamp: "2026-03-15T00:00:00Z",
    connections: { sse: 3, ws: 7, total: 10 },
    subscriptions: [
      { name: "public_posts", type: "table", count: 8 },
      { name: "room:lobby", type: "broadcast", count: 4 },
      { name: "room:lobby", type: "presence", count: 2 },
    ],
    counters: { droppedMessages: 5, heartbeatFailures: 1 },
  };
}

describe("RealtimeInspector", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedGet.mockResolvedValue(makeSnapshot());
  });

  it("renders connection cards with SSE/WS/Total breakdown", async () => {
    render(<RealtimeInspector />);

    await expect(screen.findByTestId("realtime-total-metric-value")).resolves.toHaveTextContent("10");
    expect(screen.getByTestId("realtime-sse-metric-value")).toHaveTextContent("3");
    expect(screen.getByTestId("realtime-ws-metric-value")).toHaveTextContent("7");
  });

  it("renders counter metrics (dropped messages, heartbeat failures)", async () => {
    render(<RealtimeInspector />);

    await expect(screen.findByTestId("realtime-dropped-metric-value")).resolves.toHaveTextContent("5");
    expect(screen.getByTestId("realtime-heartbeat-failures-metric-value")).toHaveTextContent("1");
  });

  it("renders subscription rows with type labels", async () => {
    render(<RealtimeInspector />);

    await expect(screen.findByText("public_posts")).resolves.toBeInTheDocument();
    expect(screen.getAllByText("room:lobby")).toHaveLength(2);
    expect(screen.getAllByText("table")).toHaveLength(1);
    expect(screen.getAllByText("broadcast")).toHaveLength(1);
  });

  it("does not render dead mock-only UI (range selector, throughput, auth/anon)", async () => {
    render(<RealtimeInspector />);

    await expect(screen.findByTestId("realtime-total-metric-value")).resolves.toHaveTextContent("10");
    // No time-range selector
    expect(screen.queryByLabelText("Window")).not.toBeInTheDocument();
    expect(screen.queryByText("15m")).not.toBeInTheDocument();
    // No throughput section
    expect(screen.queryByText("Event Throughput")).not.toBeInTheDocument();
    // No auth/anon/churn labels
    expect(screen.queryByText("Authenticated")).not.toBeInTheDocument();
    expect(screen.queryByText("Anonymous")).not.toBeInTheDocument();
    expect(screen.queryByText("Churn/min")).not.toBeInTheDocument();
  });

  it("supports manual refresh and polling", async () => {
    const user = userEvent.setup();
    render(<RealtimeInspector pollMs={25} />);

    await screen.findByText("Realtime Inspector");
    await user.click(screen.getByRole("button", { name: /Refresh/i }));

    await waitFor(() => expect(mockedGet.mock.calls.length).toBeGreaterThanOrEqual(2));
    await waitFor(() => expect(mockedGet.mock.calls.length).toBeGreaterThanOrEqual(3));
  });

  it("does not show an empty state before the first snapshot resolves", async () => {
    let resolveSnapshot: ((snapshot: RealtimeInspectorSnapshot) => void) | undefined;
    mockedGet.mockReturnValueOnce(
      new Promise<RealtimeInspectorSnapshot>((resolve) => {
        resolveSnapshot = resolve;
      }),
    );

    render(<RealtimeInspector />);

    expect(screen.getByText(/Loading realtime telemetry/i)).toBeInTheDocument();
    expect(screen.queryByText(/No active subscriptions/i)).not.toBeInTheDocument();

    resolveSnapshot?.(makeSnapshot());
    await expect(screen.findByTestId("realtime-total-metric-value")).resolves.toHaveTextContent("10");
  });

  it("filters subscription rows by name", async () => {
    const user = userEvent.setup();
    render(<RealtimeInspector />);

    await expect(screen.findByText("public_posts")).resolves.toBeInTheDocument();
    const filterInput = screen.getByPlaceholderText("Filter subscriptions");
    await user.type(filterInput, "room");

    expect(screen.getAllByText("room:lobby")).toHaveLength(2);
    expect(screen.queryByText("public_posts")).not.toBeInTheDocument();
  });

  it("renders empty subscription state", async () => {
    mockedGet.mockResolvedValueOnce({
      ...makeSnapshot(),
      subscriptions: [],
    });

    render(<RealtimeInspector />);

    await expect(screen.findByText(/No active subscriptions/i)).resolves.toBeInTheDocument();
  });

  it("renders error state", async () => {
    mockedGet.mockRejectedValueOnce(new Error("network failure"));

    render(<RealtimeInspector />);

    await screen.findByText(/network failure/i);
    expect(screen.queryByText(/No active subscriptions/i)).not.toBeInTheDocument();
  });
});

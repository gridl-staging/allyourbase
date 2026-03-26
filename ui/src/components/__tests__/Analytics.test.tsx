import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Analytics } from "../Analytics";

vi.mock("../../api_analytics", () => ({
  listRequestLogs: vi.fn(),
  listQueryStats: vi.fn(),
}));

import * as api from "../../api_analytics";

const mockRequestLogs = {
  items: [
    {
      id: "r-1",
      timestamp: "2026-03-12T14:00:00Z",
      method: "GET",
      path: "/api/users",
      status_code: 200,
      duration_ms: 45,
      request_size: 0,
      response_size: 1024,
    },
    {
      id: "r-2",
      timestamp: "2026-03-12T14:01:00Z",
      method: "POST",
      path: "/api/records",
      status_code: 500,
      duration_ms: 150,
      request_size: 256,
      response_size: 64,
    },
  ],
  count: 2,
  limit: 100,
  offset: 0,
};

const mockQueryStats = {
  items: [
    {
      queryid: "q-1",
      query: "SELECT * FROM users",
      calls: 1000,
      total_exec_time: 5000.5,
      mean_exec_time: 5.0,
      rows: 50000,
      shared_blks_hit: 8000,
      shared_blks_read: 200,
      index_suggestions: [
        { statement: "CREATE INDEX idx_users_email ON users(email)", confidence: "high" },
      ],
    },
  ],
  count: 1,
  limit: 50,
  sort: "total_time",
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.listRequestLogs as ReturnType<typeof vi.fn>).mockResolvedValue(mockRequestLogs);
  (api.listQueryStats as ReturnType<typeof vi.fn>).mockResolvedValue(mockQueryStats);
});

describe("Analytics", () => {
  it("renders request log entries with method/path/status/duration", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/users")).toBeInTheDocument();
    });
    expect(screen.getByText("200")).toBeInTheDocument();
    expect(screen.getByText("45ms")).toBeInTheDocument();
    expect(screen.getByText("500")).toBeInTheDocument();
  });

  it("switches to query performance tab", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/users")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /query performance/i }));
    await waitFor(() => {
      expect(screen.getByText("SELECT * FROM users")).toBeInTheDocument();
    });
  });

  it("applies request-log filters including status code and date range", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/users")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Method"), {
      target: { value: "GET" },
    });
    fireEvent.change(screen.getByLabelText("Path"), {
      target: { value: "/api/users*" },
    });
    fireEvent.change(screen.getByLabelText("Status Code"), {
      target: { value: "200" },
    });
    fireEvent.change(screen.getByLabelText("From"), {
      target: { value: "2026-03-01" },
    });
    fireEvent.change(screen.getByLabelText("To"), {
      target: { value: "2026-03-12" },
    });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenLastCalledWith({
        method: "GET",
        path: "/api/users*",
        status: "200",
        from: "2026-03-01",
        to: "2026-03-12",
      });
    });
  });

  it("does not reload request logs until Apply is clicked", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(screen.getByLabelText("Method"), {
      target: { value: "GET" },
    });
    expect(api.listRequestLogs).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));
    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenCalledTimes(2);
    });
    expect(api.listRequestLogs).toHaveBeenLastCalledWith({
      method: "GET",
      path: undefined,
      status: undefined,
      from: undefined,
      to: undefined,
    });
  });

  it("updates query stats sort parameter when sort dropdown changes", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/users")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /query performance/i }));
    await waitFor(() => {
      expect(api.listQueryStats).toHaveBeenLastCalledWith({ sort: "total_time" });
    });

    fireEvent.change(screen.getByLabelText("Sort by"), {
      target: { value: "calls" },
    });
    await waitFor(() => {
      expect(api.listQueryStats).toHaveBeenLastCalledWith({ sort: "calls" });
    });
  });

  it("displays index suggestions in query stats tab", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/users")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /query performance/i }));
    await waitFor(() => {
      expect(
        screen.getByText("CREATE INDEX idx_users_email ON users(email)"),
      ).toBeInTheDocument();
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listRequestLogs as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Failed to load"),
    );
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("Failed to load")).toBeInTheDocument();
    });
  });
});

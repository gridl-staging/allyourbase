import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { AuditLogs } from "../AuditLogs";

vi.mock("../../api_audit", () => ({
  listAuditLogs: vi.fn(),
}));

import * as api from "../../api_audit";

const mockAuditResponse = {
  items: [
    {
      id: "a-1",
      timestamp: "2026-03-12T14:00:00Z",
      user_id: "user-1",
      table_name: "users",
      operation: "INSERT",
      old_values: null,
      new_values: { email: "test@example.com" },
      ip_address: "192.168.1.1",
    },
    {
      id: "a-2",
      timestamp: "2026-03-12T14:05:00Z",
      user_id: "user-2",
      api_key_id: "key-1",
      table_name: "orders",
      record_id: "42",
      operation: "UPDATE",
      old_values: { status: "pending" },
      new_values: { status: "shipped" },
      ip_address: "10.0.0.1",
    },
  ],
  count: 2,
  limit: 100,
  offset: 0,
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.listAuditLogs as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockAuditResponse,
  );
});

describe("AuditLogs", () => {
  it("renders entries with operation/table_name/timestamp/user_id columns", async () => {
    renderWithProviders(<AuditLogs />);
    await waitFor(() => {
      expect(screen.getByText("users")).toBeInTheDocument();
    });
    expect(screen.getByText("orders")).toBeInTheDocument();
    expect(screen.getByText("user-1")).toBeInTheDocument();
    expect(screen.getByText("user-2")).toBeInTheDocument();
    // Operation values appear in both table cells and filter dropdown — use getAllByText
    expect(screen.getAllByText("INSERT").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("UPDATE").length).toBeGreaterThanOrEqual(1);
  });

  it("applies filters only on Apply click (useDraftFilters pattern)", async () => {
    renderWithProviders(<AuditLogs />);
    await waitFor(() => {
      expect(api.listAuditLogs).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(screen.getByLabelText("Table"), {
      target: { value: "users" },
    });
    expect(api.listAuditLogs).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));
    await waitFor(() => {
      expect(api.listAuditLogs).toHaveBeenCalledTimes(2);
    });
  });

  it("sends filter params when filters are applied", async () => {
    renderWithProviders(<AuditLogs />);
    await waitFor(() => {
      expect(screen.getByText("INSERT")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Table"), {
      target: { value: "orders" },
    });
    fireEvent.change(screen.getByLabelText("Operation"), {
      target: { value: "UPDATE" },
    });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    await waitFor(() => {
      expect(api.listAuditLogs).toHaveBeenLastCalledWith(
        expect.objectContaining({
          table: "orders",
          operation: "UPDATE",
        }),
      );
    });
  });

  it("navigates pages with pagination controls", async () => {
    (api.listAuditLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...mockAuditResponse,
      count: 200,
      limit: 100,
      offset: 0,
    });
    renderWithProviders(<AuditLogs />);
    await waitFor(() => {
      expect(screen.getByText("INSERT")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => {
      expect(api.listAuditLogs).toHaveBeenLastCalledWith(
        expect.objectContaining({ offset: 100 }),
      );
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listAuditLogs as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Connection refused"),
    );
    renderWithProviders(<AuditLogs />);
    await waitFor(() => {
      expect(screen.getByText("Connection refused")).toBeInTheDocument();
    });
  });
});

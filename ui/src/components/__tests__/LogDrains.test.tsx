import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { LogDrains } from "../LogDrains";

vi.mock("../../api_drains", () => ({
  listDrains: vi.fn(),
  createDrain: vi.fn(),
  deleteDrain: vi.fn(),
}));

import * as api from "../../api_drains";

const mockDrains = [
  {
    id: "drain-1",
    name: "datadog-prod",
    stats: { sent: 15000, failed: 12, dropped: 3 },
  },
  {
    id: "drain-2",
    name: "loki-dev",
    stats: { sent: 500, failed: 0, dropped: 0 },
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  (api.listDrains as ReturnType<typeof vi.fn>).mockResolvedValue(mockDrains);
  (api.createDrain as ReturnType<typeof vi.fn>).mockResolvedValue(mockDrains[0]);
  (api.deleteDrain as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
});

describe("LogDrains", () => {
  it("renders drain list with name and sent/failed/dropped stats", async () => {
    renderWithProviders(<LogDrains />);
    await waitFor(() => {
      expect(screen.getByText("datadog-prod")).toBeInTheDocument();
    });
    expect(screen.getByText("loki-dev")).toBeInTheDocument();
    expect(screen.getByText("15000")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
  });

  it("creates drain via form", async () => {
    renderWithProviders(<LogDrains />);
    await waitFor(() => {
      expect(screen.getByText("datadog-prod")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create drain/i }));
    fireEvent.change(screen.getByLabelText("Type"), { target: { value: "http" } });
    fireEvent.change(screen.getByLabelText("URL"), { target: { value: "https://logs.example.com" } });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createDrain).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "http",
          url: "https://logs.example.com",
        }),
      );
    });
  });

  it("passes optional headers JSON through to createDrain", async () => {
    renderWithProviders(<LogDrains />);
    await waitFor(() => {
      expect(screen.getByText("datadog-prod")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create drain/i }));
    fireEvent.change(screen.getByLabelText("URL"), { target: { value: "https://logs.example.com" } });
    fireEvent.change(screen.getByLabelText("Headers (JSON, optional)"), {
      target: { value: '{"Authorization":"Bearer token","X-Scope-OrgID":"tenant1"}' },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createDrain).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "https://logs.example.com",
          headers: {
            Authorization: "Bearer token",
            "X-Scope-OrgID": "tenant1",
          },
        }),
      );
    });
  });

  it("shows validation error for invalid headers JSON", async () => {
    renderWithProviders(<LogDrains />);
    await waitFor(() => {
      expect(screen.getByText("datadog-prod")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create drain/i }));
    fireEvent.change(screen.getByLabelText("URL"), { target: { value: "https://logs.example.com" } });
    fireEvent.change(screen.getByLabelText("Headers (JSON, optional)"), {
      target: { value: "{invalid json" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    expect(screen.getByText("Headers must be valid JSON")).toBeInTheDocument();
    expect(api.createDrain).not.toHaveBeenCalled();
  });

  it("fires ConfirmDialog on delete", async () => {
    renderWithProviders(<LogDrains />);
    await waitFor(() => {
      expect(screen.getByText("datadog-prod")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByLabelText(/^Delete /);
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /delete drain/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    await waitFor(() => {
      expect(api.deleteDrain).toHaveBeenCalledWith("drain-1");
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listDrains as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Unavailable"));
    renderWithProviders(<LogDrains />);
    await waitFor(() => {
      expect(screen.getByText("Unavailable")).toBeInTheDocument();
    });
  });
});

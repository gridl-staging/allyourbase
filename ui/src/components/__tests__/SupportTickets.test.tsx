import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { SupportTickets } from "../SupportTickets";

vi.mock("../../api_support", () => ({
  adminListTickets: vi.fn(),
  adminGetTicket: vi.fn(),
  adminUpdateTicket: vi.fn(),
  adminAddMessage: vi.fn(),
}));

import * as api from "../../api_support";

const mockTickets = [
  {
    id: "t-1",
    tenant_id: "tenant-abc",
    user_id: "user-1",
    subject: "Cannot connect to database",
    status: "open",
    priority: "high",
    created_at: "2026-03-14T10:00:00Z",
    updated_at: "2026-03-14T10:00:00Z",
  },
  {
    id: "t-2",
    tenant_id: "tenant-xyz",
    user_id: "user-2",
    subject: "Billing question",
    status: "resolved",
    priority: "low",
    created_at: "2026-03-13T08:00:00Z",
    updated_at: "2026-03-13T09:00:00Z",
  },
];

const mockTicketWithMessages = {
  ticket: mockTickets[0],
  messages: [
    {
      id: "m-1",
      ticket_id: "t-1",
      sender_type: "customer",
      body: "I keep getting connection timeouts",
      created_at: "2026-03-14T10:01:00Z",
    },
    {
      id: "m-2",
      ticket_id: "t-1",
      sender_type: "support",
      body: "Let me check the server logs",
      created_at: "2026-03-14T10:05:00Z",
    },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.adminListTickets as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockTickets,
  );
  (api.adminGetTicket as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockTicketWithMessages,
  );
  (api.adminUpdateTicket as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockTickets[0],
  );
  (api.adminAddMessage as ReturnType<typeof vi.fn>).mockResolvedValue({
    id: "m-3",
    ticket_id: "t-1",
    sender_type: "support",
    body: "Issue resolved",
    created_at: "2026-03-14T11:00:00Z",
  });
});

describe("SupportTickets", () => {
  it("renders ticket list with subject/status/priority", async () => {
    renderWithProviders(<SupportTickets />);
    await waitFor(() => {
      expect(
        screen.getByText("Cannot connect to database"),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Billing question")).toBeInTheDocument();
    // "open" and "high" appear in both filter select options and the table
    expect(screen.getAllByText("open").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("high").length).toBeGreaterThanOrEqual(1);
  });

  it("expand-to-detail shows message thread", async () => {
    renderWithProviders(<SupportTickets />);
    await waitFor(() => {
      expect(
        screen.getByText("Cannot connect to database"),
      ).toBeInTheDocument();
    });

    const detailBtns = screen.getAllByRole("button", { name: /details/i });
    fireEvent.click(detailBtns[0]);

    await waitFor(() => {
      expect(
        screen.getByText("I keep getting connection timeouts"),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText("Let me check the server logs"),
    ).toBeInTheDocument();
  });

  it("admin can add reply via message form", async () => {
    renderWithProviders(<SupportTickets />);
    await waitFor(() => {
      expect(
        screen.getByText("Cannot connect to database"),
      ).toBeInTheDocument();
    });

    // Expand ticket detail
    const detailBtns = screen.getAllByRole("button", { name: /details/i });
    fireEvent.click(detailBtns[0]);

    await waitFor(() => {
      expect(
        screen.getByText("I keep getting connection timeouts"),
      ).toBeInTheDocument();
    });

    fireEvent.change(screen.getByPlaceholderText(/reply/i), {
      target: { value: "Issue resolved" },
    });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(api.adminAddMessage).toHaveBeenCalledWith("t-1", {
        body: "Issue resolved",
      });
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.adminListTickets as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Network error"),
    );
    renderWithProviders(<SupportTickets />);
    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });
});

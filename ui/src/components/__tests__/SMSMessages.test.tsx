import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SMSMessages } from "../SMSMessages";
import { listAdminSMSMessages } from "../../api";
import type { SMSMessage, SMSMessageListResponse } from "../../types";

vi.mock("../../api", () => ({
  listAdminSMSMessages: vi.fn(),
  adminSendSMS: vi.fn(),
}));

const mockListMessages = vi.mocked(listAdminSMSMessages);

function makeSMSMessage(overrides: Partial<SMSMessage> = {}): SMSMessage {
  return {
    id: "msg_1",
    to: "+15551234567",
    body: "Hello from test",
    provider: "twilio",
    message_id: "SM_abc123",
    status: "delivered",
    created_at: "2026-02-20T10:00:00Z",
    updated_at: "2026-02-20T10:00:00Z",
    ...overrides,
  };
}

function makeSMSMessageListResponse(
  overrides: Partial<SMSMessageListResponse> = {},
): SMSMessageListResponse {
  return {
    items: [
      makeSMSMessage(),
      makeSMSMessage({
        id: "msg_2",
        to: "+15559876543",
        body: "Second test message",
        status: "failed",
        error_message: "delivery failed",
      }),
      makeSMSMessage({
        id: "msg_3",
        to: "+15550001111",
        body: "Third message pending",
        status: "pending",
      }),
    ],
    page: 1,
    perPage: 50,
    totalItems: 3,
    totalPages: 1,
    ...overrides,
  };
}

describe("SMSMessages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders heading", async () => {
    mockListMessages.mockResolvedValueOnce(makeSMSMessageListResponse());
    render(<SMSMessages />);
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: /SMS Messages/i }),
      ).toBeVisible();
    });
  });

  it("shows loading state", () => {
    mockListMessages.mockReturnValue(new Promise(() => {}));
    render(<SMSMessages />);
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("shows empty state when no messages", async () => {
    mockListMessages.mockResolvedValueOnce({
      items: [],
      page: 1,
      perPage: 50,
      totalItems: 0,
      totalPages: 0,
    });
    render(<SMSMessages />);
    await waitFor(() => {
      expect(screen.getByText("No messages sent yet")).toBeInTheDocument();
    });
  });

  it("renders message rows with correct data", async () => {
    const longBody =
      "This is a very long message body that should be truncated after sixty characters to fit nicely";
    const fixture = makeSMSMessageListResponse({
      items: [
        makeSMSMessage({
          id: "msg_1",
          to: "+15551234567",
          body: longBody,
          provider: "twilio",
          status: "delivered",
          created_at: "2026-02-20T10:00:00Z",
        }),
        makeSMSMessage({
          id: "msg_2",
          to: "+15559876543",
          body: "Short body",
          provider: "log",
          status: "failed",
          created_at: "2026-02-20T11:00:00Z",
        }),
      ],
    });
    mockListMessages.mockResolvedValueOnce(fixture);
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByTestId("sms-row-msg_1")).toBeInTheDocument();
      expect(screen.getByTestId("sms-row-msg_2")).toBeInTheDocument();
    });

    // Row 1: phone, truncated body, provider
    const row1 = screen.getByTestId("sms-row-msg_1");
    expect(row1).toHaveTextContent("+15551234567");
    expect(row1).toHaveTextContent(longBody.slice(0, 60) + "…");
    expect(row1).toHaveTextContent("twilio");

    // Row 2: phone, short body (no truncation), provider
    const row2 = screen.getByTestId("sms-row-msg_2");
    expect(row2).toHaveTextContent("+15559876543");
    expect(row2).toHaveTextContent("Short body");
    expect(row2).toHaveTextContent("log");
  });

  it("status badge: delivered is green", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        items: [makeSMSMessage({ status: "delivered" })],
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      const badge = screen.getByTestId("status-badge-delivered");
      expect(badge).toBeInTheDocument();
      expect(badge.className).toContain("bg-green-100");
    });
  });

  it("status badge: failed is red", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        items: [makeSMSMessage({ id: "msg_f", status: "failed" })],
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      const badge = screen.getByTestId("status-badge-failed");
      expect(badge).toBeInTheDocument();
      expect(badge.className).toContain("bg-red-100");
    });
  });

  it("status badge: pending is yellow", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        items: [makeSMSMessage({ id: "msg_p", status: "pending" })],
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      const badge = screen.getByTestId("status-badge-pending");
      expect(badge).toBeInTheDocument();
      expect(badge.className).toContain("bg-yellow-100");
    });
  });

  it("shows error_message text in row when present", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        items: [
          makeSMSMessage({
            id: "msg_err",
            status: "failed",
            error_message: "provider timeout",
          }),
        ],
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      const row = screen.getByTestId("sms-row-msg_err");
      expect(row).toHaveTextContent("provider timeout");
    });
  });

  it("shows pagination when totalPages > 1", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        page: 2,
        totalPages: 3,
        totalItems: 150,
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByTestId("pagination-next")).toBeInTheDocument();
      expect(screen.getByTestId("pagination-prev")).toBeInTheDocument();
      expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();
    });
  });

  it("hides pagination when totalPages <= 1", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        items: [makeSMSMessage()],
        page: 1,
        totalPages: 1,
        totalItems: 1,
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByTestId("sms-row-msg_1")).toBeInTheDocument();
    });

    expect(screen.queryByTestId("pagination-next")).not.toBeInTheDocument();
  });

  it("clicking Next calls listAdminSMSMessages with page 2", async () => {
    const user = userEvent.setup();
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        page: 1,
        totalPages: 3,
        totalItems: 150,
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByTestId("pagination-next")).toBeInTheDocument();
    });

    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({ page: 2, totalPages: 3 }),
    );
    await user.click(screen.getByTestId("pagination-next"));

    await waitFor(() => {
      expect(mockListMessages).toHaveBeenCalledWith(
        expect.objectContaining({ page: 2 }),
      );
    });
  });

  it("Prev button is disabled on page 1", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        page: 1,
        totalPages: 3,
        totalItems: 150,
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByTestId("pagination-prev")).toBeDisabled();
    });
  });

  it("Next button is disabled on last page", async () => {
    mockListMessages.mockResolvedValueOnce(
      makeSMSMessageListResponse({
        page: 3,
        totalPages: 3,
        totalItems: 150,
      }),
    );
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByTestId("pagination-next")).toBeDisabled();
    });
  });

  it("shows error state with retry button", async () => {
    const user = userEvent.setup();
    mockListMessages.mockRejectedValueOnce(new Error("network failure"));
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByText("network failure")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });

    mockListMessages.mockResolvedValueOnce(makeSMSMessageListResponse());
    await user.click(screen.getByText("Retry"));

    expect(mockListMessages).toHaveBeenCalledTimes(2);
  });

  it("Send SMS button is visible in header", async () => {
    mockListMessages.mockResolvedValueOnce(makeSMSMessageListResponse());
    render(<SMSMessages />);

    await waitFor(() => {
      const btn = screen.getByTestId("open-send-modal");
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent("Send SMS");
    });
  });

  it("clicking Send SMS opens the send modal", async () => {
    const user = userEvent.setup();
    mockListMessages.mockResolvedValueOnce(makeSMSMessageListResponse());
    render(<SMSMessages />);

    await waitFor(() => {
      expect(screen.getByTestId("open-send-modal")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("open-send-modal"));

    expect(screen.getByText("Send Test SMS")).toBeInTheDocument();
    expect(screen.getByLabelText("To (phone number)")).toBeInTheDocument();
  });
});

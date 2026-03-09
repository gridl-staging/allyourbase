import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SMSSendTester } from "../SMSSendTester";
import { adminSendSMS } from "../../api";
import type { SMSSendResponse } from "../../types";

vi.mock("../../api", () => ({
  adminSendSMS: vi.fn(),
}));

const mockAdminSendSMS = vi.mocked(adminSendSMS);

function makeSMSSendResponse(
  overrides: Partial<SMSSendResponse> = {},
): SMSSendResponse {
  return {
    message_id: "SM_abc",
    status: "queued",
    to: "+15551234567",
    ...overrides,
  };
}

describe("SMSSendTester", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders phone input, body textarea, and Send button", () => {
    render(<SMSSendTester onClose={vi.fn()} />);
    expect(screen.getByLabelText("To (phone number)")).toBeInTheDocument();
    expect(screen.getByLabelText("Message body")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /send/i })).toBeInTheDocument();
  });

  it("Send button is disabled when both inputs are empty", () => {
    render(<SMSSendTester onClose={vi.fn()} />);
    expect(screen.getByRole("button", { name: /send/i })).toBeDisabled();
  });

  it("Send button is disabled when phone is empty but body is filled", async () => {
    const user = userEvent.setup();
    render(<SMSSendTester onClose={vi.fn()} />);
    await user.type(screen.getByLabelText("Message body"), "Hello");
    expect(screen.getByRole("button", { name: /send/i })).toBeDisabled();
  });

  it("Send button is disabled when body is empty but phone is filled", async () => {
    const user = userEvent.setup();
    render(<SMSSendTester onClose={vi.fn()} />);
    await user.type(screen.getByLabelText("To (phone number)"), "+15551234567");
    expect(screen.getByRole("button", { name: /send/i })).toBeDisabled();
  });

  it("Send button is enabled when both inputs are non-empty", async () => {
    const user = userEvent.setup();
    render(<SMSSendTester onClose={vi.fn()} />);
    await user.type(screen.getByLabelText("To (phone number)"), "+15551234567");
    await user.type(screen.getByLabelText("Message body"), "Hello");
    expect(screen.getByRole("button", { name: /send/i })).toBeEnabled();
  });

  it("shows Sending... while in flight", async () => {
    const user = userEvent.setup();
    mockAdminSendSMS.mockReturnValue(new Promise(() => {}));
    render(<SMSSendTester onClose={vi.fn()} />);

    await user.type(screen.getByLabelText("To (phone number)"), "+15551234567");
    await user.type(screen.getByLabelText("Message body"), "Hello test");
    await user.click(screen.getByRole("button", { name: /send/i }));

    expect(screen.getByRole("button", { name: /sending/i })).toBeDisabled();
  });

  it("calls adminSendSMS with correct args on submit", async () => {
    const user = userEvent.setup();
    mockAdminSendSMS.mockResolvedValueOnce(makeSMSSendResponse());
    render(<SMSSendTester onClose={vi.fn()} />);

    await user.type(screen.getByLabelText("To (phone number)"), "+15551234567");
    await user.type(screen.getByLabelText("Message body"), "Hello test");
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(mockAdminSendSMS).toHaveBeenCalledWith(
        "+15551234567",
        "Hello test",
      );
    });
  });

  it("shows success result with status and to after send", async () => {
    const user = userEvent.setup();
    const fixture = makeSMSSendResponse({
      status: "queued",
      to: "+15559999999",
    });
    mockAdminSendSMS.mockResolvedValueOnce(fixture);
    render(<SMSSendTester onClose={vi.fn()} />);

    await user.type(screen.getByLabelText("To (phone number)"), "+15559999999");
    await user.type(screen.getByLabelText("Message body"), "Test msg");
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      const result = screen.getByTestId("send-result");
      expect(result).toBeInTheDocument();
      expect(result).toHaveTextContent("queued");
      expect(result).toHaveTextContent("+15559999999");
    });
  });

  it("shows error message when adminSendSMS rejects", async () => {
    const user = userEvent.setup();
    mockAdminSendSMS.mockRejectedValueOnce(new Error("send failed"));
    render(<SMSSendTester onClose={vi.fn()} />);

    await user.type(screen.getByLabelText("To (phone number)"), "+15551234567");
    await user.type(screen.getByLabelText("Message body"), "Hello");
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      const err = screen.getByTestId("send-error");
      expect(err).toBeInTheDocument();
      expect(err).toHaveTextContent("send failed");
    });
  });

  it("clears inputs after successful send", async () => {
    const user = userEvent.setup();
    mockAdminSendSMS.mockResolvedValueOnce(makeSMSSendResponse());
    render(<SMSSendTester onClose={vi.fn()} />);

    const phoneInput = screen.getByLabelText("To (phone number)");
    const bodyInput = screen.getByLabelText("Message body");

    await user.type(phoneInput, "+15551234567");
    await user.type(bodyInput, "Hello test");
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(screen.getByTestId("send-result")).toBeInTheDocument();
    });

    expect(phoneInput).toHaveValue("");
    expect(bodyInput).toHaveValue("");
  });

  it("calls onClose when Cancel is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<SMSSendTester onClose={onClose} />);

    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onSent after successful send", async () => {
    const user = userEvent.setup();
    const onSent = vi.fn();
    mockAdminSendSMS.mockResolvedValueOnce(makeSMSSendResponse());
    render(<SMSSendTester onClose={vi.fn()} onSent={onSent} />);

    await user.type(screen.getByLabelText("To (phone number)"), "+15551234567");
    await user.type(screen.getByLabelText("Message body"), "Hello");
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(onSent).toHaveBeenCalledTimes(1);
    });
  });

  it("does not call onSent on failed send", async () => {
    const user = userEvent.setup();
    const onSent = vi.fn();
    mockAdminSendSMS.mockRejectedValueOnce(new Error("send failed"));
    render(<SMSSendTester onClose={vi.fn()} onSent={onSent} />);

    await user.type(screen.getByLabelText("To (phone number)"), "+15551234567");
    await user.type(screen.getByLabelText("Message body"), "Hello");
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(screen.getByTestId("send-error")).toBeInTheDocument();
    });
    expect(onSent).not.toHaveBeenCalled();
  });
});

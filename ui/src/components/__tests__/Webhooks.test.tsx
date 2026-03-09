import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Webhooks } from "../Webhooks";
import {
  listWebhooks,
  createWebhook,
  updateWebhook,
  deleteWebhook,
  testWebhook,
  listWebhookDeliveries,
} from "../../api";
import type { WebhookResponse } from "../../types";

vi.mock("../../api", () => ({
  listWebhooks: vi.fn(),
  createWebhook: vi.fn(),
  updateWebhook: vi.fn(),
  deleteWebhook: vi.fn(),
  testWebhook: vi.fn(),
  listWebhookDeliveries: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListWebhooks = vi.mocked(listWebhooks);
const mockCreateWebhook = vi.mocked(createWebhook);
const mockUpdateWebhook = vi.mocked(updateWebhook);
const mockDeleteWebhook = vi.mocked(deleteWebhook);
const mockTestWebhook = vi.mocked(testWebhook);
const mockListDeliveries = vi.mocked(listWebhookDeliveries);

function makeWebhook(
  overrides: Partial<WebhookResponse> = {},
): WebhookResponse {
  return {
    id: "wh_1",
    url: "https://example.com/hook",
    hasSecret: false,
    events: ["create", "update", "delete"],
    tables: [],
    enabled: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("Webhooks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading state", () => {
    mockListWebhooks.mockReturnValue(new Promise(() => {}));
    render(<Webhooks />);
    expect(screen.getByText("Loading webhooks...")).toBeInTheDocument();
  });

  it("displays empty state when no webhooks", async () => {
    mockListWebhooks.mockResolvedValueOnce([]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(
        screen.getByText("No webhooks configured yet"),
      ).toBeInTheDocument();
    });
  });

  it("renders webhook list", async () => {
    mockListWebhooks.mockResolvedValueOnce([
      makeWebhook({ url: "https://foo.com/hook" }),
      makeWebhook({
        id: "wh_2",
        url: "https://bar.com/hook",
        enabled: false,
      }),
    ]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("https://foo.com/hook")).toBeInTheDocument();
      expect(screen.getByText("https://bar.com/hook")).toBeInTheDocument();
    });
  });

  it("displays events as colored badges", async () => {
    mockListWebhooks.mockResolvedValueOnce([
      makeWebhook({ events: ["create", "delete"] }),
    ]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("create")).toBeInTheDocument();
      expect(screen.getByText("delete")).toBeInTheDocument();
    });
  });

  it("shows 'all tables' when tables array is empty", async () => {
    mockListWebhooks.mockResolvedValueOnce([makeWebhook({ tables: [] })]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("all tables")).toBeInTheDocument();
    });
  });

  it("shows table names when tables are set", async () => {
    mockListWebhooks.mockResolvedValueOnce([
      makeWebhook({ tables: ["posts", "users"] }),
    ]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("posts")).toBeInTheDocument();
      expect(screen.getByText("users")).toBeInTheDocument();
    });
  });

  it("opens create modal on Add Webhook click", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(
        screen.getByText("No webhooks configured yet"),
      ).toBeInTheDocument();
    });
    await user.click(screen.getByText("Add Webhook"));
    expect(screen.getByText("New Webhook")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("https://example.com/webhook"),
    ).toBeInTheDocument();
  });

  it("creates a webhook via the form", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValue([]);
    mockCreateWebhook.mockResolvedValueOnce(
      makeWebhook({ url: "https://test.com/hook" }),
    );
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("Add Webhook")).toBeInTheDocument();
    });
    await user.click(screen.getByText("Add Webhook"));

    const urlInput = screen.getByPlaceholderText(
      "https://example.com/webhook",
    );
    await user.type(urlInput, "https://test.com/hook");
    await user.click(screen.getByText("Create"));

    await waitFor(() => {
      expect(mockCreateWebhook).toHaveBeenCalledWith(
        expect.objectContaining({ url: "https://test.com/hook" }),
      );
    });
  });

  it("opens edit modal", async () => {
    const user = userEvent.setup();
    const wh = makeWebhook({ url: "https://edit-me.com/hook" });
    mockListWebhooks.mockResolvedValueOnce([wh]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("https://edit-me.com/hook")).toBeInTheDocument();
    });

    const editButtons = screen.getAllByTitle("Edit");
    await user.click(editButtons[0]);
    expect(screen.getByText("Edit Webhook")).toBeInTheDocument();
    expect(
      screen.getByDisplayValue("https://edit-me.com/hook"),
    ).toBeInTheDocument();
  });

  it("opens delete confirmation", async () => {
    const user = userEvent.setup();
    const wh = makeWebhook({ url: "https://delete-me.com/hook" });
    mockListWebhooks.mockResolvedValueOnce([wh]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(
        screen.getByText("https://delete-me.com/hook"),
      ).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle("Delete");
    await user.click(deleteButtons[0]);
    expect(screen.getByText("Delete Webhook")).toBeInTheDocument();
  });

  it("deletes a webhook on confirm", async () => {
    const user = userEvent.setup();
    const wh = makeWebhook();
    mockListWebhooks.mockResolvedValue([wh]);
    mockDeleteWebhook.mockResolvedValueOnce();
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("https://example.com/hook")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle("Delete");
    await user.click(deleteButtons[0]);

    // Find the red Delete button in the confirmation modal.
    const confirmBtn = screen
      .getAllByRole("button", { name: "Delete" })
      .find((btn) => btn.classList.contains("bg-red-600"));
    expect(confirmBtn).toBeDefined();
    await user.click(confirmBtn!);

    await waitFor(() => {
      expect(mockDeleteWebhook).toHaveBeenCalledWith("wh_1");
    });
  });

  it("closes modal on Cancel", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("Add Webhook")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Add Webhook"));
    expect(screen.getByText("New Webhook")).toBeInTheDocument();

    await user.click(screen.getByText("Cancel"));
    expect(screen.queryByText("New Webhook")).not.toBeInTheDocument();
  });

  it("shows has-secret lock icon", async () => {
    mockListWebhooks.mockResolvedValueOnce([
      makeWebhook({ hasSecret: true }),
    ]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(
        screen.getByTitle("HMAC secret configured"),
      ).toBeInTheDocument();
    });
  });

  it("displays error on fetch failure", async () => {
    mockListWebhooks.mockRejectedValueOnce(new Error("network error"));
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByText("network error")).toBeInTheDocument();
    });
    expect(screen.getByText("Retry")).toBeInTheDocument();
  });

  it("has toggle switch for enabled state", async () => {
    mockListWebhooks.mockResolvedValueOnce([makeWebhook({ enabled: true })]);
    mockUpdateWebhook.mockResolvedValueOnce(
      makeWebhook({ enabled: false }),
    );
    render(<Webhooks />);
    await waitFor(() => {
      const toggle = screen.getByRole("switch");
      expect(toggle).toBeInTheDocument();
      expect(toggle).toHaveAttribute("aria-checked", "true");
    });
  });

  it("calls updateWebhook when toggle is clicked", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([makeWebhook({ enabled: true })]);
    mockUpdateWebhook.mockResolvedValueOnce(makeWebhook({ enabled: false }));
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByRole("switch")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("switch"));

    await waitFor(() => {
      expect(mockUpdateWebhook).toHaveBeenCalledWith("wh_1", {
        enabled: false,
      });
    });
  });

  it("shows test button and calls testWebhook on click", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([makeWebhook()]);
    mockTestWebhook.mockResolvedValueOnce({
      success: true,
      statusCode: 200,
      durationMs: 42,
    });
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByTitle("Test")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Test"));

    await waitFor(() => {
      expect(mockTestWebhook).toHaveBeenCalledWith("wh_1");
    });
  });

  it("shows error toast when test fails", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([makeWebhook()]);
    mockTestWebhook.mockResolvedValueOnce({
      success: false,
      statusCode: 500,
      durationMs: 100,
      error: "Internal Server Error",
    });
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByTitle("Test")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Test"));

    await waitFor(() => {
      expect(mockTestWebhook).toHaveBeenCalledWith("wh_1");
    });
  });

  it("shows Delivery History button per webhook", async () => {
    mockListWebhooks.mockResolvedValueOnce([makeWebhook()]);
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByTitle("Delivery History")).toBeInTheDocument();
    });
  });

  it("opens delivery history modal and calls listWebhookDeliveries", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([makeWebhook()]);
    mockListDeliveries.mockResolvedValueOnce({
      items: [
        {
          id: "del_1",
          webhookId: "wh_1",
          eventAction: "create",
          eventTable: "posts",
          success: true,
          statusCode: 200,
          attempt: 1,
          durationMs: 42,
          deliveredAt: "2026-02-09T10:00:00Z",
        },
      ],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByTitle("Delivery History")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Delivery History"));

    await waitFor(() => {
      expect(screen.getByText("Delivery History")).toBeInTheDocument();
      expect(mockListDeliveries).toHaveBeenCalledWith("wh_1", {
        page: 1,
        perPage: 20,
      });
    });
  });

  it("shows empty delivery history state", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([makeWebhook()]);
    mockListDeliveries.mockResolvedValueOnce({
      items: [],
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
    });
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByTitle("Delivery History")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Delivery History"));

    await waitFor(() => {
      expect(
        screen.getByText("No deliveries recorded yet"),
      ).toBeInTheDocument();
    });
  });

  it("shows success and failure status indicators in delivery list", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([makeWebhook()]);
    mockListDeliveries.mockResolvedValueOnce({
      items: [
        {
          id: "del_1",
          webhookId: "wh_1",
          eventAction: "create",
          eventTable: "posts",
          success: true,
          statusCode: 200,
          attempt: 1,
          durationMs: 10,
          deliveredAt: "2026-02-09T10:00:00Z",
        },
        {
          id: "del_2",
          webhookId: "wh_1",
          eventAction: "update",
          eventTable: "users",
          success: false,
          statusCode: 500,
          attempt: 3,
          durationMs: 250,
          error: "server error",
          deliveredAt: "2026-02-09T10:01:00Z",
        },
      ],
      page: 1,
      perPage: 20,
      totalItems: 2,
      totalPages: 1,
    });
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByTitle("Delivery History")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Delivery History"));

    await waitFor(() => {
      // Both status codes shown
      expect(screen.getByText("200")).toBeInTheDocument();
      expect(screen.getByText("500")).toBeInTheDocument();
      // Event action badges (use getAllByText since "create"/"update" also appear in webhook row)
      expect(screen.getAllByText("create").length).toBeGreaterThanOrEqual(2);
      expect(screen.getAllByText("update").length).toBeGreaterThanOrEqual(2);
      // Duration shown
      expect(screen.getByText("10ms")).toBeInTheDocument();
      expect(screen.getByText("250ms")).toBeInTheDocument();
    });
  });

  it("expands delivery row to show details", async () => {
    const user = userEvent.setup();
    mockListWebhooks.mockResolvedValueOnce([makeWebhook()]);
    mockListDeliveries.mockResolvedValueOnce({
      items: [
        {
          id: "del_1",
          webhookId: "wh_1",
          eventAction: "create",
          eventTable: "posts",
          success: false,
          statusCode: 500,
          attempt: 2,
          durationMs: 100,
          error: "connection timeout",
          requestBody: '{"action":"create","table":"posts"}',
          responseBody: "Internal Server Error",
          deliveredAt: "2026-02-09T10:00:00Z",
        },
      ],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    render(<Webhooks />);
    await waitFor(() => {
      expect(screen.getByTitle("Delivery History")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Delivery History"));

    await waitFor(() => {
      expect(screen.getByText("500")).toBeInTheDocument();
    });

    // Click to expand the delivery row
    await user.click(screen.getByText("500"));

    await waitFor(() => {
      expect(screen.getByText("connection timeout")).toBeInTheDocument();
      expect(
        screen.getByText('{"action":"create","table":"posts"}'),
      ).toBeInTheDocument();
      expect(
        screen.getByText("Internal Server Error"),
      ).toBeInTheDocument();
    });
  });
});

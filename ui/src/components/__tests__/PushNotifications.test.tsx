import { vi, describe, it, expect, beforeEach } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import userEvent from "@testing-library/user-event";
import { PushNotifications } from "../PushNotifications";
import {
  listAdminPushDevices,
  registerAdminPushDevice,
  revokeAdminPushDevice,
  listAdminPushDeliveries,
  getAdminPushDelivery,
  adminSendPush,
} from "../../api";
import type {
  PushDeviceToken,
  PushDelivery,
  PushDeviceListResponse,
  PushDeliveryListResponse,
  PushSendResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listAdminPushDevices: vi.fn(),
  registerAdminPushDevice: vi.fn(),
  revokeAdminPushDevice: vi.fn(),
  listAdminPushDeliveries: vi.fn(),
  getAdminPushDelivery: vi.fn(),
  adminSendPush: vi.fn(),
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListAdminPushDevices = vi.mocked(listAdminPushDevices);
const mockRegisterAdminPushDevice = vi.mocked(registerAdminPushDevice);
const mockRevokeAdminPushDevice = vi.mocked(revokeAdminPushDevice);
const mockListAdminPushDeliveries = vi.mocked(listAdminPushDeliveries);
const mockGetAdminPushDelivery = vi.mocked(getAdminPushDelivery);
const mockAdminSendPush = vi.mocked(adminSendPush);

function makeDevice(overrides: Partial<PushDeviceToken> = {}): PushDeviceToken {
  return {
    id: "tok-1",
    app_id: "app-1",
    user_id: "user-1",
    provider: "fcm",
    platform: "android",
    token: "fcm-token-alpha-1234567890",
    device_name: "Pixel 8",
    is_active: true,
    last_used: "2026-02-22T09:00:00Z",
    last_refreshed_at: "2026-02-22T08:00:00Z",
    created_at: "2026-02-22T07:00:00Z",
    updated_at: "2026-02-22T08:00:00Z",
    ...overrides,
  };
}

function makeDelivery(overrides: Partial<PushDelivery> = {}): PushDelivery {
  return {
    id: "deliv-1",
    device_token_id: "tok-1",
    job_id: "job-1",
    app_id: "app-1",
    user_id: "user-1",
    provider: "fcm",
    title: "Challenge updated",
    body: "A new weekly challenge is available",
    data_payload: { challenge_id: "c-123" },
    status: "pending",
    error_code: null,
    error_message: null,
    provider_message_id: null,
    sent_at: null,
    created_at: "2026-02-22T10:00:00Z",
    updated_at: "2026-02-22T10:00:00Z",
    ...overrides,
  };
}

function makeDeviceList(items: PushDeviceToken[]): PushDeviceListResponse {
  return { items };
}

function makeDeliveryList(items: PushDelivery[]): PushDeliveryListResponse {
  return { items };
}

function makeSendResponse(deliveries: PushDelivery[]): PushSendResponse {
  return { deliveries };
}

describe("PushNotifications", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockListAdminPushDevices.mockResolvedValue(makeDeviceList([makeDevice()]));
    mockRegisterAdminPushDevice.mockResolvedValue(makeDevice({ id: "tok-new" }));
    mockRevokeAdminPushDevice.mockResolvedValue(undefined);
    mockListAdminPushDeliveries.mockResolvedValue(makeDeliveryList([makeDelivery()]));
    mockGetAdminPushDelivery.mockResolvedValue(makeDelivery());
    mockAdminSendPush.mockResolvedValue(makeSendResponse([makeDelivery()]));
  });

  it("renders devices tab and revokes a device", async () => {
    renderWithProviders(<PushNotifications />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Push Notifications" })).toBeInTheDocument();
      expect(screen.getByText("Pixel 8")).toBeInTheDocument();
      expect(screen.getByText("fcm")).toBeInTheDocument();
      expect(screen.getByText("android")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Revoke device tok-1" }));

    await waitFor(() => {
      expect(mockRevokeAdminPushDevice).toHaveBeenCalledWith("tok-1");
    });
  });

  it("applies trimmed device filters and reuses applied filters after revoke refresh", async () => {
    renderWithProviders(<PushNotifications />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(mockListAdminPushDevices).toHaveBeenCalledWith({
        app_id: "",
        user_id: "",
        include_inactive: false,
      });
    });

    const appFilter = screen.getByLabelText("Filter App ID");
    await user.type(appFilter, "  app-filter  ");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() => {
      expect(mockListAdminPushDevices).toHaveBeenLastCalledWith({
        app_id: "app-filter",
        user_id: "",
        include_inactive: false,
      });
    });

    await user.clear(appFilter);
    await user.type(appFilter, "draft-app");
    await user.click(screen.getByRole("button", { name: "Revoke device tok-1" }));

    await waitFor(() => {
      expect(mockListAdminPushDevices).toHaveBeenLastCalledWith({
        app_id: "app-filter",
        user_id: "",
        include_inactive: false,
      });
    });
  });

  it("registers a new device from the devices tab", async () => {
    renderWithProviders(<PushNotifications />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Register Device" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Register Device" }));
    await user.type(screen.getByLabelText("App ID"), "app-2");
    await user.type(screen.getByLabelText("User ID"), "user-2");
    await user.selectOptions(screen.getByLabelText("Provider"), "apns");
    await user.selectOptions(screen.getByLabelText("Platform"), "ios");
    await user.type(screen.getByLabelText("Token"), "apns-token-123");
    await user.type(screen.getByLabelText("Device Name"), "iPhone 16");

    await user.click(screen.getByRole("button", { name: "Save Device" }));

    await waitFor(() => {
      expect(mockRegisterAdminPushDevice).toHaveBeenCalledWith({
        app_id: "app-2",
        user_id: "user-2",
        provider: "apns",
        platform: "ios",
        token: "apns-token-123",
        device_name: "iPhone 16",
      });
    });
  });

  it("loads deliveries, applies status filter, and expands delivery details", async () => {
    renderWithProviders(<PushNotifications />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Deliveries" })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: "Deliveries" }));

    await waitFor(() => {
      expect(screen.getByText("Challenge updated")).toBeInTheDocument();
      expect(mockListAdminPushDeliveries).toHaveBeenCalledWith({
        app_id: "",
        user_id: "",
        status: "",
      });
    });

    await user.selectOptions(screen.getByLabelText("Status"), "failed");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() => {
      expect(mockListAdminPushDeliveries).toHaveBeenLastCalledWith({
        app_id: "",
        user_id: "",
        status: "failed",
      });
    });

    await user.click(screen.getByRole("button", { name: "View delivery deliv-1" }));

    await waitFor(() => {
      expect(mockGetAdminPushDelivery).toHaveBeenCalledWith("deliv-1");
      expect(screen.getByText("Job ID")).toBeInTheDocument();
      expect(screen.getByText("job-1")).toBeInTheDocument();
      expect(screen.getByText("\"challenge_id\": \"c-123\"")).toBeInTheDocument();
    });
  });

  it("sends a test push from deliveries tab", async () => {
    renderWithProviders(<PushNotifications />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Deliveries" })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: "Deliveries" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Send Test Push" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Send Test Push" }));

    await user.type(screen.getByLabelText("App ID"), "app-3");
    await user.type(screen.getByLabelText("User ID"), "user-3");
    await user.type(screen.getByLabelText("Title"), "Weekly Kudos");
    await user.type(screen.getByLabelText("Body"), "You got 5 kudos");
    fireEvent.change(screen.getByLabelText("Data (JSON)"), {
      target: { value: "{\"kudos_id\":\"k-55\"}" },
    });

    await user.click(screen.getByRole("button", { name: "Send Push" }));

    await waitFor(() => {
      expect(mockAdminSendPush).toHaveBeenCalledWith({
        app_id: "app-3",
        user_id: "user-3",
        title: "Weekly Kudos",
        body: "You got 5 kudos",
        data: { kudos_id: "k-55" },
      });
    });
  });
});

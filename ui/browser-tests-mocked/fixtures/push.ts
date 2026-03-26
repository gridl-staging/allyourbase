/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/push.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface PushMockState {
  listDevicesCalls: number;
  lastDevicesQuery: URLSearchParams | null;
  registerCalls: number;
  lastRegisterBody: Record<string, unknown> | null;
  revokeCalls: number;
  lastRevokedId: string | null;
  listDeliveriesCalls: number;
  lastDeliveriesQuery: URLSearchParams | null;
  sendCalls: number;
  lastSendBody: Record<string, unknown> | null;
  getDeliveryCalls: number;
}

export interface PushMockOptions {
  registerResponder?: (body: Record<string, unknown>) => MockApiResponse;
  sendResponder?: (body: Record<string, unknown>) => MockApiResponse;
}

const defaultDevices = {
  items: [
    {
      id: "device-001",
      app_id: "app-001",
      user_id: "user-001",
      provider: "fcm",
      platform: "android",
      token: "abcdef1234567890abcdef1234567890xyz789",
      device_name: "Pixel 8",
      is_active: true,
      last_refreshed_at: "2026-02-20T10:00:00Z",
      last_used: "2026-02-21T15:00:00Z",
      created_at: "2026-02-01T08:00:00Z",
      updated_at: "2026-02-20T10:00:00Z",
    },
  ],
};

const defaultDeliveries = {
  items: [
    {
      id: "delivery-001",
      device_token_id: "device-001",
      job_id: "job-001",
      app_id: "app-001",
      user_id: "delivery-user-001",
      provider: "fcm",
      title: "Hello push",
      body: "Push notification body text",
      data_payload: { action_url: "https://example.com/action" },
      status: "sent",
      error_code: null,
      error_message: null,
      provider_message_id: "fcm-msg-001",
      sent_at: "2026-02-21T15:01:00Z",
      created_at: "2026-02-21T15:00:00Z",
      updated_at: "2026-02-21T15:01:00Z",
    },
  ],
};

const defaultDeliveryDetail = defaultDeliveries.items[0];

const defaultRegisteredDevice = {
  id: "device-new",
  app_id: "app-register-test",
  user_id: "user-register-test",
  provider: "fcm",
  platform: "android",
  token: "my-new-token-value",
  device_name: "Test Phone",
  is_active: true,
  last_refreshed_at: "2026-02-22T12:00:00Z",
  created_at: "2026-02-22T12:00:00Z",
  updated_at: "2026-02-22T12:00:00Z",
};

const defaultSendResponse = {
  deliveries: [
    {
      id: "delivery-new-001",
      device_token_id: "device-001",
      app_id: "app-send-test",
      user_id: "user-send-test",
      provider: "fcm",
      title: "Test Notification",
      body: "This is a test push body",
      status: "pending",
      created_at: "2026-02-22T12:00:00Z",
      updated_at: "2026-02-22T12:00:00Z",
    },
  ],
};

/**
 * TODO: Document mockAdminPushApis.
 */
export async function mockAdminPushApis(
  page: Page,
  options: PushMockOptions = {},
): Promise<PushMockState> {
  const state: PushMockState = {
    listDevicesCalls: 0,
    lastDevicesQuery: null,
    registerCalls: 0,
    lastRegisterBody: null,
    revokeCalls: 0,
    lastRevokedId: null,
    listDeliveriesCalls: 0,
    lastDeliveriesQuery: null,
    sendCalls: 0,
    lastSendBody: null,
    getDeliveryCalls: 0,
  };

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (await handleCommonAdminRoutes(route, method, path)) return;

    if (method === "GET" && path === "/api/admin/push/devices") {
      state.listDevicesCalls += 1;
      state.lastDevicesQuery = url.searchParams;
      return json(route, 200, defaultDevices);
    }

    if (method === "POST" && path === "/api/admin/push/devices") {
      state.registerCalls += 1;
      const body = request.postDataJSON();
      state.lastRegisterBody = body;
      if (options.registerResponder) {
        const resp = options.registerResponder(body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 201, defaultRegisteredDevice);
    }

    if (method === "DELETE" && path.startsWith("/api/admin/push/devices/")) {
      state.revokeCalls += 1;
      state.lastRevokedId = path.split("/").pop() || null;
      return json(route, 200, { ok: true });
    }

    if (method === "GET" && path === "/api/admin/push/deliveries") {
      state.listDeliveriesCalls += 1;
      state.lastDeliveriesQuery = url.searchParams;
      return json(route, 200, defaultDeliveries);
    }

    if (method === "GET" && path.startsWith("/api/admin/push/deliveries/")) {
      state.getDeliveryCalls += 1;
      return json(route, 200, defaultDeliveryDetail);
    }

    if (method === "POST" && path === "/api/admin/push/send") {
      state.sendCalls += 1;
      const body = request.postDataJSON();
      state.lastSendBody = body;
      if (options.sendResponder) {
        const resp = options.sendResponder(body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, defaultSendResponse);
    }

    return unhandledMockedApiRoute(route, method, path);
  });

  return state;
}

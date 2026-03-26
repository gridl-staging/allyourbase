import {
  test,
  expect,
  waitForDashboard,
  ensureUserByEmail,
  cleanupUserByEmail,
  cleanupApiKeyByName,
  createApiKeyForUser,
} from "../fixtures";
import type { Page, Response } from "@playwright/test";

const REALTIME_STATS_PATH = "/api/admin/realtime/stats";
const REALTIME_WS_PATH = "/api/realtime/ws";
const POLL_TIMEOUT_MS = 8000;

interface InspectorMetrics {
  total: number;
  sse: number;
  ws: number;
  usersTableSubscriptions: number;
}

/**
 * SMOKE TEST: Realtime Inspector
 *
 * Critical Path: Navigate to Realtime Inspector → Verify live telemetry renders
 * from /api/admin/realtime/stats (connection metrics, subscription table).
 *
 * The first smoke test does NOT inject websocket traffic — it verifies the page
 * loads and renders the live snapshot even when counts are zero.
 */

test.describe("Smoke: Realtime Inspector", () => {
  test("admin can navigate to Realtime Inspector and see live metrics", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);

    const initialStatsResponsePromise = waitForRealtimeStatsResponse(page);

    await page.locator("aside").getByRole("button", { name: /Realtime Inspector/i }).click();

    await expect(page.getByRole("heading", { name: /Realtime Inspector/i })).toBeVisible({ timeout: 15_000 });

    const initialStatsResponse = await initialStatsResponsePromise;
    expect(initialStatsResponse.ok()).toBeTruthy();

    const initialSnapshot = await initialStatsResponse.json();
    expect(initialSnapshot.version).toBeTruthy();
    expect(typeof initialSnapshot.timestamp).toBe("string");
    expect(typeof initialSnapshot.connections?.sse).toBe("number");
    expect(typeof initialSnapshot.connections?.ws).toBe("number");
    expect(typeof initialSnapshot.connections?.total).toBe("number");
    expect(typeof initialSnapshot.counters?.dropped_messages).toBe("number");
    expect(typeof initialSnapshot.counters?.heartbeat_failures).toBe("number");
    expect(initialSnapshot.subscriptions).toBeTruthy();
    expect(initialSnapshot.subscriptions.channels).toBeTruthy();

    // Verify metric cards reflect the live snapshot values.
    await expect(page.getByTestId("realtime-total-metric-value")).toHaveText(
      String(initialSnapshot.connections.total),
    );
    await expect(page.getByTestId("realtime-sse-metric-value")).toHaveText(
      String(initialSnapshot.connections.sse),
    );
    await expect(page.getByTestId("realtime-ws-metric-value")).toHaveText(
      String(initialSnapshot.connections.ws),
    );
    await expect(page.getByTestId("realtime-dropped-metric-value")).toHaveText(
      String(initialSnapshot.counters.dropped_messages),
    );
    await expect(page.getByTestId("realtime-heartbeat-failures-metric-value")).toHaveText(
      String(initialSnapshot.counters.heartbeat_failures),
    );

    // Verify subscriptions section is present
    await expect(page.getByText("Subscriptions").first()).toBeVisible();

    // Verify refresh button exists and is clickable
    const refreshBtn = page.locator("main").getByRole("button", { name: /Refresh/i });
    await expect(refreshBtn).toBeVisible();

    const refreshStatsResponsePromise = waitForRealtimeStatsResponse(page);
    await refreshBtn.click();

    const refreshStatsResponse = await refreshStatsResponsePromise;
    expect(refreshStatsResponse.ok()).toBeTruthy();

    // After refresh, the heading should still be visible and the live request should still succeed.
    await expect(page.getByRole("heading", { name: /Realtime Inspector/i })).toBeVisible();
  });

  test("opens live realtime activity and returns to baseline after cleanup", async ({ page, request, adminToken }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /Realtime Inspector/i }).click();
    await expect(page.getByRole("heading", { name: /Realtime Inspector/i })).toBeVisible({ timeout: 15_000 });

    const runId = Date.now();
    const wsUserEmail = `realtime-smoke-${runId}@example.test`;
    const wsKeyName = `realtime-smoke-key-${runId}`;
    const wsUser = await ensureUserByEmail(request, adminToken, wsUserEmail);
    const wsKeyBody = await createApiKeyForUser(request, adminToken, wsUser.id, wsKeyName);

    const baseline = await readInspectorMetrics(page);
    try {
      await withRealtimeWsSubscription(page.url(), wsKeyBody.key, "users", async () => {
        const withActivity = await waitForInspectorMetrics(
          page,
          "WebSocket activity to appear in inspector metrics",
          (snapshot) =>
            snapshot.ws >= baseline.ws + 1 &&
            snapshot.total >= baseline.total + 1 &&
            snapshot.usersTableSubscriptions >= baseline.usersTableSubscriptions + 1,
        );

        expect(withActivity.ws).toBeGreaterThanOrEqual(baseline.ws + 1);
        expect(withActivity.total).toBeGreaterThanOrEqual(baseline.total + 1);
        expect(withActivity.usersTableSubscriptions).toBeGreaterThanOrEqual(
          baseline.usersTableSubscriptions + 1,
        );
      });

      const afterCleanup = await waitForInspectorMetrics(
        page,
        "WebSocket activity cleanup to return inspector metrics to baseline",
        (snapshot) =>
          snapshot.ws === baseline.ws &&
          snapshot.total === baseline.total &&
          snapshot.usersTableSubscriptions === baseline.usersTableSubscriptions,
      );

      expect(afterCleanup.ws).toBe(baseline.ws);
      expect(afterCleanup.total).toBe(baseline.total);
      expect(afterCleanup.usersTableSubscriptions).toBe(baseline.usersTableSubscriptions);
    } finally {
      await cleanupApiKeyByName(request, adminToken, wsKeyName).catch(() => {});
      await cleanupUserByEmail(request, adminToken, wsUserEmail).catch(() => {});
    }
  });
});

async function waitForRealtimeStatsResponse(page: Page): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === REALTIME_STATS_PATH && response.request().method() === "GET";
  });
}

async function refreshAndReadMetrics(
  page: Page,
): Promise<Omit<InspectorMetrics, "usersTableSubscriptions">> {
  const responsePromise = waitForRealtimeStatsResponse(page);
  await page.locator("main").getByRole("button", { name: /Refresh/i }).click();
  const response = await responsePromise;
  expect(response.ok()).toBeTruthy();
  return {
    total: await readMetricValue(page, "realtime-total-metric-value"),
    sse: await readMetricValue(page, "realtime-sse-metric-value"),
    ws: await readMetricValue(page, "realtime-ws-metric-value"),
  };
}

async function readMetricValue(page: Page, testId: string): Promise<number> {
  const valueText = await page.getByTestId(testId).innerText();
  const value = Number(valueText.trim());
  if (Number.isNaN(value)) {
    throw new Error(`Metric ${testId} was not numeric: ${valueText}`);
  }
  return value;
}

type SubscriptionType = "table" | "broadcast" | "presence";

async function readSubscriptionCount(
  page: Page,
  subscriptionName: string,
  subscriptionType: SubscriptionType,
): Promise<number> {
  const rows = page.getByRole("row");
  const rowCount = await rows.count();
  for (let i = 0; i < rowCount; i += 1) {
    const row = rows.nth(i);
    const cells = row.getByRole("cell");
    if ((await cells.count()) < 3) {
      continue;
    }
    const nameText = (await cells.nth(0).innerText()).trim();
    const typeText = (await cells.nth(1).innerText()).trim();
    if (nameText !== subscriptionName || typeText !== subscriptionType) {
      continue;
    }
    const countText = (await cells.nth(2).innerText()).trim();
    const count = Number(countText);
    if (Number.isNaN(count)) {
      throw new Error(`Subscription count was not numeric for ${subscriptionName}: ${countText}`);
    }
    return count;
  }
  return 0;
}

async function readInspectorMetrics(page: Page): Promise<InspectorMetrics> {
  const metrics = await refreshAndReadMetrics(page);
  const usersTableSubscriptions = await readSubscriptionCount(page, "users", "table");
  return { ...metrics, usersTableSubscriptions };
}

async function waitForInspectorMetrics(
  page: Page,
  description: string,
  predicate: (metrics: InspectorMetrics) => boolean,
): Promise<InspectorMetrics> {
  let latestSnapshot = await readInspectorMetrics(page);
  await expect
    .poll(
      async () => {
        latestSnapshot = await readInspectorMetrics(page);
        return predicate(latestSnapshot);
      },
      { timeout: POLL_TIMEOUT_MS, message: description },
    )
    .toBe(true);
  return latestSnapshot;
}

async function openRealtimeWsSubscription(
  currentPageUrl: string,
  token: string,
  table: string,
): Promise<WebSocket> {
  const wsURL = buildRealtimeWsUrl(currentPageUrl, token);
  const ws = new WebSocket(wsURL);
  await waitForWebSocketOpen(ws);
  ws.send(JSON.stringify({ type: "subscribe", ref: "inspect-users", tables: [table] }));
  return ws;
}

async function withRealtimeWsSubscription<T>(
  currentPageUrl: string,
  token: string,
  table: string,
  run: () => Promise<T>,
): Promise<T> {
  const ws = await openRealtimeWsSubscription(currentPageUrl, token, table);
  let runSucceeded = false;
  try {
    const result = await run();
    runSucceeded = true;
    return result;
  } finally {
    if (runSucceeded) {
      // Run body passed — await cleanup so a cleanup-only failure surfaces clearly.
      await closeRealtimeWsSubscription(ws);
    } else {
      // Run body already failed — close fire-and-forget so the primary error
      // propagates immediately without risking a cleanup timeout that masks it.
      closeRealtimeWsSubscription(ws).catch(() => {});
    }
  }
}

function buildRealtimeWsUrl(currentPageUrl: string, token: string): string {
  const currentURL = new URL(currentPageUrl);
  const wsProtocol = currentURL.protocol === "https:" ? "wss:" : "ws:";
  const wsURL = new URL(REALTIME_WS_PATH, `${wsProtocol}//${currentURL.host}`);
  wsURL.searchParams.set("token", token);
  return wsURL.toString();
}

async function waitForWebSocketOpen(ws: WebSocket): Promise<void> {
  if (ws.readyState === ws.OPEN) {
    return;
  }
  if (ws.readyState === ws.CLOSING || ws.readyState === ws.CLOSED) {
    throw new Error("WebSocket is not openable");
  }

  await new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      cleanup();
      reject(new Error("Timed out waiting for WebSocket to open"));
    }, 5000);
    const onOpen = () => {
      cleanup();
      resolve();
    };
    const onError = () => {
      cleanup();
      reject(new Error("WebSocket failed to open"));
    };
    const onClose = () => {
      cleanup();
      reject(new Error("WebSocket closed before opening"));
    };
    const cleanup = () => {
      clearTimeout(timeout);
      ws.removeEventListener("open", onOpen);
      ws.removeEventListener("error", onError);
      ws.removeEventListener("close", onClose);
    };
    ws.addEventListener("open", onOpen);
    ws.addEventListener("error", onError);
    ws.addEventListener("close", onClose);
    if (ws.readyState === ws.OPEN) {
      cleanup();
      resolve();
    }
  });
}

async function closeRealtimeWsSubscription(ws: WebSocket): Promise<void> {
  if (ws.readyState === ws.CLOSED) {
    return;
  }

  await new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      cleanup();
      reject(new Error("Timed out waiting for WebSocket to close"));
    }, 5000);
    const onClose = () => {
      cleanup();
      resolve();
    };
    const onError = () => {
      cleanup();
      reject(new Error("WebSocket failed while closing"));
    };
    const cleanup = () => {
      clearTimeout(timeout);
      ws.removeEventListener("close", onClose);
      ws.removeEventListener("error", onError);
    };
    ws.addEventListener("close", onClose);
    ws.addEventListener("error", onError);
    if (ws.readyState !== ws.CLOSING) {
      ws.close();
    }
    if (ws.readyState === ws.CLOSED) {
      cleanup();
      resolve();
    }
  });
}

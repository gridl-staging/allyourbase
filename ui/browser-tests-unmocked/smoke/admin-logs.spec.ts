import { test, expect, triggerAdminStatsRequest, waitForDashboard } from "../fixtures";
import type { Page, Response } from "@playwright/test";

const ADMIN_LOGS_PATH = "/api/admin/logs";

interface AdminLogsPayload {
  entries?: unknown;
  message?: unknown;
}

function isBufferingUnavailable(payload: AdminLogsPayload): boolean {
  return typeof payload.message === "string" && payload.message.toLowerCase().includes("not enabled");
}

async function waitForAdminLogsResponse(page: Page): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === ADMIN_LOGS_PATH && response.request().method() === "GET";
  });
}

test.describe("Smoke: Admin Logs", () => {
  test("admin can verify a known in-run log row or buffering-disabled fallback", async ({
    page,
    request,
    adminToken,
  }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);

    const initialLogsResponsePromise = waitForAdminLogsResponse(page);
    await page.locator("aside").getByRole("button", { name: /Admin Logs/i }).click();
    await expect(page.getByRole("heading", { name: /Admin Logs/i })).toBeVisible({ timeout: 15_000 });

    const initialLogsResponse = await initialLogsResponsePromise;
    expect(initialLogsResponse.ok()).toBeTruthy();

    const initialPayload = (await initialLogsResponse.json()) as AdminLogsPayload;
    if (isBufferingUnavailable(initialPayload)) {
      await expect(page.getByText(/log buffering not enabled/i)).toBeVisible();
      return;
    }

    const requestId = await triggerAdminStatsRequest(request, adminToken);

    const refreshResponsePromise = waitForAdminLogsResponse(page);
    await page.getByTestId("admin-logs-panel").getByRole("button", { name: /^Refresh$/i }).click();
    const refreshResponse = await refreshResponsePromise;
    expect(refreshResponse.ok()).toBeTruthy();

    const refreshPayload = (await refreshResponse.json()) as AdminLogsPayload;
    expect(isBufferingUnavailable(refreshPayload)).toBe(false);

    const triggeredRow = page
      .getByTestId("admin-logs-panel")
      .getByRole("row")
      .filter({ hasText: requestId })
      .first();

    await expect(triggeredRow).toBeVisible();
    await expect(triggeredRow).toContainText("request");
    await expect(triggeredRow).toContainText("/api/admin/stats");
  });
});

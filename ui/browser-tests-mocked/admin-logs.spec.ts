import { test, expect, bootstrapMockedAdminApp, mockAdminLogsApis } from "./fixtures";

test.describe("Admin Logs (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
    await mockAdminLogsApis(page, {
      responses: [
        {
          entries: [
            {
              time: "2026-03-15T10:00:00Z",
              level: "INFO",
              message: "boot complete",
              attrs: { path: "/api/admin/status", status: 200 },
            },
            {
              time: "2026-03-15T10:05:00Z",
              level: "ERROR",
              message: "request failed",
              attrs: { path: "/api/admin/sql", status: 500 },
            },
          ],
        },
        {
          entries: [
            {
              time: "2026-03-15T10:00:00Z",
              level: "INFO",
              message: "boot complete",
              attrs: { path: "/api/admin/status", status: 200 },
            },
            {
              time: "2026-03-15T10:05:00Z",
              level: "ERROR",
              message: "request failed",
              attrs: { path: "/api/admin/sql", status: 500 },
            },
          ],
        },
        {
          entries: [
            {
              time: "2026-03-15T10:10:00Z",
              level: "WARN",
              message: "refresh result",
              attrs: { path: "/api/admin/logs", status: 200 },
            },
          ],
        },
      ],
    });
  });

  test("navigates, filters rows, and refreshes with updated payload", async ({ page }) => {
    await page.goto("/admin/");

    await page.locator("aside").getByRole("button", { name: /Admin Logs/i }).click();
    await expect(page.getByRole("heading", { name: /Admin Logs/i })).toBeVisible();

    await expect(page.getByText("boot complete")).toBeVisible();
    await expect(page.getByText("request failed")).toBeVisible();

    await page.getByLabel("Level").selectOption("error");
    await expect(page.getByText("request failed")).toBeVisible();
    await expect(page.getByText("boot complete")).toBeHidden();

    await page.getByLabel("Level").selectOption("");
    await page.getByLabel("Search logs").fill("boot");
    await expect(page.getByText("boot complete")).toBeVisible();
    await expect(page.getByText("request failed")).toBeHidden();

    await page.getByLabel("Search logs").fill("");
    await page.getByTestId("admin-logs-panel").getByRole("button", { name: /^Refresh$/i }).click();

    await expect(page.getByText("refresh result")).toBeVisible();
    await expect(page.getByText("request failed")).toBeHidden();
  });
});

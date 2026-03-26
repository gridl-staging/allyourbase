import { test, expect, bootstrapMockedAdminApp, mockAdminIncidentApis } from "./fixtures";

test.describe("Incidents Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("list-500: shows error text with heading", async ({ page }) => {
    await mockAdminIncidentApis(page, {
      listResponder: () => ({ status: 500, body: { message: "failed to list incidents" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Incidents$/i }).click();

    await expect(page.getByRole("heading", { name: /Incidents/i })).toBeVisible();
    await expect(page.getByText(/failed to list incidents/i)).toBeVisible();
  });

  test("empty-state: shows AdminTable empty message", async ({ page }) => {
    await mockAdminIncidentApis(page);
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Incidents$/i }).click();

    await expect(page.getByRole("heading", { name: /Incidents/i })).toBeVisible();
    await expect(page.getByText(/No incidents/i)).toBeVisible();
  });

  test("create-500: shows error after creation attempt", async ({ page }) => {
    await mockAdminIncidentApis(page, {
      createResponder: () => ({ status: 500, body: { message: "incident store unavailable" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Incidents$/i }).click();

    // Open create form
    await page.getByRole("button", { name: /Create Incident/i }).click();

    // Fill title (required)
    await page.getByPlaceholder(/Incident title/i).fill("API outage");

    // Submit
    await page.getByRole("button", { name: /^Create$/i }).click();

    await expect(page.getByText(/incident store unavailable/i)).toBeVisible();
  });

  test("add-update-500: shows error after adding timeline update", async ({ page }) => {
    // Provide an incident in the list, then fail the add-update
    await mockAdminIncidentApis(page, {
      listResponder: () => ({
        status: 200,
        body: [
          {
            id: "inc-001",
            title: "Database degradation",
            status: "investigating",
            affectedServices: ["api", "dashboard"],
            createdAt: "2026-02-28T12:00:00Z",
            updatedAt: "2026-02-28T12:00:00Z",
          },
        ],
      }),
      addUpdateResponder: () => ({ status: 500, body: { message: "timeline write failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Incidents$/i }).click();

    // Expand incident to show timeline
    await page.getByText("Details").click();

    // Fill update message and submit
    await page.getByPlaceholder(/Update message/i).fill("Root cause identified");
    await page.getByRole("button", { name: /Add Update/i }).click();

    await expect(page.getByText(/timeline write failed/i)).toBeVisible();
  });

  test("resolve-500: shows error after resolve attempt", async ({ page }) => {
    await mockAdminIncidentApis(page, {
      listResponder: () => ({
        status: 200,
        body: [
          {
            id: "inc-002",
            title: "Cache miss storm",
            status: "monitoring",
            affectedServices: ["cdn"],
            createdAt: "2026-02-28T12:00:00Z",
            updatedAt: "2026-02-28T12:00:00Z",
          },
        ],
      }),
      updateResponder: () => ({ status: 500, body: { message: "failed to resolve incident" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Incidents$/i }).click();

    // Click the Resolve button on the incident row
    await page.getByRole("button", { name: /Resolve/i }).click();

    await expect(page.getByText(/failed to resolve incident/i)).toBeVisible();
  });
});

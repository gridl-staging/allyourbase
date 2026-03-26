import {
  test,
  expect,
  probeEndpoint,
  seedIncident,
  cleanupIncidentByID,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Incidents Lifecycle
 *
 * Critical Path: Load seeded incident → create incident via UI → add update → resolve
 */

test.describe("Incidents Lifecycle (Full E2E)", () => {
  const incidentIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (incidentIDs.length > 0) {
      const id = incidentIDs.pop();
      if (!id) continue;
      await cleanupIncidentByID(request, adminToken, id).catch(() => {});
    }
  });

  test("load-and-verify seeded incident, then create, update timeline, and resolve via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/incidents");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Incidents service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededTitle = `incident-full-seeded-${runId}`;
    const createdTitle = `incident-full-created-${runId}`;
    const affectedService = `svc-lifecycle-${runId}`;

    const seeded = await seedIncident(request, adminToken, {
      title: seededTitle,
      status: "investigating",
      affectedServices: ["api-gateway"],
    });
    incidentIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Incidents$/i }).click();
    await expect(page.getByRole("heading", { name: /Incidents/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded incident
    const seededRow = page.getByRole("row", { name: new RegExp(seededTitle) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow).toContainText("investigating");

    // Create new incident via UI
    await page.getByRole("button", { name: /Create Incident/i }).click();

    await page.getByPlaceholder(/Incident title/i).fill(createdTitle);
    await page.getByPlaceholder(/Affected services/i).fill(affectedService);
    await page.getByRole("button", { name: /^Create$/i }).click();

    const createdRow = page.getByRole("row", { name: new RegExp(createdTitle) }).first();
    await expect(createdRow).toBeVisible({ timeout: 5000 });
    await expect(createdRow).toContainText(affectedService);
    await expect(createdRow).toContainText("investigating");

    // Expand details and add an update
    await createdRow.getByRole("button", { name: /Details/i }).click();

    const updateMessage = `Timeline update at ${runId}`;
    await page.getByPlaceholder(/Update message/i).fill(updateMessage);
    await page.getByRole("button", { name: /Add Update/i }).click();

    await expect(page.getByText(updateMessage)).toBeVisible({ timeout: 5000 });

    // Resolve the created incident
    await createdRow.getByRole("button", { name: /Resolve/i }).click();
    await expect(createdRow).toContainText("resolved", { timeout: 5000 });
  });
});

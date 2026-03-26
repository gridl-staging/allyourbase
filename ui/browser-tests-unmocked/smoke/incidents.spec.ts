import {
  test,
  expect,
  seedIncident,
  cleanupIncidentByID,
  probeEndpoint,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Incidents
 *
 * Critical Path: Navigate to Incidents → Verify seeded incident row and timeline details
 */

test.describe("Smoke: Incidents", () => {
  const incidentIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (incidentIDs.length > 0) {
      const incidentID = incidentIDs.pop();
      if (!incidentID) continue;
      await cleanupIncidentByID(request, adminToken, incidentID).catch(() => {});
    }
  });

  test("seeded incident renders in list and detail timeline", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/incidents");
    test.skip(
      probeStatus === 501 || probeStatus === 404,
      `Incidents service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const incidentTitle = `Smoke Incident ${runId}`;
    const updateMessage = `Incident update ${runId}`;

    const seeded = await seedIncident(request, adminToken, {
      title: incidentTitle,
      status: "investigating",
      affectedServices: ["database", "api"],
      initialUpdateMessage: updateMessage,
      initialUpdateStatus: "monitoring",
    });
    incidentIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Incidents/i }).click();
    await expect(page.getByRole("heading", { name: /Incidents/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByText(incidentTitle).first()).toBeVisible({ timeout: 5000 });

    const row = page.locator("tr").filter({ hasText: incidentTitle }).first();
    await row.getByRole("button", { name: /Details/i }).click();

    await expect(page.getByRole("heading", { name: new RegExp(incidentTitle) })).toBeVisible({
      timeout: 5000,
    });
    await expect(page.getByText(updateMessage)).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: /Create Incident/i })).toBeVisible();
  });
});

import { randomUUID } from "crypto";
import {
  test,
  expect,
  probeEndpoint,
  seedSite,
  cleanupSiteByID,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Sites Hosting
 *
 * Critical Path: Seed a site via admin API -> Navigate to Sites -> Verify
 * seeded row renders with name, slug, and row actions.
 */

test.describe("Smoke: Sites Hosting", () => {
  test("seeded site renders in Sites table with row actions", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/sites");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `Sites admin API unavailable in this environment (status ${probeStatus})`,
    );

    const runID = randomUUID().replace(/-/g, "").slice(0, 10);
    const siteName = `Smoke Site ${runID}`;
    const siteSlug = `smoke-site-${runID}`;
    let seededSiteID: string | null = null;

    let testFailure: unknown = null;
    let cleanupFailure: unknown = null;

    try {
      const seededSite = await seedSite(request, adminToken, {
        name: siteName,
        slug: siteSlug,
      });
      seededSiteID = seededSite.id;

      await page.goto("/admin/");
      await waitForDashboard(page);

      await page.locator("aside").getByRole("button", { name: /Sites/i }).click();
      await expect(page.getByRole("heading", { name: /^Sites$/i })).toBeVisible({ timeout: 15_000 });

      const seededSiteRow = page
        .locator("tr")
        .filter({ has: page.getByRole("button", { name: `View ${seededSite.name}` }) })
        .first();
      await expect(seededSiteRow).toBeVisible({ timeout: 5000 });
      await expect(seededSiteRow).toContainText(seededSite.name);
      await expect(seededSiteRow).toContainText(seededSite.slug);
      await expect(
        seededSiteRow.getByRole("button", { name: `View ${seededSite.name}` }),
      ).toBeVisible();
      await expect(
        seededSiteRow.getByRole("button", { name: `Delete ${seededSite.name}` }),
      ).toBeVisible();
    } catch (error) {
      testFailure = error;
    }

    if (seededSiteID !== null) {
      try {
        await cleanupSiteByID(request, adminToken, seededSiteID);
      } catch (error) {
        cleanupFailure = error;
      }
    }

    if (testFailure !== null && cleanupFailure !== null) {
      throw new AggregateError(
        [testFailure, cleanupFailure],
        `Sites smoke failed and cleanup for seeded site ${seededSiteID} also failed`,
      );
    }
    if (testFailure !== null) {
      throw testFailure;
    }
    if (cleanupFailure !== null) {
      throw cleanupFailure;
    }
  });
});

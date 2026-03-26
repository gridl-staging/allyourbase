import { randomUUID } from "crypto";
import { test, expect, getSite, getSiteStatus, cleanupSiteByID, waitForDashboard } from "../fixtures";

test.describe("Sites Lifecycle (Full E2E)", () => {
  test("create, update, and delete a site through the Sites detail view", async ({
    page,
    request,
    adminToken,
  }) => {
    const runSuffix = randomUUID().replace(/-/g, "").slice(0, 8);
    const siteName = `Lifecycle Site ${runSuffix}`;
    const updatedSiteName = `${siteName} Updated`;
    const siteSlug = `lifecycle-site-${runSuffix}`;
    let siteID: string | null = null;

    try {
      await page.goto("/admin/");
      await waitForDashboard(page);

      await page.locator("aside").getByRole("button", { name: /Sites/i }).click();
      await expect(page.getByRole("heading", { name: /^Sites$/i })).toBeVisible({ timeout: 5000 });

      await page.getByRole("button", { name: /Add Site/i }).click();
      await page.getByLabel("Name").fill(siteName);
      await page.getByLabel("Slug").fill(siteSlug);

      const createResponsePromise = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          response.url().endsWith("/api/admin/sites"),
      );
      await page.getByRole("button", { name: /^Create$/i }).click();
      const createResponse = await createResponsePromise;
      expect(createResponse.ok()).toBeTruthy();

      const createdSite = (await createResponse.json()) as { id: string };
      siteID = createdSite.id;
      expect(siteID).toBeTruthy();

      await expect(page.getByRole("button", { name: `View ${siteName}` })).toBeVisible({
        timeout: 5000,
      });
      await page.getByRole("button", { name: `View ${siteName}` }).click();
      await expect(page.getByRole("heading", { name: /Site Settings/i })).toBeVisible();

      await page.getByLabel("Name").fill(updatedSiteName);
      const spaModeCheckbox = page.getByLabel("SPA mode");
      if (!(await spaModeCheckbox.isChecked())) {
        await spaModeCheckbox.click();
      }

      const updateResponsePromise = page.waitForResponse(
        (response) =>
          response.request().method() === "PUT" &&
          siteID !== null &&
          response.url().endsWith(`/api/admin/sites/${siteID}`),
      );
      await page.getByRole("button", { name: /Save Settings/i }).click();
      const updateResponse = await updateResponsePromise;
      expect(updateResponse.ok()).toBeTruthy();

      const updatedSite = await getSite(request, adminToken, siteID);
      expect(updatedSite.name).toBe(updatedSiteName);
      expect(updatedSite.spaMode).toBe(true);

      await page.getByRole("button", { name: /Back to Sites/i }).click();
      await expect(page.getByRole("heading", { name: /^Sites$/i })).toBeVisible();
      await expect(page.getByRole("button", { name: `View ${updatedSiteName}` })).toBeVisible();

      await page.getByRole("button", { name: `Delete ${updatedSiteName}` }).click();
      await expect(page.getByRole("heading", { name: /Delete Site/i })).toBeVisible();

      const deleteResponsePromise = page.waitForResponse(
        (response) =>
          response.request().method() === "DELETE" &&
          siteID !== null &&
          response.url().endsWith(`/api/admin/sites/${siteID}`),
      );
      await page.getByRole("button", { name: /^Delete$/i }).click();
      const deleteResponse = await deleteResponsePromise;
      expect(deleteResponse.status()).toBe(204);

      const deletedStatus = await getSiteStatus(request, adminToken, siteID);
      expect(deletedStatus).toBe(404);
      siteID = null;
    } finally {
      if (siteID) {
        await cleanupSiteByID(request, adminToken, siteID);
      }
    }
  });
});

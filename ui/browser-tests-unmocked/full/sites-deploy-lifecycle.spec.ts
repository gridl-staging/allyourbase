import { randomUUID } from "crypto";
import { type Page } from "@playwright/test";
import {
  test,
  expect,
  probeEndpoint,
  seedSite,
  cleanupSiteByID,
  createSiteDeploy,
  getSiteDeploy,
  listSiteDeploys,
  uploadSiteDeployFile,
  promoteSiteDeploy,
  failSiteDeploy,
  waitForDashboard,
} from "../fixtures";

function deployRowByPromoteButton(page: Page, deployID: string) {
  return page
    .locator("tr")
    .filter({ has: page.getByRole("button", { name: `Promote ${deployID}` }) })
    .first();
}

function escapeRegex(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function deployRowByStatusAndBytes(page: Page, status: string, bytes: string) {
  return page.getByRole("row", {
    name: new RegExp(`\\b${status}\\b.*${escapeRegex(bytes)}`, "i"),
  });
}

async function openSiteDetail(page: Page, siteName: string): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /Sites/i }).click();
  await expect(page.getByRole("heading", { name: /^Sites$/i })).toBeVisible({
    timeout: 5000,
  });
  await page.getByRole("button", { name: `View ${siteName}` }).click();
  await expect(page.getByRole("heading", { name: /Site Settings/i })).toBeVisible({
    timeout: 5000,
  });
  await expect(page.getByText("Deploy History")).toBeVisible();
}

test.describe("Sites Deploy Lifecycle (Full E2E)", () => {
  test("create, inspect, upload, promote, fail, and rollback deploys with dashboard history proof", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/sites");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `Sites admin API unavailable in this environment (status ${probeStatus})`,
    );

    const runSuffix = randomUUID().replace(/-/g, "").slice(0, 8);
    const siteName = `Deploy Site ${runSuffix}`;
    const siteSlug = `deploy-site-${runSuffix}`;
    let siteID: string | null = null;

    const firstDeployIndexHTML = `<!doctype html><html><body>${"A".repeat(1600)}</body></html>`;
    const firstDeployAppJS = `console.log("${"B".repeat(128)}");`;
    const secondDeployIndexHTML = `<!doctype html><html><body>${"C".repeat(64)}</body></html>`;

    try {
      const site = await seedSite(request, adminToken, {
        name: siteName,
        slug: siteSlug,
      });
      siteID = site.id;

      const firstDeploy = await createSiteDeploy(request, adminToken, siteID);
      expect(firstDeploy.status).toBe("uploading");
      expect(firstDeploy.fileCount).toBe(0);

      let uploadedFirstIndex;
      try {
        uploadedFirstIndex = await uploadSiteDeployFile(
          request,
          adminToken,
          siteID,
          firstDeploy.id,
          {
            name: "index.html",
            content: firstDeployIndexHTML,
            mimeType: "text/html",
          },
        );
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        test.skip(
          /failed with status 404: 404 page not found/i.test(message),
          `Site deploy upload endpoint unavailable in this environment (${message})`,
        );
        throw error;
      }
      expect(uploadedFirstIndex.fileCount).toBe(1);
      expect(uploadedFirstIndex.totalBytes).toBe(firstDeployIndexHTML.length);

      const uploadedFirstAsset = await uploadSiteDeployFile(
        request,
        adminToken,
        siteID,
        firstDeploy.id,
        {
          name: "assets/app.js",
          content: firstDeployAppJS,
          mimeType: "application/javascript",
        },
      );
      expect(uploadedFirstAsset.fileCount).toBe(2);
      expect(uploadedFirstAsset.totalBytes).toBe(
        firstDeployIndexHTML.length + firstDeployAppJS.length,
      );

      const promotedFirstDeploy = await promoteSiteDeploy(
        request,
        adminToken,
        siteID,
        firstDeploy.id,
      );
      expect(promotedFirstDeploy.status).toBe("live");

      const secondDeploy = await createSiteDeploy(request, adminToken, siteID);
      expect(secondDeploy.status).toBe("uploading");
      expect(secondDeploy.fileCount).toBe(0);

      const secondDeployDetail = await getSiteDeploy(
        request,
        adminToken,
        siteID,
        secondDeploy.id,
      );
      expect(secondDeployDetail.id).toBe(secondDeploy.id);
      expect(secondDeployDetail.siteId).toBe(siteID);
      expect(secondDeployDetail.status).toBe("uploading");

      const uploadedSecondDeploy = await uploadSiteDeployFile(
        request,
        adminToken,
        siteID,
        secondDeploy.id,
        {
          name: "index.html",
          content: secondDeployIndexHTML,
          mimeType: "text/html",
        },
      );
      expect(uploadedSecondDeploy.fileCount).toBe(1);
      expect(uploadedSecondDeploy.totalBytes).toBe(secondDeployIndexHTML.length);

      const deployListBeforeUI = await listSiteDeploys(request, adminToken, siteID);
      expect(deployListBeforeUI.totalCount).toBe(2);
      expect(deployListBeforeUI.deploys.map((deploy) => deploy.id)).toEqual(
        expect.arrayContaining([firstDeploy.id, secondDeploy.id]),
      );

      await openSiteDetail(page, siteName);

      const secondDeployUploadingRow = deployRowByPromoteButton(page, secondDeploy.id);
      await expect(secondDeployUploadingRow).toBeVisible({ timeout: 5000 });
      await expect(secondDeployUploadingRow).toContainText("uploading");
      await expect(secondDeployUploadingRow).toContainText("1");
      await expect(secondDeployUploadingRow).toContainText("105.0 B");

      const firstDeployLiveRow = deployRowByStatusAndBytes(page, "live", "1.7 KB");
      await expect(firstDeployLiveRow).toBeVisible({ timeout: 5000 });
      await expect(firstDeployLiveRow).toContainText("2");

      const promoteResponsePromise = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          response.url().endsWith(`/api/admin/sites/${siteID}/deploys/${secondDeploy.id}/promote`),
      );
      await page.getByRole("button", { name: `Promote ${secondDeploy.id}` }).click();
      const promoteResponse = await promoteResponsePromise;
      expect(promoteResponse.ok()).toBeTruthy();

      const secondDeployLiveRow = deployRowByStatusAndBytes(page, "live", "105.0 B");
      await expect(secondDeployLiveRow).toBeVisible({ timeout: 5000 });
      const firstDeploySupersededRow = deployRowByStatusAndBytes(page, "superseded", "1.7 KB");
      await expect(firstDeploySupersededRow).toBeVisible({ timeout: 5000 });

      const failedDeploy = await createSiteDeploy(request, adminToken, siteID);
      expect(failedDeploy.status).toBe("uploading");

      const failedDeployResponse = await failSiteDeploy(
        request,
        adminToken,
        siteID,
        failedDeploy.id,
        "upload timed out in browser audit",
      );
      expect(failedDeployResponse.status).toBe("failed");
      expect(failedDeployResponse.errorMessage).toContain("upload timed out");

      await openSiteDetail(page, siteName);

      const failedDeployRow = deployRowByStatusAndBytes(page, "failed", "0 B");
      await expect(failedDeployRow).toBeVisible({ timeout: 5000 });
      await expect(failedDeployRow).toContainText("0");

      const rollbackResponsePromise = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          response.url().endsWith(`/api/admin/sites/${siteID}/deploys/rollback`),
      );
      await page.getByRole("button", { name: /Rollback Site/i }).click();
      const rollbackResponse = await rollbackResponsePromise;
      expect(rollbackResponse.ok()).toBeTruthy();

      const rolledBackLiveRow = deployRowByStatusAndBytes(page, "live", "1.7 KB");
      await expect(rolledBackLiveRow).toBeVisible({ timeout: 5000 });
      const rolledBackSupersededRow = deployRowByStatusAndBytes(page, "superseded", "105.0 B");
      await expect(rolledBackSupersededRow).toBeVisible({ timeout: 5000 });
      await expect(deployRowByStatusAndBytes(page, "failed", "0 B")).toBeVisible();

      const deployListAfterRollback = await listSiteDeploys(request, adminToken, siteID);
      expect(deployListAfterRollback.totalCount).toBe(3);

      const rollbackLiveDeploy = deployListAfterRollback.deploys.find(
        (deploy) => deploy.id === firstDeploy.id,
      );
      expect(rollbackLiveDeploy?.status).toBe("live");

      const rollbackSupersededDeploy = deployListAfterRollback.deploys.find(
        (deploy) => deploy.id === secondDeploy.id,
      );
      expect(rollbackSupersededDeploy?.status).toBe("superseded");

      const rollbackFailedDeploy = deployListAfterRollback.deploys.find(
        (deploy) => deploy.id === failedDeploy.id,
      );
      expect(rollbackFailedDeploy?.status).toBe("failed");
    } finally {
      if (siteID) {
        await cleanupSiteByID(request, adminToken, siteID);
      }
    }
  });
});

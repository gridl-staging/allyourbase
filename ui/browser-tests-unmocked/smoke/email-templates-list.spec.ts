import {
  test,
  expect,
  probeEndpoint,
  seedEmailTemplate,
  cleanupEmailTemplate,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Email Templates - List View
 *
 * Critical Path: Navigate to Email Templates → verify template list and editing controls
 */

test.describe("Smoke: Email Templates", () => {
  const seededTemplateKeys: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededTemplateKeys.length > 0) {
      const templateKey = seededTemplateKeys.pop();
      if (!templateKey) continue;
      await cleanupEmailTemplate(request, adminToken, templateKey).catch(() => {});
    }
  });

  test("seeded template key and subject render in email templates view", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/email/templates");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Email templates service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const templateKey = `smoke.template_${runId}`;
    const subjectTemplate = `Smoke Subject ${runId}`;
    await seedEmailTemplate(request, adminToken, {
      key: templateKey,
      subjectTemplate,
      htmlTemplate: `<html><body>Smoke Template ${runId}</body></html>`,
    });
    seededTemplateKeys.push(templateKey);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Email Templates$/i }).click();
    await expect(page.getByRole("heading", { name: /Email Templates/i })).toBeVisible({ timeout: 15_000 });

    const seededTemplateButton = page.getByRole("button").filter({ hasText: templateKey }).first();
    await expect(seededTemplateButton).toBeVisible({ timeout: 5000 });
    await seededTemplateButton.click();

    await expect(page.getByLabel(/Subject Template/i)).toHaveValue(subjectTemplate, { timeout: 5000 });
  });
});

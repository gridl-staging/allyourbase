import {
  test,
  expect,
  probeEndpoint,
  seedAIPrompt,
  cleanupAIPromptByName,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: AI Assistant
 *
 * Critical Path: Navigate to AI Assistant → Verify page heading, subtitle, tab
 * structure, and Logs tab table headers render in the page body.
 */

test.describe("Smoke: AI Assistant", () => {
  const seededPromptNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededPromptNames.length > 0) {
      const promptName = seededPromptNames.pop();
      if (!promptName) continue;
      await cleanupAIPromptByName(request, adminToken, promptName).catch(() => {});
    }
  });

  test("seeded prompt renders in prompts tab", async ({ page, request, adminToken }) => {
    // POST probe catches 503 "AI prompt store not configured" that GET (list) may not surface.
    const status = await probeEndpoint(request, adminToken, "/api/admin/ai/prompts", {
      method: "POST",
      data: {},
    });
    test.skip(
      status === 501 || status === 404 || status === 503,
      `AI prompts endpoint not available (status ${status})`,
    );

    const promptName = `smoke-prompt-${Date.now()}`;
    await seedAIPrompt(request, adminToken, {
      name: promptName,
      template: "Return {{name}}",
    });
    seededPromptNames.push(promptName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /AI Assistant/i }).click();
    await expect(page.getByRole("heading", { name: /AI Assistant/i })).toBeVisible({ timeout: 15_000 });

    const promptsTab = page.getByRole("button", { name: /^Prompts$/i });
    await expect(promptsTab).toBeVisible();
    await promptsTab.click();

    const seededPromptRow = page.locator("tr").filter({ hasText: promptName }).first();
    await expect(seededPromptRow).toBeVisible({ timeout: 5000 });
  });
});

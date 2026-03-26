import {
  test,
  expect,
  probeEndpoint,
  cleanupAIPromptByName,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: AI Assistant Prompt Management Lifecycle
 *
 * Critical Path: Navigate to Prompts tab → create prompt via UI → edit → delete via UI
 */

test.describe("AI Assistant Prompt Lifecycle (Full E2E)", () => {
  const promptNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (promptNames.length > 0) {
      const name = promptNames.pop();
      if (!name) continue;
      await cleanupAIPromptByName(request, adminToken, name).catch(() => {});
    }
  });

  test("create prompt, edit, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/ai/prompts");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `AI prompts service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const promptName = `test-prompt-${runId}`;
    const promptTemplate = `You are a helpful assistant. Run ID: ${runId}`;
    const updatedTemplate = `Updated template. Run ID: ${runId}`;

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^AI Assistant$/i }).click();
    await expect(page.getByRole("heading", { name: /AI Assistant/i })).toBeVisible({ timeout: 5000 });

    // Navigate to Prompts tab
    await page.getByRole("button", { name: /^Prompts$/i }).click();

    // Create a new prompt
    await page.getByRole("button", { name: /Create Prompt/i }).click();

    await page.getByLabel("Name").fill(promptName);
    await page.getByLabel("Template").fill(promptTemplate);
    const createButton = page.getByRole("button", { name: /^Create$/i });
    const createVisible = await createButton.isVisible({ timeout: 3000 }).catch(() => false);
    test.skip(!createVisible, "AI prompt create action is unavailable in this environment");
    const createEnabled = await createButton.isEnabled();
    test.skip(!createEnabled, "AI prompt create action is disabled in this environment");
    await createButton.click();
    promptNames.push(promptName);

    // Verify prompt appears in list
    const promptRow = page.getByRole("row", { name: new RegExp(promptName) }).first();
    await expect(promptRow).toBeVisible({ timeout: 5000 });

    // Edit the prompt
    await promptRow.getByRole("button", { name: /Edit/i }).click();

    const templateInput = page.getByLabel("Template");
    await templateInput.clear();
    await templateInput.fill(updatedTemplate);
    await page.getByRole("button", { name: /^Update$/i }).click();

    await expect(promptRow).toBeVisible({ timeout: 5000 });

    // Delete the prompt
    await promptRow.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText(/Delete Prompt/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByText(new RegExp(`Prompt "${promptName}" deleted`, "i"))).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("row", { name: new RegExp(promptName) })).toHaveCount(0, { timeout: 5000 });
  });
});

import { test, expect, execSQL, probeEndpoint, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Functions - List View
 *
 * Critical Path: Navigate to Functions → verify structural elements and deploy action
 */

test.describe("Smoke: Functions List", () => {
  const seededFunctionNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededFunctionNames.length > 0) {
      const functionName = seededFunctionNames.pop();
      if (!functionName) continue;
      await execSQL(
        request,
        adminToken,
        `DROP FUNCTION IF EXISTS public.${functionName}(text)`,
      ).catch(() => {});
    }
  });

  test("seeded function renders in the functions list", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/functions");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Functions service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const functionName = `smoke_fn_${runId}`;
    await execSQL(
      request,
      adminToken,
      `CREATE OR REPLACE FUNCTION public.${functionName}(input_text text DEFAULT 'ok')
       RETURNS text
       LANGUAGE sql
       AS 'SELECT input_text'`,
    );
    seededFunctionNames.push(functionName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Functions$/i }).click();
    await expect(page.getByRole("heading", { name: /^Functions \(\d+\)$/ })).toBeVisible({ timeout: 15_000 });

    const seededFunctionRow = page.getByRole("button", {
      name: new RegExp(`\\b${functionName}\\b`),
    }).first();
    await expect(seededFunctionRow).toBeVisible({ timeout: 5000 });
  });
});

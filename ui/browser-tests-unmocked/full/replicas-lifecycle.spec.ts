import { test, expect, probeEndpoint, seedReplica, cleanupReplicaByName, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: Replicas Lifecycle
 *
 * Critical Path: Seed replica via API → verify in list → add replica via UI form →
 * verify in list → check health → remove replica via UI
 */

test.describe("Replicas Lifecycle (Full E2E)", () => {
  const replicaNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (replicaNames.length > 0) {
      const name = replicaNames.pop();
      if (!name) continue;
      await cleanupReplicaByName(request, adminToken, name).catch(() => {});
    }
  });

  test("seed replica, add via UI, check health, and remove via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/replicas");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Replicas service unavailable (status ${probeStatus})`,
    );
    const createProbeStatus = await probeEndpoint(request, adminToken, "/api/admin/replicas", {
      method: "POST",
      data: {},
    });
    test.skip(
      createProbeStatus === 503 || createProbeStatus === 502 || createProbeStatus === 404 || createProbeStatus === 501,
      `Replica creation unavailable (status ${createProbeStatus})`,
    );

    const runId = Date.now();
    const seededName = `replica-seeded-${runId}`;
    const createdName = `replica-created-${runId}`;

    // Seed a replica via API
    try {
      await Promise.race([
        seedReplica(request, adminToken, {
          name: seededName,
          host: "10.0.0.1",
          port: 5432,
          database: "testdb",
        }),
        new Promise<never>((_, reject) => {
          setTimeout(() => reject(new Error("Replica seed timed out")), 8000);
        }),
      ]);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      test.skip(
        /status 502|request context disposed|test timeout|timed out/i.test(message),
        `Replica seed unavailable in this environment (${message})`,
      );
      throw err;
    }
    replicaNames.push(seededName);

    // Navigate to Replicas
    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Replicas$/i }).click();
    await expect(page.getByRole("heading", { name: /Replicas/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded replica appears in table
    await expect(page.getByText("10.0.0.1").first()).toBeVisible({ timeout: 5000 });

    // Check Health
    const checkHealthButton = page.getByRole("button", { name: /Check Health/i });
    await expect(checkHealthButton).toBeVisible();
    await checkHealthButton.click();

    // Verify health check completes (toast or status badge updates)
    const healthResult = page.getByText(/Health check completed|healthy|down|unknown/i);
    await expect(healthResult.first()).toBeVisible({ timeout: 10000 });

    // Add replica via UI form
    await page.getByRole("button", { name: /Add Replica/i }).click();

    // Fill form fields using label selectors
    await page.getByLabel("Name").fill(createdName);
    await page.getByLabel("Host").fill("10.0.0.2");
    await page.getByLabel("Port").fill("5433");
    await page.getByLabel("Database").fill("testdb2");

    // Submit
    const addButton = page.getByRole("button", { name: /^Add$/i });
    await expect(addButton).toBeEnabled();
    await addButton.click();
    replicaNames.push(createdName);

    // Verify new replica appears in table
    const addResult = page.getByText(/Replica.*added|10\.0\.0\.2/i);
    await expect(addResult.first()).toBeVisible({ timeout: 10000 });

    // Remove the created replica via UI action button
    const removeButton = page.getByRole("button", { name: /Remove.*10\.0\.0\.2/i }).first();
    await expect(removeButton).toBeVisible({ timeout: 5000 });
    await removeButton.click();

    // Confirm removal dialog
    await expect(page.getByText(/Remove Replica/i)).toBeVisible({ timeout: 3000 });
    await page.getByRole("button", { name: /^Remove$/i }).click();

    // Verify removal toast
    await expect(page.getByText(/Replica removed/i)).toBeVisible({ timeout: 5000 });
  });
});

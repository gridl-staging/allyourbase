import {
  test,
  expect,
  probeEndpoint,
  execSQL,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Vector Indexes Lifecycle
 *
 * Critical Path: Seed table with vector column → create index via UI → verify in list
 *
 * Requires pgvector extension to be available. The test enables it,
 * creates a test table with a vector(3) column, then creates an index via UI.
 */

test.describe("Vector Indexes Lifecycle (Full E2E)", () => {
  const tableName = `_test_vectors_${Date.now()}`;

  test.afterEach(async ({ request, adminToken }) => {
    await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${tableName} CASCADE`).catch(() => {});
  });

  test("seed vector table, create index via UI, and verify in list", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/vector/indexes");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Vector Indexes service unavailable (status ${probeStatus})`,
    );

    // Enable pgvector extension and create a test table with a vector column
    try {
      await execSQL(request, adminToken, "CREATE EXTENSION IF NOT EXISTS vector");
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      test.skip(
        /extension "vector" is not available|SQLSTATE 0A000/i.test(message),
        `pgvector extension unavailable in this environment (${message})`,
      );
      throw err;
    }
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (id serial PRIMARY KEY, embedding vector(3))`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Vector Indexes$/i }).click();
    await expect(page.getByRole("heading", { name: /Vector Indexes/i })).toBeVisible({ timeout: 5000 });

    // Create a new vector index via UI
    await page.getByRole("button", { name: /Create Index/i }).click();
    await expect(page.getByRole("heading", { name: /New Vector Index/i })).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Schema").clear();
    await page.getByLabel("Schema").fill("public");
    await page.getByLabel("Table", { exact: true }).fill(tableName);
    await page.getByLabel("Column").fill("embedding");
    await page.getByLabel("Method").selectOption("hnsw");
    await page.getByPlaceholder("cosine").fill("cosine");
    await page.getByRole("button", { name: /^Create$/i }).click();

    // Verify the index appears in the list
    const indexRow = page.getByRole("row", { name: new RegExp(tableName) }).first();
    await expect(indexRow).toBeVisible({ timeout: 10000 });
    await expect(indexRow).toContainText("hnsw");
  });
});

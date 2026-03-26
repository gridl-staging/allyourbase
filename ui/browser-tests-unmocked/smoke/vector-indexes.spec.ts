import {
  test,
  expect,
  probeEndpoint,
  execSQL,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Vector Indexes
 *
 * Critical Path: Navigate to Vector Indexes → Verify page heading, table structure
 * with column headers, and action controls render in the page body.
 */

test.describe("Smoke: Vector Indexes", () => {
  const seededVectorTables: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededVectorTables.length > 0) {
      const tableName = seededVectorTables.pop();
      if (!tableName) continue;
      await execSQL(
        request,
        adminToken,
        `DROP TABLE IF EXISTS public.${tableName} CASCADE`,
      ).catch(() => {});
    }
  });

  test("seeded vector index renders in vector indexes table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/vector/indexes");
    test.skip(
      status === 501 || status === 404,
      `Vector indexes endpoint not available (status ${status})`,
    );

    const vectorExtensionAvailability = await execSQL(
      request,
      adminToken,
      `SELECT EXISTS (
         SELECT 1
         FROM pg_available_extensions
         WHERE name = 'vector'
       )`,
    );
    const isVectorExtensionAvailable = vectorExtensionAvailability.rows[0]?.[0] === true;
    test.skip(!isVectorExtensionAvailable, "vector extension is not available in this Postgres environment");

    const runId = Date.now();
    const tableName = `smoke_vectors_${runId}`;
    const indexName = `smoke_vectors_idx_${runId}`;
    await execSQL(
      request,
      adminToken,
      `CREATE EXTENSION IF NOT EXISTS vector`,
    );
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE public.${tableName} (
         id bigserial PRIMARY KEY,
         embedding vector(3)
       )`,
    );
    await execSQL(
      request,
      adminToken,
      `CREATE INDEX ${indexName}
       ON public.${tableName}
       USING hnsw (embedding vector_l2_ops)`,
    );
    seededVectorTables.push(tableName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Vector Indexes/i }).click();
    await expect(page.getByRole("heading", { name: /Vector Indexes/i })).toBeVisible({ timeout: 15_000 });

    const seededIndexRow = page.locator("tr").filter({ hasText: indexName }).first();
    await expect(seededIndexRow).toBeVisible({ timeout: 5000 });
    await expect(seededIndexRow.getByText("public")).toBeVisible();
    await expect(seededIndexRow.getByText(tableName)).toBeVisible();
    await expect(seededIndexRow.getByText(/hnsw/i)).toBeVisible();
  });
});

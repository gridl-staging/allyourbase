import { test, expect, execSQL } from "../fixtures";

test.describe("Matviews Lifecycle (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded matview registration renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const sourceTable = `mv_seed_source_${runId}`;
    const matviewName = `mv_seed_${runId}`;

    pendingCleanup.push(`DELETE FROM _ayb_matview_refreshes WHERE schema_name = 'public' AND view_name = '${matviewName}'`);
    pendingCleanup.push(`DROP MATERIALIZED VIEW IF EXISTS public.${matviewName}`);
    pendingCleanup.push(`DROP TABLE IF EXISTS public.${sourceTable}`);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE public.${sourceTable} (
        id SERIAL PRIMARY KEY,
        score INTEGER NOT NULL
      )`
    );
    await execSQL(
      request,
      adminToken,
      `INSERT INTO public.${sourceTable} (score) VALUES (10), (20), (30)`
    );
    await execSQL(
      request,
      adminToken,
      `CREATE MATERIALIZED VIEW public.${matviewName} AS
       SELECT count(*)::int AS total_rows, sum(score)::int AS total_score
       FROM public.${sourceTable}`
    );
    await execSQL(
      request,
      adminToken,
      `INSERT INTO _ayb_matview_refreshes (schema_name, view_name, refresh_mode)
       VALUES ('public', '${matviewName}', 'standard')`
    );

    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /^Matviews$/i }).click();
    await expect(page.getByRole("heading", { name: "Materialized Views" })).toBeVisible({ timeout: 5000 });

    const row = page.locator("tr").filter({ hasText: matviewName }).first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row.getByText("standard")).toBeVisible();
  });

  test("registers and refreshes a matview through the UI", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const sourceTable = `mv_refresh_source_${runId}`;
    const matviewName = `mv_refresh_${runId}`;

    pendingCleanup.push(`DELETE FROM _ayb_matview_refreshes WHERE schema_name = 'public' AND view_name = '${matviewName}'`);
    pendingCleanup.push(`DROP MATERIALIZED VIEW IF EXISTS public.${matviewName}`);
    pendingCleanup.push(`DROP TABLE IF EXISTS public.${sourceTable}`);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE public.${sourceTable} (
        id SERIAL PRIMARY KEY,
        points INTEGER NOT NULL
      )`
    );
    await execSQL(
      request,
      adminToken,
      `INSERT INTO public.${sourceTable} (points) VALUES (1), (2), (3), (4)`
    );
    await execSQL(
      request,
      adminToken,
      `CREATE MATERIALIZED VIEW public.${matviewName} AS
       SELECT count(*)::int AS total_rows, sum(points)::int AS total_points
       FROM public.${sourceTable}`
    );

    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /^Matviews$/i }).click();
    await expect(page.getByRole("heading", { name: "Materialized Views" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: "Register Matview" }).click();
    await expect(page.getByText("Register Materialized View")).toBeVisible({ timeout: 3000 });
    await page.getByLabel("View").selectOption(`public.${matviewName}`);
    await page.getByLabel("Refresh Mode").first().selectOption("standard");
    await page.getByRole("button", { name: "Register" }).click();

    const row = page.locator("tr").filter({ hasText: matviewName }).first();
    await expect(row).toBeVisible({ timeout: 5000 });

    await row.getByRole("button", { name: "Refresh" }).click();
    await expect(page.getByText(`Refreshed public.${matviewName}`)).toBeVisible({ timeout: 10000 });
    await expect(row.getByText("success")).toBeVisible({ timeout: 10000 });
  });
});

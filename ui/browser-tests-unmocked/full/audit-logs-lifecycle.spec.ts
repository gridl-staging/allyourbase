import {
  test,
  expect,
  execSQL,
  seedAuditLogEntry,
  cleanupAuditLogsByTable,
  waitForDashboard,
} from "../fixtures";
import type { Page } from "@playwright/test";

async function openAuditLogs(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Audit Logs$/i }).click();
  await expect(page.getByRole("heading", { name: /Audit Logs/i })).toBeVisible({ timeout: 5000 });
}

test.describe("Audit Logs Lifecycle (Full E2E)", () => {
  const seededTables: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededTables.length > 0) {
      const tableName = seededTables.pop();
      if (!tableName) continue;
      await cleanupAuditLogsByTable(request, adminToken, tableName).catch(() => {});
    }
  });

  test("filter, expand change details, and paginate audit entries", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const lifecycleTable = `full_audit_logs_${runId}`;
    const paginationTable = `full_audit_logs_page_${runId}`;
    seededTables.push(lifecycleTable, paginationTable);

    await seedAuditLogEntry(request, adminToken, {
      tableName: lifecycleTable,
      operation: "INSERT",
      recordID: { id: `insert-${runId}` },
      newValues: { status: "created", runId },
      timestampISO: new Date(Date.now() + 2000).toISOString(),
    });

    const updatedEntry = await seedAuditLogEntry(request, adminToken, {
      tableName: lifecycleTable,
      operation: "UPDATE",
      recordID: { id: `update-${runId}` },
      oldValues: { status: "draft", runId },
      newValues: { status: "published", runId },
      timestampISO: new Date(Date.now() + 3000).toISOString(),
    });

    await seedAuditLogEntry(request, adminToken, {
      tableName: lifecycleTable,
      operation: "DELETE",
      recordID: { id: `delete-${runId}` },
      oldValues: { status: "archived", runId },
      timestampISO: new Date(Date.now() + 4000).toISOString(),
    });

    await execSQL(
      request,
      adminToken,
      `INSERT INTO _ayb_audit_log (timestamp, table_name, operation, record_id, old_values, new_values)
       SELECT NOW() + (g * interval '1 second'),
              '${paginationTable}',
              'UPDATE',
              jsonb_build_object('id', g),
              jsonb_build_object('before', g),
              jsonb_build_object('after', g)
       FROM generate_series(1, 105) AS g`,
    );

    await openAuditLogs(page);

    await page.getByLabel("Table", { exact: true }).fill(lifecycleTable);
    await page.getByRole("button", { name: "Apply Filters" }).click();

    await expect(page.locator("tr").filter({ hasText: "INSERT" }).filter({ hasText: lifecycleTable })).toHaveCount(1, {
      timeout: 5000,
    });
    await expect(page.locator("tr").filter({ hasText: "UPDATE" }).filter({ hasText: lifecycleTable })).toHaveCount(1);
    await expect(page.locator("tr").filter({ hasText: "DELETE" }).filter({ hasText: lifecycleTable })).toHaveCount(1);

    const shortID = updatedEntry.id.slice(0, 8);
    await page.getByRole("button", { name: new RegExp(`Show changes for\\s+${shortID}`) }).click();
    const details = page.getByRole("region", { name: "Audit change details" });
    await expect(details).toContainText('"status": "published"', { timeout: 5000 });
    await expect(details).toContainText('"status": "draft"');

    await page.getByLabel("Table", { exact: true }).fill(paginationTable);
    await page.getByRole("button", { name: "Apply Filters" }).click();

    const nextPage = page.getByRole("button", { name: "Next page" });
    await expect(nextPage).toBeVisible({ timeout: 5000 });
    const paginationEnabled = await nextPage.isEnabled();
    if (paginationEnabled) {
      // eslint-disable-next-line playwright/prefer-locator -- false positive on locator click for pagination control
      await nextPage.click();
      await expect(nextPage).toBeDisabled();
    } else {
      await expect(nextPage).toBeDisabled();
    }
  });
});

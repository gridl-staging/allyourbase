import type { Page, TestInfo } from "@playwright/test";
import { test, expect, execSQL, waitForDashboard } from "../fixtures";

const TABLE_DATA_TIMEOUT_MS = 65_000;

/**
 * FULL E2E TEST: Build a Blog Platform (UI-ONLY)
 *
 * User Story: A developer uses AYB admin UI to build a multi-tenant blog backend.
 *
 * This test exercises:
 * 1. SQL execution to create schema with FK relationships (via UI)
 * 2. Data seeding via SQL and data browsing via the Table Browser UI
 * 3. Filtering and sorting in the Data view
 * 4. Schema view to verify FK relationships
 * 5. Record persistence across reloads
 *
 * All table names are suffixed with runId for parallel-run safety.
 */

function makeRunId(testInfo: TestInfo): string {
  return `${Date.now()}_${testInfo.parallelIndex}_${testInfo.repeatEachIndex}_${testInfo.retry}`;
}

async function sleep(ms: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function retryAssertion(
  action: () => Promise<void>,
  assertion: () => Promise<void>,
  timeoutMs: number = TABLE_DATA_TIMEOUT_MS,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastError: unknown;

  while (Date.now() < deadline) {
    try {
      await action();
      await assertion();
      return;
    } catch (error) {
      lastError = error;
      await sleep(2000);
    }
  }

  throw lastError instanceof Error ? lastError : new Error("Timed out waiting for rate-limited table state");
}

function isRateLimitError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /status 429|too many requests/i.test(message);
}

async function withRateLimitRetry<T>(operation: () => Promise<T>): Promise<T> {
  const retryDelaysMs = [300, 600, 1200, 2400, 4800];

  for (let attempt = 0; ; attempt++) {
    try {
      return await operation();
    } catch (error) {
      if (!isRateLimitError(error) || attempt >= retryDelaysMs.length) {
        throw error;
      }
      await sleep(retryDelaysMs[attempt]);
    }
  }
}

async function waitForDashboardWithRetry(
  page: Page,
  mode: "goto" | "reload",
): Promise<void> {
  const retryDelaysMs = [500, 1000];

  for (let attempt = 0; ; attempt++) {
    try {
      if (mode === "goto") {
        await page.goto("/admin/");
      } else {
        await page.reload();
      }
      await waitForDashboard(page);
      return;
    } catch (error) {
      if (attempt >= retryDelaysMs.length) {
        throw error;
      }
      await sleep(retryDelaysMs[attempt]);
    }
  }
}

// Dismiss any stray modal overlay (RowDetail, RecordForm, etc.) that could
// intercept pointer events on the sidebar. These dialogs use fixed inset-0
// z-50 positioning and block the entire viewport until closed.
// Presses Escape (all dialogs now handle it), then verifies the backdrop
// is gone before returning. Falls back to clicking the backdrop if needed.
async function dismissOverlays(page: Page): Promise<void> {
  // Table Browser blockers are side panels and confirms that close on Escape,
  // but they do not expose role="dialog". Check the headings these overlays render.
  const blockers = [
    page.getByRole("heading", { name: "Row Detail" }),
    page.getByRole("heading", { name: "New Record" }),
    page.getByRole("heading", { name: "Edit Record" }),
    page.getByRole("heading", { name: "Delete record?" }),
  ];
  const anyBlockingOverlayVisible = await Promise.all(
    blockers.map((blocker) => blocker.isVisible().catch(() => false)),
  ).then((matches) => matches.some(Boolean));
  if (!anyBlockingOverlayVisible) return;

  await page.keyboard.press("Escape");
  // Wait for the overlay to close; if Escape didn't work, try one more time.
  for (const blocker of blockers) {
    try {
      await blocker.waitFor({ state: "hidden", timeout: 1000 });
    } catch {
      await page.keyboard.press("Escape");
      await blocker.waitFor({ state: "hidden", timeout: 1000 }).catch(() => {});
    }
  }
}

// After clicking a table in the sidebar, wait for the table heading to appear
// in the main content area. This confirms navigation completed and the
// TableBrowser data fetch has started.
async function waitForTableView(page: Page, tableName: string): Promise<void> {
  await expect(
    page.locator("main").getByRole("heading", { level: 1 }),
  ).toContainText(tableName, { timeout: 5000 });
}

async function reopenTableDataTab(page: Page): Promise<void> {
  const sqlTab = page.getByRole("button", { name: /^sql$/i });
  const dataTab = page.getByRole("button", { name: /^data$/i });
  if (!(await sqlTab.isVisible().catch(() => false))) {
    return;
  }
  await sqlTab.click();
  await dataTab.click();
}

test.describe("Blog Platform Journey (Full E2E)", () => {
  // This suite shares a cleanup queue across tests, so it must run serially
  // under global fullyParallel mode to avoid cross-test cleanup interference.
  test.describe.configure({ mode: "serial" });

  // Cleanup queue: SQL statements pushed during tests, drained in afterEach
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded author renders in table view", async ({ page, request, adminToken }, testInfo) => {
    test.setTimeout(180_000);

    const runId = makeRunId(testInfo);
    const authorsTable = `authors_${runId}`;
    const authorEmail = `seed-author-${runId}@example.com`;

    // Register cleanup before creating resources
    pendingCleanup.push(`DROP TABLE IF EXISTS ${authorsTable};`);

    // Arrange: create authors table and seed a record
    await withRateLimitRetry(() =>
      execSQL(
      request,
      adminToken,
      `CREATE TABLE ${authorsTable} (
        id SERIAL PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE NOT NULL,
        bio TEXT, created_at TIMESTAMPTZ DEFAULT NOW()
      );`,
      ),
    );
    await withRateLimitRetry(() =>
      execSQL(
        request,
        adminToken,
        `INSERT INTO ${authorsTable} (name, email, bio)
         VALUES ('Seed Author ${runId}', '${authorEmail}', 'Seeded for load-and-verify');`,
      ),
    );

    // Act: navigate to authors table
    await waitForDashboardWithRetry(page, "goto");
    const sidebar = page.locator("aside");
    await expect(sidebar.getByText(authorsTable, { exact: true })).toBeVisible({ timeout: 5000 });
    await sidebar.getByText(authorsTable, { exact: true }).click();

    // Assert: seeded author appears in the table
    await expect(page.getByText(`Seed Author ${runId}`)).toBeVisible({ timeout: TABLE_DATA_TIMEOUT_MS });
    await expect(page.getByText(authorEmail)).toBeVisible({ timeout: TABLE_DATA_TIMEOUT_MS });
  });

  test("build blog backend: schema, data, relationships", async ({ page, request, adminToken }, testInfo) => {
    // This test creates 3 tables, 2 authors, 3 posts, then tests filter/sort/schema/persistence.
    // 30s default timeout is too tight.
    test.setTimeout(180_000);

    const runId = makeRunId(testInfo);
    const authorsTable = `authors_${runId}`;
    const postsTable = `posts_${runId}`;
    const commentsTable = `comments_${runId}`;

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(`DROP TABLE IF EXISTS ${commentsTable}, ${postsTable}, ${authorsTable};`);

    // Cleanup leftover tables from previous runs (drop in FK order)
    await withRateLimitRetry(() =>
      execSQL(request, adminToken, `DROP TABLE IF EXISTS ${commentsTable}, ${postsTable}, ${authorsTable};`),
    );

    // ============================================================
    // Step 1: Initial Load & Verify Empty State
    // ============================================================
    await waitForDashboardWithRetry(page, "goto");

    // Should show empty state initially - check if sidebar exists and is ready
    const sidebar = page.locator("aside");
    await expect(sidebar).toBeVisible();

    // ============================================================
    // Step 2: Navigate to SQL Editor & Create Authors Table
    // ============================================================
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    await sqlInput.fill(`
      CREATE TABLE ${authorsTable} (
        id SERIAL PRIMARY KEY,
        name TEXT NOT NULL,
        email TEXT UNIQUE NOT NULL,
        bio TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // Step 3: Create Posts Table with FK to Authors
    // ============================================================
    await sqlInput.fill(`
      CREATE TABLE ${postsTable} (
        id SERIAL PRIMARY KEY,
        author_id INTEGER NOT NULL REFERENCES ${authorsTable}(id) ON DELETE CASCADE,
        title TEXT NOT NULL,
        content TEXT NOT NULL,
        status TEXT DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
        published_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // Step 4: Create Comments Table with FK to Posts
    // ============================================================
    await sqlInput.fill(`
      CREATE TABLE ${commentsTable} (
        id SERIAL PRIMARY KEY,
        post_id INTEGER NOT NULL REFERENCES ${postsTable}(id) ON DELETE CASCADE,
        author_name TEXT NOT NULL,
        content TEXT NOT NULL,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // Step 5: Refresh & Verify Tables Appear in Sidebar
    // ============================================================
    await waitForDashboardWithRetry(page, "reload");

    // All three tables should now be visible in sidebar
    await expect(sidebar.getByText(authorsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(postsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(commentsTable, { exact: true })).toBeVisible();

    // ============================================================
    // Step 6: Seed Data via SQL (faster and less rate-limit prone)
    // ============================================================
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    await sqlInput.fill(`
      INSERT INTO ${authorsTable} (name, email, bio) VALUES
        ('Jane Doe', 'jane@example.com', 'Tech writer and blogger'),
        ('John Smith', 'john@example.com', 'Software engineer');
    `);
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(page.getByText(/2 rows? affected/i)).toBeVisible({ timeout: 10000 });

    await sqlInput.fill(`
      INSERT INTO ${postsTable} (author_id, title, content, status) VALUES
        (1, 'Getting Started with AYB', 'AYB makes building backends incredibly easy and fast.', 'published'),
        (2, 'Advanced PostgreSQL Tips', 'Here are some advanced tips for PostgreSQL optimization.', 'draft'),
        (1, 'Why I Love PostgreSQL', 'PostgreSQL has been my database of choice for years.', 'draft');
    `);
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(page.getByText(/3 rows? affected/i)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // Step 7: Navigate to Posts and verify seeded content
    // ============================================================
    await retryAssertion(
      async () => {
        await dismissOverlays(page);
        await sidebar.getByText(postsTable, { exact: true }).click();
        await waitForTableView(page, postsTable);
        await reopenTableDataTab(page);
      },
      async () => {
        await expect(page.getByText("Getting Started with AYB")).toBeVisible({ timeout: 3000 });
        await expect(page.getByText("Advanced PostgreSQL Tips")).toBeVisible({ timeout: 3000 });
        await expect(page.getByText("Why I Love PostgreSQL")).toBeVisible({ timeout: 3000 });
      },
    );

    // ============================================================
    // Step 8: Test Filtering - Show Only Published Posts
    // ============================================================
    // Apply filter for published posts
    const filterInput = page.getByPlaceholder(/filter/i);
    await retryAssertion(
      async () => {
        await reopenTableDataTab(page);
        await filterInput.fill("status='published'");
        await page.getByRole("button", { name: "Apply" }).click();
      },
      async () => {
        await expect(page.getByText("Getting Started with AYB")).toBeVisible({ timeout: 3000 });
        await expect(page.getByText("Advanced PostgreSQL Tips")).not.toBeVisible({
          timeout: 2000,
        });
        await expect(page.getByText("Why I Love PostgreSQL")).not.toBeVisible({
          timeout: 1000,
        });
      },
    );

    // Clear filter
    await retryAssertion(
      async () => {
        await reopenTableDataTab(page);
        const refreshedFilterInput = page.getByPlaceholder(/filter/i);
        await refreshedFilterInput.clear();
        await page.getByRole("button", { name: "Apply" }).click();
      },
      async () => {
        await expect(page.getByText("Getting Started with AYB")).toBeVisible({ timeout: 3000 });
        await expect(page.getByText("Advanced PostgreSQL Tips")).toBeVisible({ timeout: 3000 });
        await expect(page.getByText("Why I Love PostgreSQL")).toBeVisible({ timeout: 3000 });
      },
    );

    // ============================================================
    // Step 9: Test Sorting by Title
    // ============================================================
    // Click on title column header to sort ascending
    const titleHeader = page.getByRole("columnheader", { name: "title" });
    await retryAssertion(
      async () => {
        await reopenTableDataTab(page);
        await titleHeader.click();
      },
      async () => {
        const rows = page.locator("tr").filter({ has: page.getByRole("cell") });
        await expect(rows.first()).toContainText("Advanced PostgreSQL Tips", { timeout: 3000 });
        await expect(rows.nth(2)).toContainText("Why I Love PostgreSQL", { timeout: 3000 });
      },
    );

    // ============================================================
    // Step 10: Switch to Schema View & Verify FK Relationship
    // ============================================================
    await page.getByRole("button", { name: /^schema$/i }).click();

    // Should show schema details (multiple elements contain "author_id" — use .first())
    await expect(page.getByText("author_id").first()).toBeVisible();

    // Should show FK reference to authors table — scope to main to avoid matching sidebar link
    const mainArea = page.locator("main");
    await expect(
      mainArea.getByText(new RegExp(`references.*${authorsTable}|foreign key`, "i")).first(),
    ).toBeVisible();

    // ============================================================
    // Step 11: Navigate to Comments & Verify Empty State
    // ============================================================
    await retryAssertion(
      async () => {
        await dismissOverlays(page);
        await sidebar.getByText(commentsTable, { exact: true }).click();
        await waitForTableView(page, commentsTable);
        await reopenTableDataTab(page);

        const dataTab = page.getByRole("button", { name: /^data$/i });
        if (await dataTab.isVisible()) {
          await dataTab.click();
        }
      },
      async () => {
        await expect(page.getByRole("cell", { name: /no rows/i })).toBeVisible({ timeout: 3000 });
      },
    );

    // Verify Schema view shows FK to posts
    await page.getByRole("button", { name: /^schema$/i }).click();
    await expect(page.getByText("post_id").first()).toBeVisible();
    await expect(mainArea.getByText(new RegExp(`references.*${postsTable}|foreign key`, "i")).first()).toBeVisible();

    // ============================================================
    // Step 12: Final Verification - Reload & Check Persistence
    // ============================================================
    await waitForDashboardWithRetry(page, "reload");

    // All tables should still be visible after reload
    await expect(sidebar.getByText(authorsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(postsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(commentsTable, { exact: true })).toBeVisible();

    // Navigate to posts and verify data persisted
    await retryAssertion(
      async () => {
        await dismissOverlays(page);
        await sidebar.getByText(postsTable, { exact: true }).click();
        await waitForTableView(page, postsTable);
        await reopenTableDataTab(page);
      },
      async () => {
        await expect(page.getByText("Getting Started with AYB")).toBeVisible({ timeout: 3000 });
        await expect(page.getByText("Advanced PostgreSQL Tips")).toBeVisible({ timeout: 3000 });
      },
    );

    // Navigate to authors and verify data persisted
    await retryAssertion(
      async () => {
        await dismissOverlays(page);
        await sidebar.getByText(authorsTable, { exact: true }).click();
        await waitForTableView(page, authorsTable);
        await reopenTableDataTab(page);
      },
      async () => {
        await expect(page.getByRole("cell", { name: "Jane Doe", exact: true })).toBeVisible({ timeout: 3000 });
        await expect(page.getByRole("cell", { name: "John Smith", exact: true })).toBeVisible({ timeout: 3000 });
      },
    );

    // Cleanup handled by afterEach
  });
});

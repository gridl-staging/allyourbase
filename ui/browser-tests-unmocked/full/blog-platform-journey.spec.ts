import { test, expect, execSQL, seedRecord } from "../fixtures";

/**
 * FULL E2E TEST: Build a Blog Platform (UI-ONLY)
 *
 * User Story: A developer uses AYB admin UI to build a multi-tenant blog backend.
 *
 * This test exercises:
 * 1. SQL execution to create schema with FK relationships (via UI)
 * 2. Data insertion via the Table Browser UI
 * 3. Filtering and sorting in the Data view
 * 4. Schema view to verify FK relationships
 * 5. Record persistence across reloads
 *
 * All table names are suffixed with runId for parallel-run safety.
 */

test.describe("Blog Platform Journey (Full E2E)", () => {
  // Cleanup queue: SQL statements pushed during tests, drained in afterEach
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded author renders in table view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const authorsTable = `authors_${runId}`;
    const authorEmail = `seed-author-${runId}@example.com`;

    // Register cleanup before creating resources
    pendingCleanup.push(`DROP TABLE IF EXISTS ${authorsTable};`);

    // Arrange: create authors table and seed a record
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${authorsTable} (
        id SERIAL PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE NOT NULL,
        bio TEXT, created_at TIMESTAMPTZ DEFAULT NOW()
      );`,
    );
    await seedRecord(request, adminToken, authorsTable, {
      name: `Seed Author ${runId}`,
      email: authorEmail,
      bio: "Seeded for load-and-verify",
    });

    // Act: navigate to authors table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const sidebar = page.locator("aside");
    await expect(sidebar.getByText(authorsTable, { exact: true })).toBeVisible({ timeout: 5000 });
    await sidebar.getByText(authorsTable, { exact: true }).click();

    // Assert: seeded author appears in the table
    await expect(page.getByText(`Seed Author ${runId}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(authorEmail)).toBeVisible();
  });

  test("build blog backend: schema, data, relationships", async ({ page, request, adminToken }) => {
    // This test creates 3 tables, 2 authors, 3 posts, then tests filter/sort/schema/persistence.
    // 30s default timeout is too tight.
    test.setTimeout(90_000);

    const runId = Date.now();
    const authorsTable = `authors_${runId}`;
    const postsTable = `posts_${runId}`;
    const commentsTable = `comments_${runId}`;

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(`DROP TABLE IF EXISTS ${commentsTable}, ${postsTable}, ${authorsTable};`);

    // Cleanup leftover tables from previous runs (drop in FK order)
    await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${commentsTable}, ${postsTable}, ${authorsTable};`);

    // ============================================================
    // Step 1: Initial Load & Verify Empty State
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

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
    await page.getByRole("button", { name: /run|execute/i }).click();
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
    await page.getByRole("button", { name: /run|execute/i }).click();
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
    await page.getByRole("button", { name: /run|execute/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // Step 5: Refresh & Verify Tables Appear in Sidebar
    // ============================================================
    await page.reload();
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // All three tables should now be visible in sidebar
    await expect(sidebar.getByText(authorsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(postsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(commentsTable, { exact: true })).toBeVisible();

    // ============================================================
    // Step 6: Navigate to Authors Table & Add First Author
    // ============================================================
    await sidebar.getByText(authorsTable, { exact: true }).click();

    // Should show Data tab by default
    await expect(page.getByRole("button", { name: /data/i })).toBeVisible();

    // Should show empty table initially
    await expect(page.getByRole("cell", { name: /no rows/i })).toBeVisible();

    // Click "New Row" button to create author
    await page.getByRole("button", { name: "New Row" }).click();

    // Form modal should open with "New Record" title
    await expect(page.getByText("New Record")).toBeVisible();

    // Fill in author fields
    await page.getByLabel("name").fill("Jane Doe");
    await page.getByLabel("email").fill("jane@example.com");
    await page.getByLabel("bio").fill("Tech writer and blogger");

    // Submit the form
    await page.getByRole("button", { name: "Create" }).click();

    // Verify author appears in the table
    await expect(page.getByText("Jane Doe")).toBeVisible();
    await expect(page.getByText("jane@example.com")).toBeVisible();

    // ============================================================
    // Step 7: Add Second Author
    // ============================================================
    await page.getByRole("button", { name: "New Row" }).click();
    await expect(page.getByText("New Record")).toBeVisible();

    await page.getByLabel("name").fill("John Smith");
    await page.getByLabel("email").fill("john@example.com");
    await page.getByLabel("bio").fill("Software engineer");

    await page.getByRole("button", { name: "Create" }).click();

    await expect(page.getByText("John Smith")).toBeVisible();
    await expect(page.getByText("john@example.com")).toBeVisible();

    // ============================================================
    // Step 8: Navigate to Posts & Create Posts with FK References
    // ============================================================
    await sidebar.getByText(postsTable, { exact: true }).click();

    // Empty posts table initially
    await expect(page.getByRole("cell", { name: /no rows/i })).toBeVisible();

    // Create first post (by Jane, published)
    await page.getByRole("button", { name: "New Row" }).click();
    await expect(page.getByText("New Record")).toBeVisible();

    // author_id = 1 (Jane Doe)
    await page.getByLabel("author_id").fill("1");
    await page.getByLabel("title").fill("Getting Started with AYB");
    await page.getByLabel("content").fill("AYB makes building backends incredibly easy and fast.");
    await page.getByLabel("status").fill("published");

    await page.getByRole("button", { name: "Create" }).click();

    await expect(page.getByText("Getting Started with AYB")).toBeVisible();

    // Create second post (by John, draft)
    await page.getByRole("button", { name: "New Row" }).click();

    await page.getByLabel("author_id").fill("2");
    await page.getByLabel("title").fill("Advanced PostgreSQL Tips");
    await page.getByLabel("content").fill("Here are some advanced tips for PostgreSQL optimization.");
    await page.getByLabel("status").fill("draft");

    await page.getByRole("button", { name: "Create" }).click();

    await expect(page.getByText("Advanced PostgreSQL Tips")).toBeVisible();

    // Create third post (by Jane, draft)
    await page.getByRole("button", { name: "New Row" }).click();

    await page.getByLabel("author_id").fill("1");
    await page.getByLabel("title").fill("Why I Love PostgreSQL");
    await page.getByLabel("content").fill("PostgreSQL has been my database of choice for years.");
    await page.getByLabel("status").fill("draft");

    await page.getByRole("button", { name: "Create" }).click();

    await expect(page.getByText("Why I Love PostgreSQL")).toBeVisible();

    // ============================================================
    // Step 9: Test Filtering - Show Only Published Posts
    // ============================================================
    // All 3 posts should be visible initially
    await expect(page.getByText("Getting Started with AYB")).toBeVisible();
    await expect(page.getByText("Advanced PostgreSQL Tips")).toBeVisible();
    await expect(page.getByText("Why I Love PostgreSQL")).toBeVisible();

    // Apply filter for published posts
    const filterInput = page.getByPlaceholder(/filter/i);
    await filterInput.fill("status='published'");
    await page.getByRole("button", { name: "Apply" }).click();

    // Only published post should be visible
    await expect(page.getByText("Getting Started with AYB")).toBeVisible();

    // Draft posts should not be visible (with a timeout to ensure filter applied)
    await expect(page.getByText("Advanced PostgreSQL Tips")).not.toBeVisible({
      timeout: 2000,
    });
    await expect(page.getByText("Why I Love PostgreSQL")).not.toBeVisible({
      timeout: 1000,
    });

    // Clear filter
    await filterInput.clear();
    await page.getByRole("button", { name: "Apply" }).click();

    // All posts visible again
    await expect(page.getByText("Getting Started with AYB")).toBeVisible();
    await expect(page.getByText("Advanced PostgreSQL Tips")).toBeVisible();
    await expect(page.getByText("Why I Love PostgreSQL")).toBeVisible();

    // ============================================================
    // Step 10: Test Sorting by Title
    // ============================================================
    // Click on title column header to sort ascending
    const titleHeader = page.getByRole("columnheader", { name: "title" });
    await titleHeader.click();

    // Verify sort ORDER — first data row must be alphabetically first
    // "Advanced PostgreSQL Tips" < "Getting Started with AYB" < "Why I Love PostgreSQL"
    const rows = page.locator("tr").filter({ has: page.getByRole("cell") });
    await expect(rows.first()).toContainText("Advanced PostgreSQL Tips", { timeout: 3000 });
    await expect(rows.nth(2)).toContainText("Why I Love PostgreSQL");

    // ============================================================
    // Step 11: Switch to Schema View & Verify FK Relationship
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
    // Step 12: Navigate to Comments & Verify Empty State
    // ============================================================
    await sidebar.getByText(commentsTable, { exact: true }).click();

    // Switch to Data tab if not already there
    const dataTab = page.getByRole("button", { name: /^data$/i });
    if (await dataTab.isVisible()) {
      await dataTab.click();
    }

    // Should show empty table
    await expect(page.getByRole("cell", { name: /no rows/i })).toBeVisible();

    // Verify Schema view shows FK to posts
    await page.getByRole("button", { name: /^schema$/i }).click();
    await expect(page.getByText("post_id").first()).toBeVisible();
    await expect(mainArea.getByText(new RegExp(`references.*${postsTable}|foreign key`, "i")).first()).toBeVisible();

    // ============================================================
    // Step 13: Final Verification - Reload & Check Persistence
    // ============================================================
    await page.reload();
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // All tables should still be visible after reload
    await expect(sidebar.getByText(authorsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(postsTable, { exact: true })).toBeVisible();
    await expect(sidebar.getByText(commentsTable, { exact: true })).toBeVisible();

    // Navigate to posts and verify data persisted
    await sidebar.getByText(postsTable, { exact: true }).click();
    await expect(page.getByText("Getting Started with AYB")).toBeVisible();
    await expect(page.getByText("Advanced PostgreSQL Tips")).toBeVisible();

    // Navigate to authors and verify data persisted
    await sidebar.getByText(authorsTable, { exact: true }).click();
    await expect(page.getByText("Jane Doe")).toBeVisible();
    await expect(page.getByText("John Smith")).toBeVisible();

    // Cleanup handled by afterEach
  });
});

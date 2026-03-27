import type { Locator, Page } from "@playwright/test";
import {
  test,
  expect,
  execSQL,
  seedRecord,
  sqlLiteral,
  waitForDashboard,
} from "../fixtures";

type DeliveryHistorySummary = {
  rowCount: number;
  attempts: number[];
  hasNextButton: boolean;
  detailChecksPassed: boolean;
};

function emptyDeliveryHistorySummary(rowCount: number): DeliveryHistorySummary {
  return {
    rowCount,
    attempts: [],
    hasNextButton: false,
    detailChecksPassed: false,
  };
}

async function closeModalIfVisible(closeButton: Locator): Promise<void> {
  if (await closeButton.isVisible().catch(() => false)) {
    await closeButton.click();
  }
}

async function refreshDeliveryHistoryModal(params: {
  historyHeading: Locator;
  historyButton: Locator;
  closeButton: Locator;
}): Promise<boolean> {
  const { historyHeading, historyButton, closeButton } = params;
  if (await historyHeading.isVisible().catch(() => false)) {
    return true;
  }
  await closeModalIfVisible(closeButton);
  await historyButton.click();
  return historyHeading
    .waitFor({ state: "visible", timeout: 5000 })
    .then(() => true)
    .catch(() => false);
}

async function collectDeliveryHistorySummary(params: {
  page: Page;
  tableName: string;
  historyButton: Locator;
  historyHeading: Locator;
  closeButton: Locator;
  expectedAttemptCount: number;
}): Promise<DeliveryHistorySummary> {
  const { page, tableName, historyButton, historyHeading, closeButton, expectedAttemptCount } = params;

  const opened = await refreshDeliveryHistoryModal({
    historyHeading,
    historyButton,
    closeButton,
  });
  if (!opened) {
    return emptyDeliveryHistorySummary(-1);
  }

  await page.getByText("Loading deliveries...").waitFor({ state: "hidden", timeout: 5000 }).catch(() => {});

  const modalCandidate = historyHeading.locator("xpath=ancestor::div[contains(@class,'rounded-lg')][1]");
  const modal = await modalCandidate.count().then((count) => (count > 0 ? modalCandidate : page.locator("main")));
  const deliveryRows = modal
    .getByRole("button")
    .filter({ hasText: tableName });
  const rowCount = await deliveryRows.count();
  if (rowCount !== expectedAttemptCount) {
    return emptyDeliveryHistorySummary(rowCount);
  }

  const attempts: number[] = [];
  let detailChecksPassed = true;

  for (let index = 0; index < expectedAttemptCount; index++) {
    const row = deliveryRows.nth(index);
    await row.click();
    const detail = row.locator("xpath=following-sibling::*[1]");
    const detailVisible = await detail
      .waitFor({ state: "visible", timeout: 5000 })
      .then(() => true)
      .catch(() => false);
    if (!detailVisible) {
      detailChecksPassed = false;
      continue;
    }
    const detailText = (await detail.textContent()) ?? "";
    const attemptMatch = detailText.match(/Attempt:\s*(\d+)/);
    if (!attemptMatch) {
      detailChecksPassed = false;
      continue;
    }
    attempts.push(Number(attemptMatch[1]));

    if (!/Status:\s*404\b/.test(detailText)) {
      detailChecksPassed = false;
    }
  }

  attempts.sort((a, b) => b - a);

  const hasNextButton = await modal.getByRole("button", { name: "Next" }).isVisible().catch(() => false);
  await closeModalIfVisible(closeButton);

  return {
    rowCount,
    attempts,
    hasNextButton,
    detailChecksPassed,
  };
}

test.describe("Webhook Delivery History Journey (Full E2E)", () => {
  test.setTimeout(90_000);

  let tableName = "";
  let webhookUrl = "";

  test("collectDeliveryHistorySummary scopes expanded details to clicked row", async ({ page }) => {
    const syntheticTableName = "stage4_webhook_table";

    await page.setContent(`
      <main>
        <h2>Delivery History</h2>
        <button type="button" aria-label="Close">Close</button>
        <div>
          <button type="button" id="delivery-row-1">404 create ${syntheticTableName}</button>
          <section id="details-1" hidden>
            <p>Attempt: 2</p>
            <p>Status: 404</p>
            <h3>Request Body</h3>
            <h3>Response Body</h3>
          </section>
        </div>
        <div>
          <button type="button" id="delivery-row-2">404 create ${syntheticTableName}</button>
          <section id="details-2" hidden>
            <p>Attempt: 1</p>
            <p>Status: 404</p>
            <h3>Request Body</h3>
            <h3>Response Body</h3>
          </section>
        </div>
      </main>
      <script>
        document.getElementById("delivery-row-1")?.addEventListener("click", () => {
          const details = document.getElementById("details-1");
          if (details) details.hidden = false;
        });
        document.getElementById("delivery-row-2")?.addEventListener("click", () => {
          const details = document.getElementById("details-2");
          if (details) details.hidden = false;
        });
      </script>
    `);

    const historyHeading = page.getByRole("heading", { name: /Delivery History/i });
    const closeButton = page.getByRole("button", { name: "Close" });
    const historyButton = page.getByRole("button", { name: "Nonexistent opener" });
    const summary = await collectDeliveryHistorySummary({
      page,
      tableName: syntheticTableName,
      historyButton,
      historyHeading,
      closeButton,
      expectedAttemptCount: 2,
    });

    expect(summary).toEqual({
      rowCount: 2,
      attempts: [2, 1],
      hasNextButton: false,
      detailChecksPassed: true,
    });
  });

  test.afterEach(async ({ request, adminToken }) => {
    if (tableName) {
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${tableName};`).catch(() => {});
    }

    if (webhookUrl) {
      await execSQL(
        request,
        adminToken,
        `DELETE FROM _ayb_webhooks WHERE url = '${sqlLiteral(webhookUrl)}';`,
      ).catch(() => {});
    }

    tableName = "";
    webhookUrl = "";
  });

  test("record insert appears in delivery history modal", async ({ page, request, adminToken }, testInfo) => {
    const runId = `${Date.now()}_${testInfo.workerIndex}`;
    tableName = `stage4_webhook_delivery_${runId}`;

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS ${tableName} (id BIGSERIAL PRIMARY KEY, title TEXT NOT NULL);`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    const origin = new URL(page.url()).origin;
    webhookUrl = `${origin}/__missing-webhook-target__/${runId}`;

    const webhooksButton = page.locator("aside").getByRole("button", { name: /^Webhooks$/i });
    await webhooksButton.click();
    await expect(page.getByRole("heading", { name: /Webhooks/i })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Add Webhook/i }).click();
    await page.getByRole("textbox", { name: /^URL/i }).fill(webhookUrl);
    await page.getByRole("textbox", { name: /^Tables$/i }).fill(tableName);
    await page.getByRole("button", { name: /^Create$/i }).click();

    const webhookRow = page.locator("tr").filter({ hasText: webhookUrl }).first();
    await expect(webhookRow).toContainText(tableName, { timeout: 5000 });

    await seedRecord(request, adminToken, tableName, {
      title: `delivery-${runId}`,
    });

    const historyButton = webhookRow.getByRole("button", { name: "Delivery History" });
    const historyHeading = page.getByRole("heading", { name: /Delivery History/i });
    const expectedAttempts = [4, 3, 2, 1];

    // AYB retries webhook deliveries with production backoff (1s, 5s, 25s),
    // so the full history can legitimately take a little over 30 seconds to appear.
    await expect.poll(async () => {
      const result = await execSQL(
        request,
        adminToken,
        `SELECT COUNT(*) FROM _ayb_webhook_deliveries d
         JOIN _ayb_webhooks w ON w.id = d.webhook_id
         WHERE w.url = '${sqlLiteral(webhookUrl)}';`,
      );
      return Number(result.rows[0]?.[0] ?? 0);
    }, { timeout: 60000 }).toBe(expectedAttempts.length);

    await historyButton.click();
    await expect(historyHeading).toBeVisible({ timeout: 5000 });
    await page.getByText("Loading deliveries...").waitFor({ state: "hidden", timeout: 5000 }).catch(() => {});

    const modal = historyHeading.locator("xpath=ancestor::div[contains(@class,'rounded-lg')][1]");
    const deliveryRows = modal.getByRole("button").filter({ hasText: tableName });
    await expect(deliveryRows).toHaveCount(expectedAttempts.length, { timeout: 5000 });
    await expect(modal.getByRole("button", { name: "Next" })).toBeHidden();

    const attempts: number[] = [];
    for (let index = 0; index < expectedAttempts.length; index++) {
      const row = deliveryRows.nth(index);
      const text = (await row.textContent()) ?? "";
      expect(text).toContain(tableName);
      expect(text).toContain("404");
      await row.click();
      const detail = row.locator("xpath=following-sibling::div[1]");
      await expect(detail).toBeVisible({ timeout: 5000 });
      if (index === 0) {
        await expect(detail).toContainText("Attempt:");
        await expect(detail).toContainText("Status: 404");
      }
      const detailText = (await detail.textContent()) ?? "";
      const attemptMatch = detailText.match(/Attempt:\s*(\d+)/);
      expect(attemptMatch).not.toBeNull();
      attempts.push(Number(attemptMatch?.[1]));
    }

    attempts.sort((a, b) => b - a);
    expect(attempts).toEqual(expectedAttempts);
  });
});

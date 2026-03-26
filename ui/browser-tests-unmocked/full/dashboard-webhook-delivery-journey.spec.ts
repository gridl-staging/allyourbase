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

type DeliveryDetailSnapshot = {
  attempt: number | null;
  hasMissingTargetStatus: boolean;
  hasRequestBody: boolean;
  hasResponseBody: boolean;
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

async function openDeliveryHistoryModal(historyHeading: Locator, historyButton: Locator): Promise<boolean> {
  const modalVisible = await historyHeading.isVisible().catch(() => false);
  if (!modalVisible) {
    await historyButton.click();
  }
  return historyHeading
    .waitFor({ state: "visible", timeout: 5000 })
    .then(() => true)
    .catch(() => false);
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function collectDeliveryDetailSnapshots(params: {
  ariaSnapshot: string;
  tableName: string;
}): DeliveryDetailSnapshot[] {
  const { ariaSnapshot, tableName } = params;
  const lines = ariaSnapshot.split("\n");
  const escapedTableName = escapeRegExp(tableName);
  const rowButtonPattern = new RegExp(
    `button\\s+\"[^\"\\n]*404[^\"\\n]*create[^\"\\n]*${escapedTableName}[^\"\\n]*\"`,
    "i",
  );

  const rowBlocks: string[] = [];
  let activeRowIndex = -1;

  for (const line of lines) {
    if (rowButtonPattern.test(line)) {
      rowBlocks.push("");
      activeRowIndex = rowBlocks.length - 1;
      continue;
    }

    if (activeRowIndex >= 0) {
      rowBlocks[activeRowIndex] += `${line}\n`;
    }
  }

  return rowBlocks.map((block) => {
    const attemptMatch = block.match(/Attempt:\s*(\d+)/);
    return {
      attempt: attemptMatch ? Number(attemptMatch[1]) : null,
      hasMissingTargetStatus: /Status:\s*404\b/.test(block),
      hasRequestBody: /Request Body/.test(block),
      hasResponseBody: /Response Body/.test(block),
    };
  });
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

  const opened = await openDeliveryHistoryModal(historyHeading, historyButton);
  if (!opened) {
    return emptyDeliveryHistorySummary(-1);
  }

  const deliveryRows = page
    .getByRole("button", {
      name: /404.*create/i,
    })
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

    const ariaSnapshot = await page.locator("main").ariaSnapshot().catch(() => null);
    if (!ariaSnapshot) {
      detailChecksPassed = false;
      continue;
    }

    const rowDetails = collectDeliveryDetailSnapshots({
      ariaSnapshot,
      tableName,
    });
    const detail = rowDetails[index];
    if (!detail || detail.attempt === null) {
      detailChecksPassed = false;
      continue;
    }

    attempts.push(detail.attempt);

    if (!detail.hasMissingTargetStatus || !detail.hasRequestBody || !detail.hasResponseBody) {
      detailChecksPassed = false;
    }
  }

  const hasNextButton = await page.getByRole("button", { name: "Next" }).isVisible().catch(() => false);
  await closeModalIfVisible(closeButton);

  return {
    rowCount,
    attempts,
    hasNextButton,
    detailChecksPassed,
  };
}

test.describe("Webhook Delivery History Journey (Full E2E)", () => {
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
    const closeButton = page.getByRole("button", { name: "Close" });
    const expectedAttempts = [4, 3, 2, 1];

    await expect.poll(async () => collectDeliveryHistorySummary({
      page,
      tableName,
      historyButton,
      historyHeading,
      closeButton,
      expectedAttemptCount: expectedAttempts.length,
    }), { timeout: 30000 }).toEqual({
      rowCount: 4,
      attempts: [4, 3, 2, 1],
      hasNextButton: false,
      detailChecksPassed: true,
    });
  });
});

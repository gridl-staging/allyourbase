import { test, expect, type Browser, type Page, type BrowserContext } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  uniqueName,
} from "./helpers";

/** Set up two tabs viewing the same board. Returns context, page1, page2. */
async function setupTwoTabs(
  browser: Browser,
  boardName: string,
): Promise<{ context: BrowserContext; page1: Page; page2: Page }> {
  const context = await browser.newContext();
  const page1 = await context.newPage();
  await registerUser(page1);
  await createBoard(page1, boardName);
  await openBoard(page1, boardName);

  const page2 = await context.newPage();
  await page2.goto("/");
  await expect(page2.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
  await page2.getByText(boardName).first().click();
  await expect(
    page2.getByRole("heading", { name: boardName }),
  ).toBeVisible({ timeout: 5000 });

  return { context, page1, page2 };
}

test.describe("Realtime SSE", () => {
  test("card created in one tab appears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("RT Board"));
    await addColumn(page1, "Column A");

    await addCard(page1, "Column A", "Realtime Card");
    await expect(page1.getByText("Realtime Card")).toBeVisible();

    await expect(page2.getByText("Realtime Card")).toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("card deleted in one tab disappears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("DelSync"));
    await addColumn(page1, "Col");
    await addCard(page1, "Col", "Will Be Deleted");

    await expect(page2.getByText("Will Be Deleted")).toBeVisible({
      timeout: 5000,
    });

    // Delete the card in tab 1.
    await page1.getByText("Will Be Deleted").click();
    await expect(page1.getByText("Edit Card")).toBeVisible();
    page1.on("dialog", (dialog) => dialog.accept());
    await page1.getByText("Delete card").click();
    await expect(page1.getByText("Will Be Deleted")).toBeHidden();

    await expect(page2.getByText("Will Be Deleted")).not.toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("column created in one tab appears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("ColSync"));

    await addColumn(page1, "Realtime Column");
    await expect(page1.getByText("Realtime Column")).toBeVisible();

    await expect(page2.getByText("Realtime Column")).toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("card updated in one tab updates in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("UpdSync"));
    await addColumn(page1, "Col");
    await addCard(page1, "Col", "Original Name");
    await expect(page2.getByText("Original Name")).toBeVisible({ timeout: 5000 });

    // Edit the card in tab 1.
    await page1.getByText("Original Name").click();
    await expect(page1.getByText("Edit Card")).toBeVisible();
    const modal = page1.getByRole("dialog");
    const titleInput = modal.getByLabel("Title");
    await titleInput.clear();
    await titleInput.fill("Renamed Card");
    await page1.getByRole("button", { name: "Save" }).click();
    await expect(page1.getByText("Renamed Card")).toBeVisible();

    await expect(page2.getByText("Renamed Card")).toBeVisible({
      timeout: 10000,
    });
    await expect(page2.getByText("Original Name")).toBeHidden();

    await context.close();
  });

  test("column deleted in one tab disappears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("ColDelSync"));
    await addColumn(page1, "Ephemeral");
    await expect(page2.getByText("Ephemeral")).toBeVisible({ timeout: 5000 });

    // Delete column in tab 1.
    page1.on("dialog", (dialog) => dialog.accept());
    await page1.getByRole("button", { name: "Delete column Ephemeral" }).click();
    await expect(page1.getByText("Ephemeral")).toBeHidden();

    await expect(page2.getByText("Ephemeral")).not.toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });
});

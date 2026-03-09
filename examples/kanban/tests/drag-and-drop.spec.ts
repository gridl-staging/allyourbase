import { test, expect } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  uniqueName,
} from "./helpers";

// NOTE: Drag-and-drop tests require low-level mouse APIs (boundingBox, mouse.move/down/up)
// and library-specific data attributes ([data-rfd-*]) because Playwright has no native
// DnD API that works with @hello-pangea/dnd. These patterns are pragmatically necessary
// and documented as exceptions to BROWSER_TESTING_STANDARDS_2.md Rule 4/5.

test.describe("Drag and Drop", () => {
  test.beforeEach(async ({ page }) => {
    const boardName = uniqueName("DnD");
    await registerUser(page);
    await createBoard(page, boardName);
    await openBoard(page, boardName);
    await addColumn(page, "To Do");
    await addColumn(page, "Done");
    await addCard(page, "To Do", "Drag Me");
  });

  test("can drag a card between columns", async ({ page }) => {
    const card = page.getByText("Drag Me");

    // eslint-disable-next-line playwright/no-raw-locators -- @hello-pangea/dnd library data attributes
    const destColumn = page.locator("[data-rfd-droppable-id]").nth(1);

    const cardBox = await card.boundingBox();
    const destBox = await destColumn.boundingBox();

    expect(cardBox).toBeTruthy();
    expect(destBox).toBeTruthy();

    await page.mouse.move(
      cardBox!.x + cardBox!.width / 2,
      cardBox!.y + cardBox!.height / 2,
    );
    await page.mouse.down();

    await page.mouse.move(
      destBox!.x + destBox!.width / 2,
      destBox!.y + destBox!.height / 2,
      { steps: 10 },
    );
    await page.mouse.up();

    await expect(page.getByText("Drag Me")).toBeVisible();

    const todoCount = page.getByTestId("column-To Do").getByTestId("card-count");
    const doneCount = page.getByTestId("column-Done").getByTestId("card-count");
    await expect(todoCount).toHaveText("0", { timeout: 5000 });
    await expect(doneCount).toHaveText("1", { timeout: 5000 });
  });

  test("multiple cards can be dragged to different columns", async ({ page }) => {
    await addCard(page, "To Do", "Card 2");
    await addCard(page, "To Do", "Card 3");

    const todoCount = page.getByTestId("column-To Do").getByTestId("card-count");
    await expect(todoCount).toHaveText("3");

    // Drag "Drag Me" to Done.
    const card = page.getByText("Drag Me");
    // eslint-disable-next-line playwright/no-raw-locators -- @hello-pangea/dnd library data attributes
    const destColumn = page.locator("[data-rfd-droppable-id]").nth(1);

    const cardBox = await card.boundingBox();
    const destBox = await destColumn.boundingBox();
    expect(cardBox).toBeTruthy();
    expect(destBox).toBeTruthy();

    await page.mouse.move(cardBox!.x + cardBox!.width / 2, cardBox!.y + cardBox!.height / 2);
    await page.mouse.down();
    await page.mouse.move(destBox!.x + destBox!.width / 2, destBox!.y + destBox!.height / 2, { steps: 10 });
    await page.mouse.up();

    await expect(todoCount).toHaveText("2", { timeout: 5000 });
    const doneCount = page.getByTestId("column-Done").getByTestId("card-count");
    await expect(doneCount).toHaveText("1", { timeout: 5000 });
  });

  test("droppable count matches column count", async ({ page }) => {
    // eslint-disable-next-line playwright/no-raw-locators -- @hello-pangea/dnd library data attributes
    await expect(page.locator("[data-rfd-droppable-id]")).toHaveCount(2);

    await addColumn(page, "In Progress");
    // eslint-disable-next-line playwright/no-raw-locators -- @hello-pangea/dnd library data attributes
    await expect(page.locator("[data-rfd-droppable-id]")).toHaveCount(3);
  });
});

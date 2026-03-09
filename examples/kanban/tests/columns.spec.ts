import { test, expect } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  uniqueName,
} from "./helpers";

test.describe("Columns", () => {
  let boardName: string;

  test.beforeEach(async ({ page }) => {
    boardName = uniqueName("ColTest");
    await registerUser(page);
    await createBoard(page, boardName);
    await openBoard(page, boardName);
  });

  test("shows add column input on empty board", async ({ page }) => {
    await expect(page.getByPlaceholder("+ Add column...")).toBeVisible();
  });

  test("can add columns", async ({ page }) => {
    await addColumn(page, "To Do");
    await expect(page.getByText("To Do")).toBeVisible();

    await addColumn(page, "In Progress");
    await addColumn(page, "Done");

    await expect(page.getByText("To Do")).toBeVisible();
    await expect(page.getByText("In Progress")).toBeVisible();
    await expect(page.getByText("Done")).toBeVisible();
  });

  test("shows card count in column header", async ({ page }) => {
    await addColumn(page, "To Do");
    // Initially 0 cards — scoped via data-testid
    const cardCount = page.getByTestId("column-To Do").getByTestId("card-count");
    await expect(cardCount).toHaveText("0");
  });

  test("can delete a column", async ({ page }) => {
    await addColumn(page, "Delete Me");
    await expect(page.getByText("Delete Me")).toBeVisible();

    // Accept the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Click the delete button (has aria-label)
    await page.getByRole("button", { name: "Delete column Delete Me" }).click();

    await expect(page.getByText("Delete Me")).toBeHidden();
  });

  test("cancel delete keeps column", async ({ page }) => {
    await addColumn(page, "Keep This");
    await expect(page.getByText("Keep This")).toBeVisible();

    // Dismiss the confirm dialog
    page.on("dialog", (dialog) => dialog.dismiss());

    await page.getByRole("button", { name: "Delete column Keep This" }).click();

    // Column should still be visible
    await expect(page.getByText("Keep This")).toBeVisible();
  });

  test("Add Column button only appears when text is typed", async ({ page }) => {
    // Initially no button
    await expect(page.getByRole("button", { name: "Add Column" })).toBeHidden();

    // Type a column name
    await page.getByPlaceholder("+ Add column...").fill("New Col");
    await expect(page.getByRole("button", { name: "Add Column" })).toBeVisible();

    // Clear the input — button should disappear
    await page.getByPlaceholder("+ Add column...").fill("");
    await expect(page.getByRole("button", { name: "Add Column" })).toBeHidden();
  });

  test("card count updates after adding cards", async ({ page }) => {
    await addColumn(page, "Counting");

    // Start at 0
    const cardCount = page.getByTestId("column-Counting").getByTestId("card-count");
    await expect(cardCount).toHaveText("0");

    await addCard(page, "Counting", "Card 1");
    await expect(cardCount).toHaveText("1");

    await addCard(page, "Counting", "Card 2");
    await expect(cardCount).toHaveText("2");
  });

  test("deleting column removes its cards too", async ({ page }) => {
    await addColumn(page, "Doomed");
    await addCard(page, "Doomed", "Card X");
    await addCard(page, "Doomed", "Card Y");

    await expect(page.getByText("Card X")).toBeVisible();
    await expect(page.getByText("Card Y")).toBeVisible();

    // Delete the column
    page.on("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "Delete column Doomed" }).click();

    // Column and its cards should be gone
    await expect(page.getByText("Doomed")).toBeHidden();
    await expect(page.getByText("Card X")).toBeHidden();
    await expect(page.getByText("Card Y")).toBeHidden();
  });

  test("columns persist after page reload", async ({ page }) => {
    await addColumn(page, "Persistent Col");
    await expect(page.getByText("Persistent Col")).toBeVisible();

    await page.reload();
    // App uses client-side routing; reload returns to board list
    await openBoard(page, boardName);
    await expect(page.getByText("Persistent Col")).toBeVisible({ timeout: 5000 });
  });
});

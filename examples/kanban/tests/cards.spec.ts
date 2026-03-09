import { test, expect } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  uniqueName,
} from "./helpers";

test.describe("Cards", () => {
  test.beforeEach(async ({ page }) => {
    const boardName = uniqueName("CardTest");
    await registerUser(page);
    await createBoard(page, boardName);
    await openBoard(page, boardName);
    await addColumn(page, "To Do");
  });

  test("can add cards to a column", async ({ page }) => {
    await addCard(page, "To Do", "Card A");
    await expect(page.getByText("Card A")).toBeVisible();

    await addCard(page, "To Do", "Card B");
    await addCard(page, "To Do", "Card C");

    await expect(page.getByText("Card A")).toBeVisible();
    await expect(page.getByText("Card B")).toBeVisible();
    await expect(page.getByText("Card C")).toBeVisible();
  });

  test("updates column card count after adding cards", async ({ page }) => {
    const column = page.getByTestId("column-To Do");
    const cardCount = column.getByTestId("card-count");

    await addCard(page, "To Do", "Card 1");
    await expect(cardCount).toHaveText("1");

    await addCard(page, "To Do", "Card 2");
    await expect(cardCount).toHaveText("2");
  });

  test("can open card edit modal by clicking a card", async ({ page }) => {
    await addCard(page, "To Do", "Editable Card");
    await page.getByText("Editable Card").click();

    // Modal should open (role="dialog" on the modal content)
    const modal = page.getByRole("dialog");
    await expect(page.getByText("Edit Card")).toBeVisible();
    await expect(modal.getByLabel("Title")).toHaveValue("Editable Card");
  });

  test("can edit card title and description", async ({ page }) => {
    await addCard(page, "To Do", "Original Title");
    await page.getByText("Original Title").click();

    // Edit title (scoped to modal via role="dialog")
    const modal = page.getByRole("dialog");
    const titleInput = modal.getByLabel("Title");
    await titleInput.clear();
    await titleInput.fill("Updated Title");

    // Add description
    await modal.getByLabel("Description").fill("My description");

    // Save
    await page.getByRole("button", { name: "Save" }).click();

    // Modal should close and card should show updated title
    await expect(page.getByText("Edit Card")).toBeHidden();
    await expect(page.getByText("Updated Title")).toBeVisible();
    await expect(page.getByText("My description")).toBeVisible();
  });

  test("can close card modal with Cancel or Escape", async ({ page }) => {
    await addCard(page, "To Do", "Close Test Modal");

    // Close with Cancel button.
    await page.getByText("Close Test Modal").click();
    await expect(page.getByText("Edit Card")).toBeVisible();
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(page.getByText("Edit Card")).toBeHidden();

    // Close with Escape key.
    await page.getByText("Close Test Modal").click();
    await expect(page.getByText("Edit Card")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByText("Edit Card")).toBeHidden();
  });

  test("can delete a card from the modal", async ({ page }) => {
    await addCard(page, "To Do", "Delete Me Card");
    await page.getByText("Delete Me Card").click();

    page.on("dialog", (dialog) => dialog.accept());
    await page.getByText("Delete card").click();

    // Modal should close and card should be gone
    await expect(page.getByText("Edit Card")).toBeHidden();
    await expect(page.getByText("Delete Me Card")).toBeHidden();
  });

  test("can cancel adding a card", async ({ page }) => {
    const column = page.getByTestId("column-To Do");
    await column.getByText("+ Add a card").click();
    await expect(column.getByPlaceholder("Card title...")).toBeVisible();

    await column.getByText("Cancel").click();
    await expect(column.getByPlaceholder("Card title...")).toBeHidden();
  });

  test("card description is visible on the board", async ({ page }) => {
    await addCard(page, "To Do", "Desc Card");
    await page.getByText("Desc Card").click();

    // Add description and save
    const modal = page.getByRole("dialog");
    await modal.getByLabel("Description").fill("Some details here");
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("Edit Card")).toBeHidden();

    // Description should show on the card in the board
    await expect(page.getByText("Some details here")).toBeVisible();
  });

  test("cancel card delete keeps the card", async ({ page }) => {
    await addCard(page, "To Do", "Keep This Card");
    await page.getByText("Keep This Card").click();
    await expect(page.getByText("Edit Card")).toBeVisible();

    // Dismiss the confirm dialog
    page.on("dialog", (dialog) => dialog.dismiss());
    await page.getByText("Delete card").click();

    // Modal should still be open and card still exists
    await expect(page.getByText("Edit Card")).toBeVisible();
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(page.getByText("Keep This Card")).toBeVisible();
  });

  test("save button is disabled when title is empty in modal", async ({ page }) => {
    await addCard(page, "To Do", "Empty Title Test");
    await page.getByText("Empty Title Test").click();
    await expect(page.getByText("Edit Card")).toBeVisible();

    // Clear the title (scoped to modal via label)
    const modal = page.getByRole("dialog");
    const titleInput = modal.getByLabel("Title");
    await titleInput.clear();

    // Save button should be disabled
    await expect(page.getByRole("button", { name: "Save" })).toBeDisabled();

    // Type something — Save should be enabled
    await titleInput.fill("Not Empty");
    await expect(page.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  test("card edits persist after modal reopen", async ({ page }) => {
    await addCard(page, "To Do", "Persist Edit");
    await page.getByText("Persist Edit").click();

    // Edit title and description (scoped to modal)
    const modal = page.getByRole("dialog");
    const titleInput = modal.getByLabel("Title");
    await titleInput.clear();
    await titleInput.fill("Edited Title");
    await modal.getByLabel("Description").fill("Edited desc");
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("Edit Card")).toBeHidden();

    // Reopen the card
    await page.getByText("Edited Title").click();
    const reopenedModal = page.getByRole("dialog");
    await expect(reopenedModal.getByLabel("Title")).toHaveValue("Edited Title");
    await expect(reopenedModal.getByLabel("Description")).toHaveValue("Edited desc");
  });

  test("card column count decrements after card deletion", async ({ page }) => {
    await addCard(page, "To Do", "Count Card 1");
    await addCard(page, "To Do", "Count Card 2");

    const cardCount = page.getByTestId("column-To Do").getByTestId("card-count");
    await expect(cardCount).toHaveText("2");

    // Delete one card
    await page.getByText("Count Card 1").click();
    page.on("dialog", (dialog) => dialog.accept());
    await page.getByText("Delete card").click();
    await expect(page.getByText("Edit Card")).toBeHidden();

    await expect(cardCount).toHaveText("1");
  });

  test("add card button shows input form on click", async ({ page }) => {
    const column = page.getByTestId("column-To Do");

    // Initially shows the "+ Add a card" button, not the input
    await expect(column.getByText("+ Add a card")).toBeVisible();
    await expect(column.getByPlaceholder("Card title...")).toBeHidden();

    // Click to open form
    await column.getByText("+ Add a card").click();
    await expect(column.getByPlaceholder("Card title...")).toBeVisible();
    await expect(column.getByRole("button", { name: "Add" })).toBeVisible();
    await expect(column.getByText("Cancel")).toBeVisible();
  });
});

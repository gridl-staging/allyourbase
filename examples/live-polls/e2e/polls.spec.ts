import { test, expect } from "@playwright/test";
import {
  registerUser,
  loginWithDemoAccount,
  createPoll,
  openCreatePoll,
  pollCard,
  runId,
} from "./helpers";

test.describe("Polls", () => {
  test.beforeEach(async ({ page }) => {
    await registerUser(page);
  });

  // Shared database: other tests' polls are visible (RLS allows public read).
  // eslint-disable-next-line playwright/no-skipped-test -- cannot test empty state with shared DB
  test.skip("shows empty state when no polls exist", async ({ page }) => {
    await expect(page.getByText("No polls yet")).toBeVisible();
    await expect(page.getByText("Create the first one!")).toBeVisible();
  });

  test("can open and close create poll form", async ({ page }) => {
    // Open.
    await page.getByRole("button", { name: "+ New Poll" }).click();
    await expect(
      page.getByRole("heading", { name: "New Poll" }),
    ).toBeVisible();

    // Button changes to "Cancel".
    await expect(page.getByRole("button", { name: "Cancel" })).toBeVisible();

    // Close.
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(
      page.getByRole("heading", { name: "New Poll" }),
    ).toBeHidden();

    // Button reverts to "+ New Poll".
    await expect(
      page.getByRole("button", { name: "+ New Poll" }),
    ).toBeVisible();
  });

  test("create poll form has correct default state", async ({ page }) => {
    await openCreatePoll(page);
    const form = page.getByRole("form", { name: "New Poll" });

    await expect(form.getByPlaceholder("Ask a question...")).toBeVisible();
    await expect(form.getByPlaceholder("Option 1")).toBeVisible();
    await expect(form.getByPlaceholder("Option 2")).toBeVisible();
    // Only 2 options by default — no remove buttons.
    await expect(form.getByRole("button", { name: "x" })).toBeHidden();
    await expect(
      form.getByRole("button", { name: "+ Add option" }),
    ).toBeVisible();
    await expect(
      form.getByRole("button", { name: "Create Poll" }),
    ).toBeVisible();
  });

  test("can add and remove options", async ({ page }) => {
    await openCreatePoll(page);
    const form = page.getByRole("form", { name: "New Poll" });

    // Add a third option.
    await form.getByRole("button", { name: "+ Add option" }).click();
    await expect(form.getByPlaceholder("Option 3")).toBeVisible();

    // Remove buttons should now be visible (> 2 options).
    const removeButtons = form.getByRole("button", { name: "x" });
    await expect(removeButtons.first()).toBeVisible();

    // Remove the third option.
    await removeButtons.last().click();
    // After removal the option inputs re-index, so check the count instead.
    await expect(form.getByPlaceholder(/^Option \d+$/)).toHaveCount(2);
  });

  test("requires at least 2 options to create poll", async ({ page }) => {
    await openCreatePoll(page);

    await page.getByPlaceholder("Ask a question...").fill("Test question?");
    // Only fill 1 option (leave second empty).
    await page.getByPlaceholder("Option 1").fill("Only option");
    await page.getByRole("button", { name: "Create Poll" }).click();

    // Should show error.
    await expect(page.getByText("At least 2 options required")).toBeVisible();
  });

  test("can create a poll", async ({ page }) => {
    const card = await createPoll(page, `What is your favorite color ${runId}?`, ["Red", "Blue"]);

    // Poll should appear in the list.
    await expect(card.getByText(`What is your favorite color ${runId}?`)).toBeVisible();
    await expect(card.getByText("Red")).toBeVisible();
    await expect(card.getByText("Blue")).toBeVisible();

    // Create form should close.
    await expect(
      page.getByRole("heading", { name: "New Poll" }),
    ).toBeHidden();

    // Empty state should be gone.
    await expect(page.getByText("No polls yet")).toBeHidden();
  });

  test("can create a poll with multiple options", async ({ page }) => {
    const card = await createPoll(page, `Best language ${runId}?`, [
      "TypeScript",
      "Python",
      "Go",
      "Rust",
    ]);

    await expect(card.getByText(`Best language ${runId}?`)).toBeVisible();
    await expect(card.getByText("TypeScript")).toBeVisible();
    await expect(card.getByText("Python")).toBeVisible();
    await expect(card.getByText("Go")).toBeVisible();
    await expect(card.getByText("Rust")).toBeVisible();
  });

  test("can create multiple polls", async ({ page }) => {
    await createPoll(page, `First poll ${runId}?`, ["A", "B"]);
    await createPoll(page, `Second poll ${runId}?`, ["C", "D"]);

    await expect(page.getByText(`First poll ${runId}?`)).toBeVisible();
    await expect(page.getByText(`Second poll ${runId}?`)).toBeVisible();
  });

  test("new poll shows 0 votes initially", async ({ page }) => {
    const card = await createPoll(page, `Zero votes test ${runId}?`, ["Yes", "No"]);

    // Check vote counts show 0.
    await expect(card.getByText("0 total votes")).toBeVisible();
    await expect(card.getByText("0 votes (0%)").first()).toBeVisible();
  });

  test("polls persist after page reload", async ({ page }) => {
    await createPoll(page, `Persistent poll ${runId}?`, ["Yes", "No"]);
    await expect(page.getByText(`Persistent poll ${runId}?`)).toBeVisible();

    await page.reload();
    await expect(page.getByText(`Persistent poll ${runId}?`)).toBeVisible({
      timeout: 5000,
    });
  });
});

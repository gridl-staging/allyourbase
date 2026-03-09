import { test, expect } from "@playwright/test";
import {
  registerUser,
  createPoll,
  pollCard,
  DEMO_ACCOUNTS,
  loginWithDemoAccount,
  runId,
} from "./helpers";

test.describe("Poll Management", () => {
  test("owner can close their poll", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Close me ${runId}?`, ["Yes", "No"]);

    // Close poll.
    await card.getByText("Close poll").click();

    // Should show "Closed" badge.
    await expect(card.getByText("Closed", { exact: true })).toBeVisible();

    // "Close poll" button should be gone.
    await expect(card.getByText("Close poll")).toBeHidden();
  });

  test("closed poll disables voting", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `No voting ${runId}?`, ["Option 1", "Option 2"]);

    // Close it.
    await card.getByText("Close poll").click();
    await expect(card.getByText("Closed", { exact: true })).toBeVisible();

    // Both option buttons should be disabled.
    const buttons = card.getByRole("button", { name: /Option/ });
    const count = await buttons.count();
    for (let i = 0; i < count; i++) {
      await expect(buttons.nth(i)).toBeDisabled();
    }
  });

  test("non-owner does not see close button", async ({ browser }) => {
    // User A creates poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `Not yours ${runId}?`, ["A", "B"]);
    await expect(cardA.getByText("Close poll")).toBeVisible();

    // User B sees poll but not close button.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, `Not yours ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 5000 });
    await expect(cardB.getByText("Close poll")).toBeHidden();

    await ctxA.close();
    await ctxB.close();
  });

  test("closed poll persists across reload", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Persist close ${runId}?`, ["X", "Y"]);

    // Close.
    await card.getByText("Close poll").click();
    await expect(card.getByText("Closed", { exact: true })).toBeVisible();

    // Reload.
    await page.reload();
    const cardAfterReload = pollCard(page, `Persist close ${runId}?`);
    await expect(cardAfterReload).toBeVisible({ timeout: 5000 });
    await expect(cardAfterReload.getByText("Closed", { exact: true })).toBeVisible();
  });

  test("closed poll seen by other users shows Closed badge", async ({
    browser,
  }) => {
    // User A creates and closes a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `Closed for all ${runId}?`, ["Yes", "No"]);
    await cardA.getByText("Close poll").click();
    await expect(cardA.getByText("Closed", { exact: true })).toBeVisible();

    // User B should see the "Closed" badge.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, `Closed for all ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 5000 });
    await expect(cardB.getByText("Closed", { exact: true })).toBeVisible();

    await ctxA.close();
    await ctxB.close();
  });
});

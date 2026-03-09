import { test, expect } from "@playwright/test";
import { registerUser, createPoll, pollCard, DEMO_ACCOUNTS, loginWithDemoAccount, runId } from "./helpers";

test.describe("Voting", () => {
  test("can vote on a poll", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Pick one ${runId}?`, ["Alpha", "Beta"]);

    // Click to vote for Alpha.
    await card.getByRole("button", { name: /Alpha/ }).click();

    // Should show checkmark and updated count.
    await expect(card.getByText("1 total vote")).toBeVisible({ timeout: 5000 });
    // Alpha should show 1 vote.
    await expect(card.getByText("1 vote (100%)")).toBeVisible();
  });

  test("can change vote to another option", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Change vote ${runId}?`, ["First", "Second"]);

    // Vote for First.
    await card.getByRole("button", { name: /First/ }).click();
    await expect(card.getByText("1 total vote")).toBeVisible({ timeout: 5000 });
    // First should show 1 vote (100%), Second should show 0.
    await expect(card.getByText("1 vote (100%)")).toBeVisible();

    // Change vote to Second.
    await card.getByRole("button", { name: /Second/ }).click();
    // Should still be 1 total vote (same user changed their vote).
    await expect(card.getByText("1 total vote")).toBeVisible({ timeout: 5000 });
    // Second should now have the vote; First should be back to 0.
    // The button containing "Second" should show "1 vote (100%)".
    await expect(card.getByRole("button", { name: /Second/ }).getByText("1 vote (100%)")).toBeVisible({ timeout: 5000 });
    await expect(card.getByRole("button", { name: /First/ }).getByText("0 votes (0%)")).toBeVisible();
  });

  test("vote counts update with percentages", async ({ browser }) => {
    // User A creates a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `Two voters ${runId}?`, ["Option X", "Option Y"]);

    // User A votes for X.
    await cardA.getByRole("button", { name: /Option X/ }).click();
    await expect(cardA.getByText("1 total vote")).toBeVisible({ timeout: 5000 });

    // User B logs in and votes for Y.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);

    // Wait for the poll to be visible.
    const cardB = pollCard(pageB, `Two voters ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 5000 });

    // Vote for Y.
    await cardB.getByRole("button", { name: /Option Y/ }).click();
    await expect(cardB.getByText("2 total votes")).toBeVisible({
      timeout: 5000,
    });

    // Each option should show 50%.
    await expect(cardB.getByText("1 vote (50%)").first()).toBeVisible();

    await ctxA.close();
    await ctxB.close();
  });

  test("cannot vote on a closed poll", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Will close ${runId}?`, ["A", "B"]);

    // Close the poll.
    await card.getByText("Close poll").click();
    await expect(card.getByText("Closed", { exact: true })).toBeVisible();

    // Vote buttons should be disabled.
    const optionA = card.getByRole("button", { name: /\bA\b/ });
    await expect(optionA).toBeDisabled();
  });
});

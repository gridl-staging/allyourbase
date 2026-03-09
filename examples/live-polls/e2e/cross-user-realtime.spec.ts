import { test, expect } from "@playwright/test";
import { DEMO_ACCOUNTS, loginWithDemoAccount, createPoll, pollCard, runId } from "./helpers";

test.describe("Cross-user realtime voting", () => {
  // Given User A creates a poll and User B is viewing it
  // When User A votes on an option
  // Then User B sees the vote count update in realtime (no refresh)
  test("vote by user A updates count for user B in realtime", async ({ browser }) => {
    // Arrange: User A logs in and creates a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `RT vote ${runId}?`, ["Alpha", "Beta"]);

    // Arrange: User B logs in and sees the poll via SSE.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, `RT vote ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 10000 });
    await expect(cardB.getByText("0 total votes")).toBeVisible();

    // Act: User A votes for "Alpha".
    await cardA.getByRole("button", { name: /Alpha/ }).click();
    await expect(cardA.getByText("1 total vote")).toBeVisible({ timeout: 5000 });

    // Assert: User B sees the vote count update WITHOUT any page refresh.
    await expect(cardB.getByText("1 total vote")).toBeVisible({ timeout: 10000 });

    await ctxA.close();
    await ctxB.close();
  });

  // Given User A has already voted on a poll
  // When User A changes their vote to a different option
  // Then User B sees the updated counts in realtime (no refresh)
  test("vote change by user A reflects for user B in realtime", async ({ browser }) => {
    // Arrange: User A creates a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `RT change ${runId}?`, ["Left", "Right"]);

    // Arrange: User B logs in and sees the poll.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, `RT change ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 10000 });

    // Act: User A votes "Left".
    await cardA.getByRole("button", { name: /Left/ }).click();
    await expect(cardA.getByText("1 total vote")).toBeVisible({ timeout: 5000 });

    // Assert: User B sees the vote on "Left" in realtime.
    await expect(cardB.getByText("1 total vote")).toBeVisible({ timeout: 10000 });

    // Act: User A changes vote to "Right".
    await cardA.getByRole("button", { name: /Right/ }).click();
    // User A should see Left drop to 0 and Right go to 1.
    await expect(
      cardA.getByRole("button", { name: /Right/ }).getByText(/1 vote/)
    ).toBeVisible({ timeout: 5000 });

    // Assert: User B sees the change in realtime — Right has 1 vote.
    await expect(
      cardB.getByRole("button", { name: /Right/ }).getByText(/1 vote/)
    ).toBeVisible({ timeout: 10000 });
    // Left should have 0 votes now.
    await expect(
      cardB.getByRole("button", { name: /Left/ }).getByText(/0 votes/)
    ).toBeVisible({ timeout: 10000 });

    await ctxA.close();
    await ctxB.close();
  });
});

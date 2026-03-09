import { test, expect } from "@playwright/test";
import { DEMO_ACCOUNTS, loginWithDemoAccount, createPoll, pollCard, runId } from "./helpers";

test.describe("Realtime SSE", () => {
  test("poll created by user A appears for user B", async ({ browser }) => {
    // User A logs in.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);

    // User B logs in.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);

    // User A creates a poll.
    await createPoll(pageA, `Realtime poll ${runId}?`, ["Alpha", "Beta"]);

    // User B should see it appear via SSE.
    await expect(pollCard(pageB, `Realtime poll ${runId}?`)).toBeVisible({
      timeout: 10000,
    });

    await ctxA.close();
    await ctxB.close();
  });

  // Vote propagation via SSE is covered thoroughly in cross-user-realtime.spec.ts
  // (first vote + vote change, both with per-option count assertions).

  test("poll closed by owner updates for other users", async ({ browser }) => {
    // User A creates a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `Close sync ${runId}?`, ["A", "B"]);

    // User B sees the poll.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, `Close sync ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 5000 });
    await expect(cardB.getByText("Closed", { exact: true })).toBeHidden();

    // User A closes the poll.
    await cardA.getByText("Close poll").click();
    await expect(cardA.getByText("Closed", { exact: true })).toBeVisible();

    // User B should see "Closed" appear via SSE.
    await expect(cardB.getByText("Closed", { exact: true })).toBeVisible({ timeout: 10000 });

    await ctxA.close();
    await ctxB.close();
  });
});

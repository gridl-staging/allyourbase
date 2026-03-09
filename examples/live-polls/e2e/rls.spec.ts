import { test, expect } from "@playwright/test";
import {
  DEMO_ACCOUNTS,
  loginWithDemoAccount,
  createPoll,
  pollCard,
  runId,
  attemptDirectVoteOnClosedPoll,
  attemptDirectClosePoll,
  attemptDirectInsertPollForOtherUser,
} from "./helpers";

test.describe("Row-Level Security", () => {
  test("all users can see all polls (public read)", async ({ browser }) => {
    // User A creates a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    await createPoll(pageA, `Public poll ${runId}?`, ["Open", "Closed"]);

    // User B should see it.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    await expect(pollCard(pageB, `Public poll ${runId}?`)).toBeVisible({
      timeout: 5000,
    });

    // User C should also see it.
    const ctxC = await browser.newContext();
    const pageC = await ctxC.newPage();
    await loginWithDemoAccount(pageC, DEMO_ACCOUNTS[2].email);
    await expect(pollCard(pageC, `Public poll ${runId}?`)).toBeVisible({
      timeout: 5000,
    });

    await ctxA.close();
    await ctxB.close();
    await ctxC.close();
  });

  test("all users can vote on any poll", async ({ browser }) => {
    // User A creates a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `Everyone votes ${runId}?`, ["Yes", "No"]);

    // User A votes.
    await cardA.getByRole("button", { name: /Yes/ }).click();
    await expect(cardA.getByText("1 total vote")).toBeVisible({
      timeout: 5000,
    });

    // User B votes.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, `Everyone votes ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 5000 });
    await cardB.getByRole("button", { name: /No/ }).click();
    await expect(cardB.getByText("2 total votes")).toBeVisible({
      timeout: 5000,
    });

    // User C votes.
    const ctxC = await browser.newContext();
    const pageC = await ctxC.newPage();
    await loginWithDemoAccount(pageC, DEMO_ACCOUNTS[2].email);
    const cardC = pollCard(pageC, `Everyone votes ${runId}?`);
    await expect(cardC).toBeVisible({ timeout: 5000 });
    await cardC.getByRole("button", { name: /Yes/ }).click();
    await expect(cardC.getByText("3 total votes")).toBeVisible({
      timeout: 5000,
    });

    await ctxA.close();
    await ctxB.close();
    await ctxC.close();
  });

  test("server rejects direct API close by non-owner (bypasses UI)", async ({
    browser,
  }) => {
    // User A creates a poll. The question embeds runId so the helper's
    // poll lookup finds this exact poll rather than an unrelated one.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const pollQuestion = `Owner RLS close ${runId}?`;
    await createPoll(pageA, pollQuestion, ["Yes", "No"]);
    await ctxA.close();

    // User B logs in and tries to close User A's poll directly via API,
    // bypassing the UI (which would never show the "Close poll" button to
    // non-owners). The server-side polls_update RLS policy must reject this.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, pollQuestion);
    await expect(cardB).toBeVisible({ timeout: 5000 });

    // Verify the poll is still open before the bypass attempt.
    await expect(cardB.getByText("Closed", { exact: true })).toBeHidden();

    // Returns 0 on setup failure so assertion fails loudly on a false pass.
    const status = await attemptDirectClosePoll(pageB, pollQuestion);

    // The polls_update RLS policy (USING user_id = ayb.user_id) must reject
    // this write with a 4xx error — User B does not own this poll.
    expect(status).toBeGreaterThanOrEqual(400);

    // The poll must still show as open in the UI — not closed by the rejected call.
    await expect(cardB.getByText("Closed", { exact: true })).toBeHidden();

    await ctxB.close();
  });

  test("server rejects voting on a closed poll via direct API call (bypasses UI)", async ({
    browser,
  }) => {
    // User A creates and closes a poll.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    // The question embeds runId so we can identify this exact poll later —
    // prevents the evaluate callback from accidentally finding an unrelated
    // closed poll from another test run and producing a false pass.
    const pollQuestion = `API closed ${runId}?`;
    const cardA = await createPoll(pageA, pollQuestion, ["Yes", "No"]);
    await cardA.getByText("Close poll").click();
    await expect(cardA.getByText("Closed", { exact: true })).toBeVisible();
    await ctxA.close();

    // User B logs in and sees the closed poll.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, pollQuestion);
    await expect(cardB).toBeVisible({ timeout: 5000 });
    await expect(cardB.getByText("Closed", { exact: true })).toBeVisible();

    // Attempt to cast a vote via direct API call, bypassing the disabled-button
    // UI guard. The helper lives in helpers.ts (shortcuts are allowed there).
    // Returns 0 on any setup failure so the assertion fails loudly, not silently.
    const voteStatus = await attemptDirectVoteOnClosedPoll(pageB, pollQuestion);

    // The server-side votes_insert RLS policy (WITH CHECK on is_closed) must
    // reject the request with a 4xx error.
    expect(voteStatus).toBeGreaterThanOrEqual(400);

    await ctxB.close();
  });

  test("votes are visible to all users (HTTP read, not realtime)", async ({ browser }) => {
    // User A creates and votes.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const cardA = await createPoll(pageA, `Vote visibility ${runId}?`, ["Red", "Blue"]);
    await cardA.getByRole("button", { name: /Red/ }).click();
    await expect(cardA.getByText("1 total vote")).toBeVisible({
      timeout: 5000,
    });

    // User B should see the vote count.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    const cardB = pollCard(pageB, `Vote visibility ${runId}?`);
    await expect(cardB).toBeVisible({ timeout: 5000 });
    await expect(cardB.getByText("1 total vote")).toBeVisible({
      timeout: 5000,
    });

    await ctxA.close();
    await ctxB.close();
  });

  test("server rejects direct poll insert on behalf of another user (bypasses UI)", async ({
    browser,
  }) => {
    // User A creates a poll — this establishes User A's user_id in the DB so
    // that User B can look it up and attempt to impersonate User A on an insert.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    const pollQuestion = `Insert RLS ${runId}?`;
    await createPoll(pageA, pollQuestion, ["Yes", "No"]);
    await ctxA.close();

    // User B logs in and attempts to POST /api/collections/polls with
    // user_id set to User A's UUID — bypassing the UI which always sends
    // the current user's own ID from the JWT. The server-side polls_insert
    // WITH CHECK policy must reject this with a 4xx error.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    await expect(pollCard(pageB, pollQuestion)).toBeVisible({ timeout: 5000 });

    // Returns 0 on setup failure so assertion fails loudly, not silently.
    const status = await attemptDirectInsertPollForOtherUser(pageB, pollQuestion);

    // The polls_insert RLS policy (WITH CHECK user_id = ayb.user_id) must
    // reject this spoofed insert — User B cannot create polls owned by User A.
    expect(status).toBeGreaterThanOrEqual(400);

    await ctxB.close();
  });
});

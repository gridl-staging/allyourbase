import { type Page, type Locator, expect } from "@playwright/test";

let userCounter = 0;
export const runId = Math.random().toString(36).slice(2, 8);

/** Generate a unique test user email to avoid collisions between test runs. */
export function uniqueEmail(): string {
  return `test-${runId}-${Date.now()}-${++userCounter}@example.com`;
}

export const TEST_PASSWORD = "testpassword123";

/** Demo account credentials. */
export const DEMO_ACCOUNTS = [
  { email: "alice@demo.test", password: "password123" },
  { email: "bob@demo.test", password: "password123" },
  { email: "charlie@demo.test", password: "password123" },
];

/** Register a new user via the UI and return the email. */
export async function registerUser(page: Page): Promise<string> {
  const email = uniqueEmail();
  await page.goto("/");

  // Switch to register mode.
  await page.getByRole("button", { name: "Register" }).click();

  // Fill form.
  await page.getByPlaceholder("Email").fill(email);
  await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Create Account" }).click();

  // Wait for login to complete ("Sign out" only appears in the authenticated UI).
  await expect(page.getByText("Sign out")).toBeVisible({ timeout: 15000 });

  return email;
}

/** Login with a demo account by clicking it in the demo accounts list. */
export async function loginWithDemoAccount(
  page: Page,
  email: string = DEMO_ACCOUNTS[0].email,
): Promise<void> {
  await page.goto("/");

  // Click the demo account button to fill credentials.
  const acct = DEMO_ACCOUNTS.find((a) => a.email === email)!;
  await page.getByText(acct.email).click();

  // Submit the form.
  await page.getByRole("button", { name: "Sign In" }).click();

  // Wait for login to complete ("Sign out" only appears in the authenticated UI,
  // not on the auth form which also shows the "Live Polls" heading).
  await expect(page.getByText("Sign out")).toBeVisible({ timeout: 10000 });
}

/** Login with existing credentials. */
export async function loginUser(
  page: Page,
  email: string,
  password: string = TEST_PASSWORD,
): Promise<void> {
  await page.goto("/");
  await page.getByPlaceholder("Email").fill(email);
  await page.getByPlaceholder("Password").fill(password);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByText("Sign out")).toBeVisible({ timeout: 10000 });
}

/** Open the create poll form. */
export async function openCreatePoll(page: Page): Promise<void> {
  await page.getByRole("button", { name: "+ New Poll" }).click();
  await expect(
    page.getByRole("heading", { name: "New Poll" }),
  ).toBeVisible();
}

/**
 * Attempt to INSERT a vote on the named closed poll via a direct fetch call,
 * bypassing the UI's disabled-button guard. Returns the HTTP response status.
 *
 * This helper exists to test server-side RLS enforcement — specifically the
 * votes_insert policy that rejects writes to closed polls. The test cannot
 * be written with UI interactions alone because the UI prevents the action
 * before it reaches the server. Per BROWSER_TESTING_STANDARDS_2.md, API
 * shortcuts belong here in helpers.ts, never in spec files.
 *
 * Returns 0 on any setup failure (token missing, poll not found, etc.) so
 * that `expect(result).toBeGreaterThanOrEqual(400)` fails loudly instead of
 * silently producing a false pass.
 */
export async function attemptDirectVoteOnClosedPoll(
  page: Page,
  pollQuestion: string,
): Promise<number> {
  return page.evaluate(async (question: string) => {
    const token = localStorage.getItem("ayb_token");
    if (!token) return 0;

    const meRes = await fetch("/api/auth/me", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const me = (await meRes.json()) as { id: string };

    const pollsRes = await fetch(
      "/api/collections/polls?perPage=500&sort=-created_at",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    const polls = ((await pollsRes.json()).items ?? []) as Array<{
      id: string;
      is_closed: boolean;
      question: string;
    }>;
    const closed = polls.find((p) => p.question === question);
    if (!closed || !closed.is_closed) return 0;

    // Paginate through ALL poll_options — a shared CI DB accumulates options
    // across test runs and a single 500-row page misses recent entries.
    const PAGE_SIZE = 500;
    const allOpts: Array<{ id: string; poll_id: string }> = [];
    for (let pg = 1; ; pg++) {
      const optsRes = await fetch(
        `/api/collections/poll_options?perPage=${PAGE_SIZE}&page=${pg}&sort=position`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      const body = (await optsRes.json()) as {
        items?: Array<{ id: string; poll_id: string }>;
      };
      const items = body.items ?? [];
      allOpts.push(...items);
      if (items.length < PAGE_SIZE) break;
    }
    const opt = allOpts.find((o) => o.poll_id === closed.id);
    if (!opt) return 0;

    const voteRes = await fetch("/api/collections/votes", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        poll_id: closed.id,
        option_id: opt.id,
        user_id: me.id,
      }),
    });
    return voteRes.status;
  }, pollQuestion);
}

/**
 * Attempt to PATCH (close) a poll owned by another user via a direct fetch call,
 * bypassing the UI's hidden-button guard. Returns the HTTP response status.
 *
 * This helper exists to test server-side RLS enforcement — specifically the
 * polls_update policy that only allows the poll owner to update their poll.
 * The test cannot be written with UI interactions alone because the UI never
 * renders the "Close poll" button for non-owners. Per BROWSER_TESTING_STANDARDS_2.md,
 * API shortcuts belong here in helpers.ts, never in spec files.
 *
 * Returns 0 on any setup failure (token missing, poll not found, etc.) so that
 * `expect(result).toBeGreaterThanOrEqual(400)` fails loudly instead of silently
 * producing a false pass.
 */
export async function attemptDirectClosePoll(
  page: Page,
  pollQuestion: string,
): Promise<number> {
  return page.evaluate(async (question: string) => {
    const token = localStorage.getItem("ayb_token");
    if (!token) return 0;

    const pollsRes = await fetch(
      "/api/collections/polls?perPage=500&sort=-created_at",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    const polls = ((await pollsRes.json()).items ?? []) as Array<{
      id: string;
      is_closed: boolean;
      question: string;
    }>;
    const target = polls.find((p) => p.question === question);
    if (!target || target.is_closed) return 0;

    // Attempt to close the poll (set is_closed=true) as a non-owner.
    const patchRes = await fetch(`/api/collections/polls/${target.id}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ is_closed: true }),
    });
    return patchRes.status;
  }, pollQuestion);
}

/**
 * Attempt to INSERT a poll on behalf of another user by spoofing their user_id
 * in the request body, bypassing the UI which always sets user_id from the JWT.
 * Returns the HTTP response status.
 *
 * This helper exists to test server-side RLS enforcement — specifically the
 * polls_insert WITH CHECK policy that rejects inserts where user_id ≠ the
 * authenticated user's ID. The test cannot be written with UI interactions alone
 * because the frontend always sends the correct user_id from the JWT. Per
 * BROWSER_TESTING_STANDARDS_2.md, API shortcuts belong here in helpers.ts, never
 * in spec files.
 *
 * Returns 0 on any setup failure (token missing, owner poll not found) so that
 * `expect(result).toBeGreaterThanOrEqual(400)` fails loudly instead of silently
 * producing a false pass.
 */
export async function attemptDirectInsertPollForOtherUser(
  page: Page,
  existingOwnerPollQuestion: string,
): Promise<number> {
  return page.evaluate(async (question: string) => {
    const token = localStorage.getItem("ayb_token");
    if (!token) return 0;

    // Find the target poll (owned by another user) to extract their user_id.
    const pollsRes = await fetch(
      "/api/collections/polls?perPage=500&sort=-created_at",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    const polls = ((await pollsRes.json()).items ?? []) as Array<{
      id: string;
      user_id: string;
      question: string;
    }>;
    const ownerPoll = polls.find((p) => p.question === question);
    if (!ownerPoll) return 0;

    // Attempt to INSERT a new poll using the other user's user_id, bypassing
    // the frontend which always supplies the current user's own ID.
    const insertRes = await fetch("/api/collections/polls", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        question: `Spoofed insert ${Date.now()}?`,
        user_id: ownerPoll.user_id,
      }),
    });
    return insertRes.status;
  }, existingOwnerPollQuestion);
}

/** Return a Locator for the poll card containing the given question. */
export function pollCard(page: Page, question: string): Locator {
  return page
    .getByTestId("poll-card")
    .filter({ has: page.getByRole("heading", { name: question }) });
}

/** Create a poll with the given question and options. Returns a Locator for the poll card. */
export async function createPoll(
  page: Page,
  question: string,
  options: string[],
): Promise<Locator> {
  await openCreatePoll(page);

  // Fill question.
  await page.getByPlaceholder("Ask a question...").fill(question);

  // Fill options (2 inputs exist by default).
  for (let i = 0; i < options.length; i++) {
    if (i >= 2) {
      // Add more option inputs as needed.
      await page.getByRole("button", { name: "+ Add option" }).click();
    }
    await page.getByPlaceholder(`Option ${i + 1}`).fill(options[i]);
  }

  // Submit.
  await page.getByRole("button", { name: "Create Poll" }).click();

  // Wait for poll to appear in the list.
  await expect(page.getByText(question)).toBeVisible({ timeout: 10000 });

  return pollCard(page, question);
}

import { test, expect, type Browser, type Page, type BrowserContext } from "@playwright/test";
import {
  DEMO_ACCOUNTS,
  loginWithDemoAccount,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  runId,
} from "./helpers";

/** Set up two different users viewing the same board.
 *  User A (alice) creates the board, User B (bob) navigates to it.
 *  Returns both contexts and pages for cleanup. */
async function setupCrossUserBoard(
  browser: Browser,
  boardName: string,
): Promise<{ ctxA: BrowserContext; ctxB: BrowserContext; pageA: Page; pageB: Page }> {
  // User A logs in and creates a board.
  const ctxA = await browser.newContext();
  const pageA = await ctxA.newPage();
  await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
  await createBoard(pageA, boardName);
  await openBoard(pageA, boardName);

  // User B logs in (different browser context = different session).
  const ctxB = await browser.newContext();
  const pageB = await ctxB.newPage();
  await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);

  // User B should see User A's board in the list.
  await expect(pageB.getByText(boardName).first()).toBeVisible({ timeout: 10000 });
  await openBoard(pageB, boardName);

  return { ctxA, ctxB, pageA, pageB };
}

test.describe("Cross-user realtime collaboration", () => {
  // Given User A creates a board
  // When User B logs in and views the board list
  // Then User B can see User A's board
  test("board created by user A is visible to user B", async ({ browser }) => {
    const boardName = `Shared Board ${runId}`;

    // Arrange: User A creates a board.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    await createBoard(pageA, boardName);

    // Act: User B logs in.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);

    // Assert: User B sees User A's board.
    await expect(pageB.getByText(boardName).first()).toBeVisible({ timeout: 10000 });

    await ctxA.close();
    await ctxB.close();
  });

  // Given both users are viewing the same board
  // When User A adds a card
  // Then User B sees the card appear via SSE (no refresh)
  test("card added by user A appears for user B in realtime", async ({ browser }) => {
    const boardName = `Card RT ${runId}`;
    const { ctxA, ctxB, pageA, pageB } = await setupCrossUserBoard(browser, boardName);

    // Arrange: User A adds a column.
    await addColumn(pageA, "Todo");
    await expect(pageB.getByText("Todo")).toBeVisible({ timeout: 10000 });

    // Act: User A adds a card.
    await addCard(pageA, "Todo", "New Task");

    // Assert: User B sees the card via SSE.
    await expect(pageB.getByText("New Task")).toBeVisible({ timeout: 10000 });

    await ctxA.close();
    await ctxB.close();
  });

  // Given both users are viewing the same board
  // When User A adds a column
  // Then User B sees the column appear via SSE (no refresh)
  test("column added by user A appears for user B in realtime", async ({ browser }) => {
    const boardName = `Col RT ${runId}`;
    const { ctxA, ctxB, pageA, pageB } = await setupCrossUserBoard(browser, boardName);

    // Act: User A adds a column.
    await addColumn(pageA, "In Progress");

    // Assert: User B sees the column via SSE.
    await expect(pageB.getByText("In Progress")).toBeVisible({ timeout: 10000 });

    await ctxA.close();
    await ctxB.close();
  });
});

import { type Page, expect } from "@playwright/test";

let userCounter = 0;
let nameCounter = 0;
export const runId = Math.random().toString(36).slice(2, 8);

/** Generate a unique board/resource name to avoid collisions across parallel workers
 *  (boards_select USING (true) makes all boards visible to all users). */
export function uniqueName(base: string): string {
  return `${base} ${runId}-${++nameCounter}`;
}

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

/** Login with a demo account by clicking it to fill credentials, then signing in. */
export async function loginWithDemoAccount(
  page: Page,
  email: string = DEMO_ACCOUNTS[0].email,
): Promise<void> {
  await page.goto("/");
  const acct = DEMO_ACCOUNTS.find((a) => a.email === email)!;
  await page.getByText(acct.email).click();
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 10000 });
}

/** Register a new user and return the email. */
export async function registerUser(page: Page): Promise<string> {
  const email = uniqueEmail();
  await page.goto("/");

  // Switch to register mode
  await page.getByRole("button", { name: "Sign up" }).click();

  // Fill form
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("At least 8 characters").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Create Account" }).click();

  // Wait for board list to load (auth succeeded).
  // First registration can be slow due to managed Postgres cold-start + bcrypt.
  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 15000 });

  return email;
}

/** Login with existing credentials. */
export async function loginUser(page: Page, email: string): Promise<void> {
  await page.goto("/");
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("At least 8 characters").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
}

/** Create a board and return the board title. */
export async function createBoard(
  page: Page,
  title: string,
): Promise<void> {
  await page.getByPlaceholder("New board name...").fill(title);
  await page.getByRole("button", { name: "Create" }).click();
  // Use .first() — collaborative model means other users' boards are visible,
  // so duplicate titles from other workers may exist.
  await expect(page.getByText(title).first()).toBeVisible();
}

/** Navigate into a board. */
export async function openBoard(page: Page, title: string): Promise<void> {
  // Use .first() — boards sorted by -created_at, so most recent is first.
  await page.getByText(title).first().click();
  await expect(
    page.getByRole("heading", { name: title }),
  ).toBeVisible({ timeout: 5000 });
}

/** Add a column to the current board. */
export async function addColumn(
  page: Page,
  title: string,
): Promise<void> {
  await page.getByPlaceholder("+ Add column...").fill(title);
  await page.getByRole("button", { name: "Add Column" }).click();
  await expect(page.getByText(title)).toBeVisible();
}

/** Add a card to a column. */
export async function addCard(
  page: Page,
  columnTitle: string,
  cardTitle: string,
): Promise<void> {
  // Scope to column via data-testid added to the column container
  const column = page.getByTestId(`column-${columnTitle}`);
  await column.getByText("+ Add a card").click();
  await column.getByPlaceholder("Card title...").fill(cardTitle);
  await column.getByRole("button", { name: "Add", exact: true }).click();
  await expect(page.getByText(cardTitle)).toBeVisible();
}

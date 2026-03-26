/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_6_test_verification_and_lint/allyourbase_dev/ui/browser-tests-unmocked/fixtures/core.ts.
 */
import { expect, type APIRequestContext, type Page } from "@playwright/test";

export function sqlLiteral(value: string): string {
  return value.replaceAll("'", "''");
}

/**
 * TODO: Document validateResponse.
 */
export async function validateResponse(
  res: Awaited<ReturnType<APIRequestContext["post"]>>,
  context: string,
): Promise<void> {
  if (!res.ok()) {
    const status = res.status();
    let errorMsg = `${context} failed with status ${status}`;
    try {
      const body = await res.json();
      if (body.message) {
        errorMsg += `: ${body.message}`;
      }
      if (body.code) {
        errorMsg += ` (code: ${body.code})`;
      }
    } catch {
      const text = await res.text();
      if (text) {
        errorMsg += `: ${text}`;
      }
    }
    throw new Error(errorMsg);
  }
}

export async function checkAuthEnabled(
  request: APIRequestContext,
): Promise<{ auth: boolean }> {
  const res = await request.get("/api/admin/status");
  await validateResponse(res, "Check admin status");
  const body = await res.json();
  return { auth: !!body.auth };
}

/**
 * TODO: Document execSQL.
 */
export async function execSQL(
  request: APIRequestContext,
  token: string,
  query: string,
): Promise<{ columns: string[]; rows: unknown[][]; rowCount: number }> {
  const statements = query
    .split(";")
    .map((statement) => statement.trim())
    .filter((statement) => statement.length > 0);

  let lastResult: { columns: string[]; rows: unknown[][]; rowCount: number } = {
    columns: [],
    rows: [],
    rowCount: 0,
  };

  for (const statement of statements) {
    const res = await request.post("/api/admin/sql", {
      headers: { Authorization: `Bearer ${token}` },
      data: { query: statement },
    });
    await validateResponse(res, `Execute SQL: ${statement.substring(0, 50)}...`);
    lastResult = await res.json();
  }

  return lastResult;
}

/**
 * TODO: Document probeEndpoint.
 */
export async function probeEndpoint(
  request: APIRequestContext,
  token: string,
  path: string,
  options: {
    method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
    data?: unknown;
  } = {},
): Promise<number> {
  const method = options.method || "GET";
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (options.data !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  const requestOptions = {
    headers,
    ...(options.data !== undefined ? { data: options.data } : {}),
  };

  switch (method) {
    case "POST": {
      const res = await request.post(path, requestOptions);
      return res.status();
    }
    case "PUT": {
      const res = await request.put(path, requestOptions);
      return res.status();
    }
    case "PATCH": {
      const res = await request.patch(path, requestOptions);
      return res.status();
    }
    case "DELETE": {
      const res = await request.delete(path, requestOptions);
      return res.status();
    }
    default: {
      const res = await request.get(path, requestOptions);
      return res.status();
    }
  }
}

/**
 * Waits for the admin dashboard to finish booting (loading/login → ready).
 * Ready state renders the Layout with the sidebar `<aside>`. Uses a generous
 * timeout to tolerate slow schema fetches under parallel test load.
 */
export async function waitForDashboard(page: Page): Promise<void> {
  const sidebar = page.locator("aside");
  const retryButton = page.getByRole("button", { name: /^retry$/i });
  const timeoutAt = Date.now() + 30000;

  while (Date.now() < timeoutAt) {
    try {
      await sidebar.waitFor({ state: "visible", timeout: 1000 });
      return;
    } catch {
      // Boot can transiently land on a "Connection Error" screen when admin
      // endpoints are rate limited. Clicking Retry keeps dashboard startup
      // resilient without requiring every spec to duplicate this recovery path.
      if (await retryButton.isVisible().catch(() => false)) {
        await retryButton.click().catch(() => {});
      }
    }
  }

  await sidebar.waitFor({ state: "visible", timeout: 1000 });
}

/**
 * TODO: Document expectRlsPolicyCard.
 */
export async function expectRlsPolicyCard(
  page: Page,
  params: {
    policyName: string;
    command: string;
    usingExpression: string;
  },
): Promise<void> {
  const { policyName, command, usingExpression } = params;
  const ariaSnapshot = await page.locator("main").ariaSnapshot();
  const lines = ariaSnapshot.split("\n");
  const policyNameLine = lines.findIndex((line) => line.includes(policyName));

  expect(policyNameLine, `Expected policy ${policyName} to appear in the RLS policy list`).toBeGreaterThanOrEqual(0);

  const deleteButtonOffset = lines
    .slice(policyNameLine)
    .findIndex((line) => line.includes('button "Delete policy"'));
  const blockEnd = deleteButtonOffset >= 0 ? policyNameLine + deleteButtonOffset + 6 : policyNameLine + 20;
  const policyBlock = lines.slice(policyNameLine, Math.min(lines.length, blockEnd)).join("\n");

  expect(policyBlock).toContain(command);
  expect(policyBlock).toContain("USING:");
  expect(policyBlock).toContain(usingExpression);
}

/**
 * TODO: Document getStoredAdminToken.
 */
export async function getStoredAdminToken(): Promise<string> {
  const fs = await import("fs/promises");
  const path = await import("path");
  const url = await import("url");

  const __dirname = path.dirname(url.fileURLToPath(import.meta.url));
  const authFile = path.join(__dirname, "../.auth/admin.json");

  try {
    const authState = JSON.parse(await fs.readFile(authFile, "utf-8"));
    const origins = authState.origins || [];
    for (const origin of origins) {
      const localStorage = origin.localStorage || [];
      for (const item of localStorage) {
        if (item.name === "ayb_admin_token") {
          return item.value;
        }
      }
    }
    throw new Error("Admin token not found in auth state file");
  } catch (err) {
    throw new Error(
      `Failed to read admin token from ${authFile}: ${err}. ` +
      `Make sure auth.setup.ts has run successfully.`,
    );
  }
}

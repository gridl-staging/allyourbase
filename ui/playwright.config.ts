/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/playwright.config.ts.
 */
import { defineConfig } from "@playwright/test";

// Environment-based configuration
const ENV = process.env.PLAYWRIGHT_ENV || "local";
const BLOCKED_PRODUCTION_ORIGIN = "https://install.allyourbase.io";
const BASE_URLS = {
  // Use the IPv4 loopback explicitly so Playwright API requests do not
  // resolve localhost to ::1 on machines where AYB is only listening on IPv4.
  local: "http://127.0.0.1:8090",
  staging: "https://staging.allyourbase.io",
};

/**
 * TODO: Document resolveBaseURL.
 */
function resolveBaseURL(): string {
  const configuredBaseURL =
    process.env.PLAYWRIGHT_BASE_URL || BASE_URLS[ENV as keyof typeof BASE_URLS];

  if (!configuredBaseURL) {
    throw new Error(
      `Unsupported PLAYWRIGHT_ENV '${ENV}'. Use one of: ${Object.keys(BASE_URLS).join(", ")} or set PLAYWRIGHT_BASE_URL explicitly.`,
    );
  }

  let configuredOrigin: string;
  try {
    configuredOrigin = new URL(configuredBaseURL).origin;
  } catch {
    throw new Error(
      `Invalid PLAYWRIGHT_BASE_URL '${configuredBaseURL}'. Provide an absolute http(s) URL.`,
    );
  }

  if (configuredOrigin === BLOCKED_PRODUCTION_ORIGIN) {
    throw new Error(
      `Unmocked admin Playwright suites are blocked from targeting production (${BLOCKED_PRODUCTION_ORIGIN}). Use a non-production base URL.`,
    );
  }

  return configuredBaseURL;
}

export default defineConfig({
  testDir: "./browser-tests-unmocked",
  globalSetup: "./browser-tests-unmocked/global-setup.ts",
  outputDir: "test-results-unmocked",
  timeout: 30_000, // Increased for network latency in staging/prod
  expect: { timeout: 10_000 },
  fullyParallel: true,
  workers: 3, // Reduce parallelism to avoid resource contention
  retries: 1, // Retry once on failure to handle timing issues
  use: {
    baseURL: resolveBaseURL(),
    headless: true, // Always run in headless mode
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  // Auth setup runs first, smoke and full depend on it
  projects: [
    {
      name: "setup",
      testMatch: /auth\.setup\.ts/,
      use: { browserName: "chromium" },
    },
    {
      name: "smoke",
      testMatch: /smoke\/.*\.spec\.ts/,
      dependencies: ["setup"],
      use: {
        browserName: "chromium",
        storageState: "browser-tests-unmocked/.auth/admin.json",
      },
    },
    {
      name: "full",
      testMatch: /full\/.*\.spec\.ts/,
      dependencies: ["setup"],
      use: {
        browserName: "chromium",
        storageState: "browser-tests-unmocked/.auth/admin.json",
      },
    },
  ],
  reporter: [
    ["html", { outputFolder: "playwright-report", open: "never" }],
    ["json", { outputFile: "playwright-report/results.json" }],
    ["list"],
  ],
});

import { defineConfig } from "@playwright/test";

// Environment-based configuration
const ENV = process.env.PLAYWRIGHT_ENV || "local";
const BASE_URLS = {
  local: "http://localhost:8090",
  staging: "https://staging.allyourbase.io",
  prod: "https://install.allyourbase.io",
};

export default defineConfig({
  testDir: "./browser-tests-unmocked",
  timeout: 30_000, // Increased for network latency in staging/prod
  expect: { timeout: 10_000 },
  fullyParallel: true,
  workers: 3, // Reduce parallelism to avoid resource contention
  retries: 1, // Retry once on failure to handle timing issues
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || BASE_URLS[ENV as keyof typeof BASE_URLS],
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

import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "*.spec.ts",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  retries: 0,
  reporter: [
    ["list"],
    ["html", { open: "never", outputFolder: "../results/playwright-report" }],
  ],
  use: {
    headless: true,
    screenshot: "on",
    trace: "on-first-retry",
    viewport: { width: 1440, height: 900 },
  },
  projects: [
    {
      name: "dashboard-qa",
      testMatch: "dashboard_explore.spec.ts",
    },
    {
      name: "demo-live-polls-qa",
      testMatch: "demo_live_polls.spec.ts",
    },
    {
      name: "demo-kanban-qa",
      testMatch: "demo_kanban.spec.ts",
    },
  ],
  outputDir: "../results/test-results",
});

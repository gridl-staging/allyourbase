import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: "http://localhost:5175",
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "npm run dev",
    port: 5175,
    reuseExistingServer: true,
    timeout: 10000,
  },
});

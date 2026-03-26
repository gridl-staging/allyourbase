import { defineConfig } from "@playwright/test";

const testPort = 4173;

export default defineConfig({
  testDir: "./tests",
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: `http://127.0.0.1:${testPort}`,
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${testPort} --strictPort`,
    port: testPort,
    reuseExistingServer: false,
    timeout: 10000,
  },
});

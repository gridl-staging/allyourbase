import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5175,
    proxy: {
      "/api": "http://localhost:8090",
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./tests/setup.ts",
    // Only pick up unit tests in tests/. The e2e/ directory contains Playwright
    // specs that must be run via `npx playwright test`, not vitest.
    include: ["tests/**/*.{test,spec}.{ts,tsx}"],
  },
});

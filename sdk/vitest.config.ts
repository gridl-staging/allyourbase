import { defineConfig } from "vitest/config";

const shouldExcludeIntegration =
  process.env.npm_lifecycle_event !== "test:integration";

export default defineConfig({
  test: {
    exclude: shouldExcludeIntegration
      ? ["**/integration*.test.ts", "**/node_modules/**"]
      : ["**/node_modules/**"],
  },
});

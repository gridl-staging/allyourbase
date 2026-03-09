import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    name: 'contract',
    globals: false,
    environment: 'node',
    setupFiles: [],
    // Contract tests hit real APIs, so they're slower
    testTimeout: 10000,
    // Fail fast - if one provider breaks, stop
    bail: 1,
  },
});

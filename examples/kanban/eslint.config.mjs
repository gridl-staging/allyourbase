// ESLint config for Playwright e2e tests.
// Enforces BROWSER_TESTING_STANDARDS_2.md — no shortcuts in spec files.
import playwright from "eslint-plugin-playwright";
import tseslint from "typescript-eslint";

export default [
  ...tseslint.configs.recommended.map((config) => ({
    ...config,
    files: ["tests/**/*.ts"],
  })),
  {
    ...playwright.configs["flat/recommended"],
    files: ["tests/**/*.spec.ts"],
    rules: {
      ...playwright.configs["flat/recommended"].rules,

      // Ban page.evaluate and friends.
      "playwright/no-eval": "error",

      // Ban raw CSS/XPath locators — use getByRole/getByText/getByLabel.
      "playwright/no-raw-locators": "error",

      // Prefer native locators.
      "playwright/prefer-native-locators": "warn",

      // Ban deprecated page.$() API.
      "playwright/no-element-handle": "error",

      // Ban { force: true } on clicks.
      "playwright/no-force-option": "error",

      // Ban page.pause() (debugging leftover).
      "playwright/no-page-pause": "error",

      // Ban waitForTimeout — use assertion timeouts instead.
      "playwright/no-wait-for-timeout": "error",

      // Ban API calls, waitForTimeout, dispatchEvent, setExtraHTTPHeaders in specs.
      "no-restricted-syntax": [
        "error",
        {
          selector: "CallExpression[callee.property.name='waitForTimeout']",
          message: "Use assertion timeout instead of waitForTimeout.",
        },
        {
          selector: "CallExpression[callee.property.name='dispatchEvent']",
          message: "Do not use dispatchEvent — simulate real user interactions.",
        },
        {
          selector: "CallExpression[callee.property.name='setExtraHTTPHeaders']",
          message: "Do not set HTTP headers — users can't do this in the UI.",
        },
      ],

      // Suppress TS rules that conflict with Playwright patterns.
      "@typescript-eslint/no-unused-vars": "off",
    },
  },
  {
    // Exempt helper files from spec-only restrictions.
    files: ["tests/helpers.ts"],
    rules: {
      "playwright/no-raw-locators": "off",
      "no-restricted-syntax": "off",
    },
  },
];

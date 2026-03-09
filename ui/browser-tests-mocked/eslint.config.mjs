import playwright from "eslint-plugin-playwright";
import tseslint from "typescript-eslint";

export default [
  {
    ...playwright.configs["flat/recommended"],
    files: ["**/*.spec.ts"],
    languageOptions: {
      parser: tseslint.parser,
    },
    rules: {
      ...playwright.configs["flat/recommended"].rules,
      "playwright/no-eval": "error",
      "playwright/no-raw-locators": ["error", {
        allowed: ["aside", "tr", "main", "option"],
      }],
      "playwright/prefer-native-locators": "error",
      "playwright/no-element-handle": "error",
      "playwright/no-page-pause": "error",
      "playwright/no-force-option": "error",
      "no-restricted-syntax": [
        "error",
        {
          selector: "MemberExpression[property.name='evaluate']",
          message: "page.evaluate() not allowed in spec files.",
        },
        {
          selector: "CallExpression[callee.property.name='waitForTimeout']",
          message: "Arbitrary waits not allowed. Use auto-waiting/assertions.",
        },
        {
          selector: "CallExpression[callee.name='setTimeout']",
          message: "setTimeout waits are not allowed in spec files.",
        },
        {
          selector: "CallExpression[callee.property.name='dispatchEvent']",
          message: "Synthetic events not allowed. Use real user interactions.",
        },
      ],
    },
  },
];

// ESLint config for browser-tests-unmocked
// Enforces human-like interaction patterns in spec files.
// See _dev/BROWSER_TESTING_STANDARDS.md for rationale.
module.exports = {
  plugins: ['playwright'],
  extends: ['plugin:playwright/recommended'],
  overrides: [
    {
      // Spec files: strict human-like interaction only
      files: ['**/*.spec.ts'],
      rules: {
        'playwright/no-eval': 'error',
        'playwright/no-raw-locators': 'error',
        'playwright/prefer-native-locators': 'error',
        'playwright/no-element-handle': 'error',
        'playwright/no-page-pause': 'error',
        'playwright/no-force-option': 'error',
        'no-restricted-syntax': ['error',
          {
            selector: "MemberExpression[object.name='request']",
            message: 'API calls not allowed in spec files. Move to fixtures.ts.'
          },
          {
            selector: "MemberExpression[property.name='evaluate']",
            message: 'page.evaluate() not allowed in spec files.'
          },
          {
            selector: "CallExpression[callee.property.name='waitForTimeout']",
            message: 'Arbitrary waits not allowed. Use Playwright auto-waiting.'
          },
          {
            selector: "CallExpression[callee.property.name='dispatchEvent']",
            message: 'Synthetic events not allowed. Use real user interactions.'
          },
          {
            selector: "CallExpression[callee.property.name='setExtraHTTPHeaders']",
            message: 'setExtraHTTPHeaders not allowed in spec files.'
          }
        ]
      }
    },
    {
      // Fixture/setup files: shortcuts allowed
      files: ['fixtures.ts', 'auth.setup.ts', '**/helpers/**'],
      rules: {
        'playwright/no-raw-locators': 'off',
      }
    }
  ]
};

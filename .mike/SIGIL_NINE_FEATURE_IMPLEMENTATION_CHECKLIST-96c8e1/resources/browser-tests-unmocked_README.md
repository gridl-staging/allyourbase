# Browser Tests (unmocked)

Real browser, real server, real database, zero mocks.
These tests simulate real human interaction with the app.

Read `_dev/BROWSER_TESTING_STANDARDS.md` before writing or modifying any test.


## Directory Structure

```
ui/browser-tests-unmocked/
├── smoke/                          # Critical path tests (2-5 min)
│   ├── admin-login.spec.ts
│   ├── admin-sql-query.spec.ts
│   ├── collections-create.spec.ts
│   ├── storage-upload.spec.ts
│   ├── users-list.spec.ts
│   └── webhooks-crud.spec.ts
├── full/                           # Comprehensive tests (10-15 min)
│   ├── api-explorer.spec.ts
│   ├── api-keys-lifecycle.spec.ts
│   ├── blog-platform-journey.spec.ts
│   ├── collections-crud.spec.ts
│   ├── functions-browser.spec.ts
│   ├── rls-policies.spec.ts
│   ├── storage-lifecycle.spec.ts
│   ├── table-browser-advanced.spec.ts
│   └── webhooks-lifecycle.spec.ts
├── config/                         # Environment configs
│   ├── .env.local
│   ├── .env.staging
│   └── .env.prod
├── auth.setup.ts                   # Login setup (saves storageState)
├── .eslintrc.js                    # Enforces human-like interaction in spec files
├── run-smoke-local.sh
├── run-smoke-staging.sh
├── run-full-staging.sh
├── run-smoke-prod.sh
├── run-staging-to-prod.sh
├── run-local.sh
└── run-on-aws.sh
```


## Quick Start

```bash
# Start your app server first
# <your-start-command>

# Run smoke tests (fast)
cd ui && ./browser-tests-unmocked/run-smoke-local.sh

# Run specific test
npx playwright test browser-tests-unmocked/smoke/admin-login.spec.ts --headed

# Run with UI mode (for debugging)
npx playwright test --ui
```


## Rules

Spec files (*.spec.ts) must simulate real human interaction:

Allowed in spec files:
- page.getByRole(), page.getByText(), page.getByLabel(), page.getByPlaceholder()
- page.getByTestId() as last resort
- Standard Playwright assertions (expect(locator).toBeVisible(), etc.)

Not allowed in spec files (ESLint will error):
- page.evaluate() or page.$eval()
- Raw CSS/XPath locators (page.locator('.class'))
- force: true on any action
- page.waitForTimeout()
- Direct API calls (request.get/post/delete)

Shortcuts (API calls, data seeding, page.goto for setup) are allowed in:
- fixtures.ts
- auth.setup.ts
- helpers/

See _dev/BROWSER_TESTING_STANDARDS.md for the full rules and rationale.


## Environment Configuration

```bash
# Load from config file
export $(cat browser-tests-unmocked/config/.env.staging | xargs)

# Or set directly
export PLAYWRIGHT_ENV=staging
export PLAYWRIGHT_BASE_URL=https://staging.<your-domain>
```


## Resources

- [Browser Testing Standards](../../_dev/BROWSER_TESTING_STANDARDS.md)
- [Playwright Docs](https://playwright.dev)
- [BDD Specifications](../../docs/BDD_SPECIFICATIONS.md)
- [Test Specs](../../tests/specs/)

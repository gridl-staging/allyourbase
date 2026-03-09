# OAuth Contract Tests

Contract tests validate that external OAuth provider APIs (Google, GitHub) match our integration assumptions.

## Purpose

- **NOT** testing our code (that's unit/integration tests)
- **YES** testing external API contracts haven't changed
- Catches breaking changes when providers update their APIs

## When to Run

- ⏰ Weekly (automated via cron/GitHub Actions)
- 🚀 Before major releases
- 📢 When OAuth provider announces API changes
- ❌ NOT on every commit (too slow, requires real credentials)

## Setup

1. **Install dependencies:**
   ```bash
   cd tests/contract
   npm install
   ```

2. **Create `.env.test` from `.env.example`:**
   ```bash
   cp .env.example .env.test
   ```

3. **Add OAuth credentials:**
   - Google: Create OAuth app at https://console.cloud.google.com/apis/credentials
   - GitHub: Create OAuth app at https://github.com/settings/developers
   - Set redirect URIs to `http://localhost:8090/api/auth/oauth/{provider}/callback`
   - Copy client ID and secret to `.env.test`

4. **Obtain test tokens (optional but recommended):**

   **Google access token** (for user info test):
   - Visit https://developers.google.com/oauthplayground
   - Select "Google OAuth2 API v2" → "userinfo.email" + "userinfo.profile"
   - Click "Authorize APIs" and exchange authorization code for tokens
   - Copy `access_token` to `.env.test` as `GOOGLE_TEST_ACCESS_TOKEN`

   **GitHub access token** (for user info test):
   - Visit https://github.com/settings/tokens
   - Generate new personal access token (classic) with `read:user` scope
   - Copy token to `.env.test` as `GITHUB_TEST_ACCESS_TOKEN`

   **Auth codes** (for token exchange test):
   - Auth codes expire in ~10 minutes, so these tests are usually skipped
   - To run: Manually trigger OAuth flow and capture auth code from redirect URL
   - Not recommended for automated testing (use Playwright for E2E instead)

## Running Tests

```bash
# Run all contract tests
npm test

# Watch mode
npm run test:watch

# Run with debug output
DEBUG=1 npm test
```

## Expected Output

```
✓ Google OAuth Contract (3/4 tests)
  ✓ authorization URL is accepted by Google
  ○ token exchange returns expected structure (skipped - no GOOGLE_TEST_AUTH_CODE)
  ✓ user info endpoint returns expected structure
  ✓ token refresh returns expected structure

✓ GitHub OAuth Contract (3/4 tests)
  ✓ authorization URL is accepted by GitHub
  ○ token exchange returns expected structure (skipped - no GITHUB_TEST_AUTH_CODE)
  ✓ user info endpoint returns expected structure

⚠️  2 tests skipped (auth codes expired/unavailable)
```

## CI/CD Integration

**GitHub Actions example:**

```yaml
name: Contract Tests

on:
  schedule:
    - cron: '0 0 * * 0' # Weekly on Sunday
  workflow_dispatch: # Manual trigger

jobs:
  contract-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - name: Install dependencies
        run: |
          cd tests/contract
          npm install
      - name: Run contract tests
        env:
          GOOGLE_CLIENT_ID: ${{ secrets.GOOGLE_CLIENT_ID }}
          GOOGLE_CLIENT_SECRET: ${{ secrets.GOOGLE_CLIENT_SECRET }}
          GOOGLE_TEST_ACCESS_TOKEN: ${{ secrets.GOOGLE_TEST_ACCESS_TOKEN }}
          GITHUB_CLIENT_ID: ${{ secrets.GITHUB_CLIENT_ID }}
          GITHUB_CLIENT_SECRET: ${{ secrets.GITHUB_CLIENT_SECRET }}
          GITHUB_TEST_ACCESS_TOKEN: ${{ secrets.GITHUB_TEST_ACCESS_TOKEN }}
        run: |
          cd tests/contract
          npm test
```

## Troubleshooting

**All tests skipped:**
- Check that `.env.test` exists with correct credentials
- Run `cat .env.test` to verify credentials are set

**401 Unauthorized errors:**
- Access tokens may have expired
- Regenerate tokens using Google OAuth Playground or GitHub Settings
- Update `.env.test` with fresh tokens

**Test failures:**
- **Expected:** This is what contract tests are for!
- **Action:**
  1. Check provider's API changelog for breaking changes
  2. Update our integration code in `internal/auth/oauth.go`
  3. Update contract tests to match new API structure
  4. Re-run tests to verify fix

## What Each Test Validates

### Google OAuth

| Test | Validates | Why It Matters |
|------|-----------|----------------|
| Authorization URL | Google accepts our auth request format | Wrong format = users can't log in |
| Token exchange | Token response structure unchanged | Missing fields = auth flow breaks |
| User info | User data structure unchanged | Missing fields = can't create user |
| Token refresh | Refresh response structure unchanged | Can't maintain sessions |

### GitHub OAuth

| Test | Validates | Why It Matters |
|------|-----------|----------------|
| Authorization URL | GitHub accepts our auth request format | Wrong format = users can't log in |
| Token exchange | Token response structure unchanged | Missing fields = auth flow breaks |
| User info | User data structure unchanged | Missing fields = can't create user |

## Further Reading

- [AI Testing Methodology](_dev/testing/AI_TESTING_METHODOLOGY.md)
- [AYB Testing Strategy](_dev/testing/TESTING.md)
- [OAuth Test Spec](../specs/oauth.md)
- [BDD Specifications](../../docs/BDD_SPECIFICATIONS.md)

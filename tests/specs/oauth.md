# OAuth Test Specification

**User Stories:** B-AUTH-008 (Google), B-AUTH-009 (GitHub), B-REAL-002 (SSE Channel)
**Read this file BEFORE implementing OAuth tests**

---

## Overview

OAuth flow has three test levels:
1. **Unit tests** (SDK) — Test SDK OAuth client logic with mocked EventSource
2. **Integration tests** (Go) — Test server OAuth handlers with mocked provider responses
3. **Contract tests** — Validate real OAuth provider APIs match our assumptions
4. **E2E tests** (Playwright) — Test full OAuth flow through UI

**This spec focuses on contract tests.** Unit/integration tests already exist.

---

## <a id="google-login"></a>TEST: Google OAuth Login (Contract)

**BDD Story:** B-AUTH-008
**Type:** Contract test
**File:** `tests/contract/google-oauth.test.ts`
**Purpose:** Validate that Google OAuth API structure matches our integration assumptions

### Prerequisites

- Google OAuth test credentials (sandbox/test mode)
- Test Google account
- Environment variables:
  - `GOOGLE_CLIENT_ID`
  - `GOOGLE_CLIENT_SECRET`
  - `GOOGLE_TEST_REFRESH_TOKEN` (optional, for token refresh test)

### Test Cases

#### 1. Authorization URL Structure

**Execute:**
```typescript
const authUrl = new URL('https://accounts.google.com/o/oauth2/v2/auth');
authUrl.searchParams.set('client_id', process.env.GOOGLE_CLIENT_ID);
authUrl.searchParams.set('redirect_uri', 'http://localhost:8090/api/auth/oauth/google/callback');
authUrl.searchParams.set('response_type', 'code');
authUrl.searchParams.set('scope', 'openid email profile');
authUrl.searchParams.set('state', 'test-state-123');

const response = await fetch(authUrl.toString());
```

**Verify:**
- Response status is 200 or 302 (redirect to login)
- Google accepts our authorization request format

**Cleanup:** None

---

#### 2. Token Exchange Response Structure

**Fixture:** `tests/fixtures/oauth/google-auth-code.json`
```json
{
  "metadata": {
    "description": "Google OAuth authorization code from test account",
    "expected_token_type": "Bearer",
    "expected_expires_in_range": [3599, 3601],
    "expected_scope": "openid email profile"
  },
  "code": "TEST_AUTH_CODE_PLACEHOLDER"
}
```

**Note:** Real authorization code must be obtained manually or via automated browser testing (out of scope for contract tests). For contract tests, use a recently obtained test code.

**Execute:**
```typescript
const fixture = loadFixture('oauth/google-auth-code.json');

const response = await fetch('https://oauth2.googleapis.com/token', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body: new URLSearchParams({
    code: fixture.code, // Must be real, recently obtained code
    client_id: process.env.GOOGLE_CLIENT_ID!,
    client_secret: process.env.GOOGLE_CLIENT_SECRET!,
    redirect_uri: 'http://localhost:8090/api/auth/oauth/google/callback',
    grant_type: 'authorization_code'
  })
});

const data = await response.json();
```

**Verify:**
```typescript
expect(response.status).toBe(200);
expect(data).toMatchObject({
  access_token: expect.any(String),
  expires_in: expect.any(Number),
  token_type: fixture.metadata.expected_token_type,
  scope: expect.stringContaining('email'),
  id_token: expect.any(String) // OpenID Connect
});

// Verify expires_in is in expected range (3600 ± 1)
expect(data.expires_in).toBeGreaterThanOrEqual(fixture.metadata.expected_expires_in_range[0]);
expect(data.expires_in).toBeLessThanOrEqual(fixture.metadata.expected_expires_in_range[1]);

// Refresh token may or may not be present (depends on access_type=offline)
if (data.refresh_token) {
  expect(data.refresh_token).toEqual(expect.any(String));
}
```

**Cleanup:** None (token will expire)

---

#### 3. User Info Response Structure

**Fixture:** `tests/fixtures/oauth/google-access-token.json`
```json
{
  "metadata": {
    "description": "Google OAuth access token from test account",
    "expected_user_fields": ["sub", "email", "email_verified", "name", "picture"]
  },
  "access_token": "TEST_ACCESS_TOKEN_PLACEHOLDER"
}
```

**Execute:**
```typescript
const fixture = loadFixture('oauth/google-access-token.json');

const response = await fetch('https://www.googleapis.com/oauth2/v3/userinfo', {
  headers: {
    'Authorization': `Bearer ${fixture.access_token}`
  }
});

const data = await response.json();
```

**Verify:**
```typescript
expect(response.status).toBe(200);
expect(data).toMatchObject({
  sub: expect.any(String),           // Google user ID
  email: expect.stringMatching(/.+@.+/), // Email address
  email_verified: expect.any(Boolean),
  name: expect.any(String),
  picture: expect.any(String)         // Profile picture URL
});

// Verify all expected fields are present
fixture.metadata.expected_user_fields.forEach(field => {
  expect(data).toHaveProperty(field);
});
```

**Cleanup:** None

---

#### 4. Token Refresh Response Structure (Optional)

**Only run if GOOGLE_TEST_REFRESH_TOKEN is available.**

**Fixture:** `tests/fixtures/oauth/google-refresh-token.json`
```json
{
  "metadata": {
    "description": "Google OAuth refresh token from test account",
    "expected_token_type": "Bearer"
  },
  "refresh_token": "TEST_REFRESH_TOKEN_PLACEHOLDER"
}
```

**Execute:**
```typescript
const fixture = loadFixture('oauth/google-refresh-token.json');

const response = await fetch('https://oauth2.googleapis.com/token', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body: new URLSearchParams({
    refresh_token: fixture.refresh_token,
    client_id: process.env.GOOGLE_CLIENT_ID!,
    client_secret: process.env.GOOGLE_CLIENT_SECRET!,
    grant_type: 'refresh_token'
  })
});

const data = await response.json();
```

**Verify:**
```typescript
expect(response.status).toBe(200);
expect(data).toMatchObject({
  access_token: expect.any(String),
  expires_in: expect.any(Number),
  token_type: fixture.metadata.expected_token_type,
  scope: expect.stringContaining('email')
});

// Note: Refresh token is NOT returned (reuse existing refresh token)
expect(data.refresh_token).toBeUndefined();
```

**Cleanup:** None

---

## <a id="github-login"></a>TEST: GitHub OAuth Login (Contract)

**BDD Story:** B-AUTH-009
**Type:** Contract test
**File:** `tests/contract/github-oauth.test.ts`
**Purpose:** Validate that GitHub OAuth API structure matches our integration assumptions

### Prerequisites

- GitHub OAuth app credentials (test mode)
- Test GitHub account
- Environment variables:
  - `GITHUB_CLIENT_ID`
  - `GITHUB_CLIENT_SECRET`

### Test Cases

#### 1. Authorization URL Structure

**Execute:**
```typescript
const authUrl = new URL('https://github.com/login/oauth/authorize');
authUrl.searchParams.set('client_id', process.env.GITHUB_CLIENT_ID);
authUrl.searchParams.set('redirect_uri', 'http://localhost:8090/api/auth/oauth/github/callback');
authUrl.searchParams.set('scope', 'read:user user:email');
authUrl.searchParams.set('state', 'test-state-123');

const response = await fetch(authUrl.toString(), { redirect: 'manual' });
```

**Verify:**
- Response status is 302 (redirect to login)
- GitHub accepts our authorization request format

**Cleanup:** None

---

#### 2. Token Exchange Response Structure

**Fixture:** `tests/fixtures/oauth/github-auth-code.json`
```json
{
  "metadata": {
    "description": "GitHub OAuth authorization code from test account",
    "expected_token_type": "bearer",
    "expected_scope": "read:user,user:email"
  },
  "code": "TEST_AUTH_CODE_PLACEHOLDER"
}
```

**Execute:**
```typescript
const fixture = loadFixture('oauth/github-auth-code.json');

const response = await fetch('https://github.com/login/oauth/access_token', {
  method: 'POST',
  headers: {
    'Accept': 'application/json',
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    client_id: process.env.GITHUB_CLIENT_ID!,
    client_secret: process.env.GITHUB_CLIENT_SECRET!,
    code: fixture.code,
    redirect_uri: 'http://localhost:8090/api/auth/oauth/github/callback'
  })
});

const data = await response.json();
```

**Verify:**
```typescript
expect(response.status).toBe(200);
expect(data).toMatchObject({
  access_token: expect.any(String),
  token_type: fixture.metadata.expected_token_type,
  scope: expect.any(String)
});

// GitHub doesn't return expires_in (tokens don't expire)
expect(data.expires_in).toBeUndefined();

// No refresh token (GitHub tokens don't expire)
expect(data.refresh_token).toBeUndefined();
```

**Cleanup:** None

---

#### 3. User Info Response Structure

**Fixture:** `tests/fixtures/oauth/github-access-token.json`
```json
{
  "metadata": {
    "description": "GitHub OAuth access token from test account",
    "expected_user_fields": ["id", "login", "email", "name", "avatar_url"]
  },
  "access_token": "TEST_ACCESS_TOKEN_PLACEHOLDER"
}
```

**Execute:**
```typescript
const fixture = loadFixture('oauth/github-access-token.json');

const response = await fetch('https://api.github.com/user', {
  headers: {
    'Authorization': `Bearer ${fixture.access_token}`,
    'Accept': 'application/vnd.github+json',
    'X-GitHub-Api-Version': '2022-11-28'
  }
});

const data = await response.json();
```

**Verify:**
```typescript
expect(response.status).toBe(200);
expect(data).toMatchObject({
  id: expect.any(Number),             // GitHub user ID
  login: expect.any(String),          // Username
  email: expect.stringMatching(/.+@.+/), // Email (may be null if not public)
  name: expect.any(String),           // Display name
  avatar_url: expect.any(String)      // Avatar URL
});

// Verify all expected fields are present
fixture.metadata.expected_user_fields.forEach(field => {
  expect(data).toHaveProperty(field);
});
```

**Cleanup:** None

---

## <a id="sse-channel"></a>TEST: OAuth SSE Channel (Integration Test)

**BDD Story:** B-REAL-002
**Type:** Integration test (NOT contract test)
**File:** `internal/realtime/handler_integration_test.go`
**Purpose:** Test that OAuth results are delivered via SSE

### Test Cases

#### 1. SSE Connection Returns Client ID

**Execute:**
1. Connect to `/api/realtime?oauth=true`
2. Wait for `event: connected`

**Verify:**
- Event data contains `{clientId: string}`
- Client ID is valid UUID

**Cleanup:** Close SSE connection

---

#### 2. OAuth Success Delivered via SSE

**Execute:**
1. Connect to `/api/realtime?oauth=true`
2. Extract clientId from connected event
3. Simulate OAuth callback success (backend sends oauth event)
4. Wait for `event: oauth`

**Verify:**
- Event data contains `{token: string, refreshToken: string, user: object}`
- Token is valid JWT
- User object has expected fields

**Cleanup:** Close SSE connection

---

#### 3. OAuth Error Delivered via SSE

**Execute:**
1. Connect to `/api/realtime?oauth=true`
2. Extract clientId from connected event
3. Simulate OAuth callback error (backend sends oauth event with error)
4. Wait for `event: oauth`

**Verify:**
- Event data contains `{error: string}`
- Error message is descriptive

**Cleanup:** Close SSE connection

---

## Fixture Data Needed

**Create these fixtures:**

1. `tests/fixtures/oauth/google-auth-code.json` — Google auth code (manual/automated)
2. `tests/fixtures/oauth/google-access-token.json` — Google access token
3. `tests/fixtures/oauth/google-refresh-token.json` — Google refresh token (optional)
4. `tests/fixtures/oauth/github-auth-code.json` — GitHub auth code (manual/automated)
5. `tests/fixtures/oauth/github-access-token.json` — GitHub access token

**Note:** Auth codes expire quickly (10 min for Google). Contract tests should either:
- Use recently obtained codes (manual process)
- Skip token exchange test if code unavailable (test only user info endpoint)
- Use automated browser to obtain codes (e.g., Playwright login flow)

---

## Implementation Notes

**When to run contract tests:**
- Weekly (automated via cron)
- Before major releases
- When OAuth provider announces API changes
- NOT on every commit (too slow, requires real credentials)

**Handling test credentials:**
- Store in `.env.test` (gitignored)
- Use GitHub Secrets for CI
- Never commit real tokens to git

**Handling expired codes:**
- Contract tests should gracefully skip if auth code expired
- Log warning: "Skipping token exchange test (auth code expired)"
- Still run user info tests (access tokens last longer)

**Provider API versioning:**
- Google: API version in URL (`/oauth2/v3/userinfo`)
- GitHub: API version in header (`X-GitHub-Api-Version: 2022-11-28`)
- Update tests when provider releases new API version

---

## OAuth 2.0 Provider Mode Test Cases

AYB's OAuth provider mode (Stage 2) is tested at the Go unit/integration level. All tests are in `internal/auth/` and `internal/server/`.

### Authorization Code Flow

| Test | File | Description |
|------|------|-------------|
| Happy path | `oauth_authorize_handler_test.go` | Authorize with valid params + prior consent → redirect with code+state |
| Missing params | `oauth_authorize_handler_test.go` | Missing client_id, redirect_uri, scope, state, code_challenge → error |
| Invalid response_type | `oauth_authorize_handler_test.go` | response_type != "code" → error |
| Unknown client | `oauth_authorize_handler_test.go` | Non-existent client_id → invalid_client |
| Revoked client | `oauth_authorize_handler_test.go` | Revoked client rejected at authorize + consent |
| Redirect URI mismatch | `oauth_authorize_handler_test.go` | URI not in registered list → error |
| Scope exceeds client | `oauth_authorize_handler_test.go` | Requested scope not in client's scopes → invalid_scope |
| PKCE S256 only | `oauth_authorize_handler_test.go` | plain method rejected, missing challenge rejected |
| Consent prompt | `oauth_authorize_handler_test.go` | No prior consent → returns consent prompt JSON |
| Consent deny | `oauth_authorize_handler_test.go` | User denies → redirect with error=access_denied |
| Consent approve | `oauth_authorize_handler_test.go` | User approves → consent saved, redirect with code |
| Unauthenticated | `oauth_authorize_handler_test.go` | No session → 401 |

### Token Endpoint

| Test | File | Description |
|------|------|-------------|
| Auth code exchange | `oauth_token_handler_test.go` | Valid code+PKCE+redirect → access+refresh token pair |
| PKCE verification | `oauth_provider_test.go` | Wrong code_verifier → invalid_grant |
| Code replay | `oauth_provider_test.go` | Code already used (used_at set) → invalid_grant |
| Expired code | `oauth_provider_test.go` | Code past TTL → invalid_grant |
| Client mismatch | `oauth_provider_test.go` | Different client_id than code's → invalid_grant |
| Redirect URI mismatch | `oauth_provider_test.go` | Different redirect_uri → invalid_grant |
| Client credentials | `oauth_token_handler_test.go` | Confidential client + secret → access token (no refresh) |
| Client credentials public | `oauth_token_handler_test.go` | Public client → unauthorized_client |
| Missing grant_type | `oauth_token_handler_test.go` | No grant_type → invalid_request |
| Missing params | `oauth_token_handler_test.go` | Missing code/redirect_uri/code_verifier/refresh_token → invalid_request |
| Invalid scope | `oauth_token_handler_test.go` | Scope not in client's registered scopes → invalid_scope |
| Unknown client | `oauth_token_handler_test.go` | Non-existent client_id → invalid_client |
| Unsupported grant | `oauth_token_handler_test.go` | Unknown grant_type → unsupported_grant_type |
| Client auth (Basic) | `oauth_token_handler_test.go` | HTTP Basic auth with client_id:client_secret |
| Client auth (POST) | `oauth_token_handler_test.go` | POST body client_id + client_secret |
| Dual auth rejected | `oauth_token_handler_test.go` | Both Basic + POST body → invalid_request |

### Refresh Token

| Test | File | Description |
|------|------|-------------|
| Rotation | `oauth_provider_test.go` | Use refresh → old revoked, new pair issued |
| Reuse detection | `oauth_provider_test.go` | Reuse rotated token → ALL grant tokens revoked |
| Expired refresh | `oauth_provider_test.go` | Expired refresh → invalid_grant |
| Client mismatch | `oauth_provider_test.go` | Wrong client_id → invalid_grant |

### Token Revocation (RFC 7009)

| Test | File | Description |
|------|------|-------------|
| Revoke access token | `oauth_revoke_handler_test.go` | Access token revoked, validation fails after |
| Revoke refresh token | `oauth_revoke_handler_test.go` | Refresh token + all grant tokens revoked |
| Unknown token | `oauth_revoke_handler_test.go` | Non-existent token → 200 OK (per RFC 7009) |
| Service error | `oauth_revoke_handler_test.go` | DB unavailable → 200 OK (return-safe) |

### Token Validation & Middleware

| Test | File | Description |
|------|------|-------------|
| Valid OAuth token | `oauth_provider_test.go` | Valid access token → scope + user + app info |
| Revoked token | `oauth_provider_test.go` | Revoked token → rejected |
| Expired token | `oauth_provider_test.go` | Expired token → rejected |
| Scope enforcement | `oauth_provider_integration_test.go` | readonly → GET only; readwrite → all CRUD |
| allowed_tables | `oauth_provider_integration_test.go` | Access restricted to specified tables |
| Rate limiting | `oauth_provider_integration_test.go` | Inherits app rate limits → 429 when exceeded |
| CORS headers | `oauth_provider_integration_test.go` | Token + revoke endpoints allow cross-origin POST |
| Mixed auth compat | `oauth_provider_integration_test.go` | OAuth + API key + JWT session all work |

### Client Registration (Admin)

| Test | File | Description |
|------|------|-------------|
| Create confidential | `oauth_clients_test.go` | Returns client_id + one-time client_secret |
| Create public | `oauth_clients_test.go` | No secret returned or stored |
| Client ID format | `oauth_clients_test.go` | ayb_cid_ + 48 lowercase hex |
| Redirect URI validation | `oauth_clients_test.go` | HTTPS required, no wildcards, localhost allowed |
| Scope validation | `oauth_clients_test.go` | Only readonly/readwrite/* allowed |
| Update client | `oauth_clients_test.go` | Name, redirect URIs, scopes updatable |
| Revoke client | `oauth_clients_test.go` | Soft-delete via revoked_at |
| Rotate secret | `oauth_clients_test.go` | New secret, old invalidated |
| Admin handler CRUD | `internal/server/oauth_clients_handler_test.go` | HTTP handler tests for all admin endpoints |

### Consent

| Test | File | Description |
|------|------|-------------|
| New consent prompt | `oauth_authorize_handler_test.go` | No prior consent → consent prompt response |
| Prior consent skip | `oauth_authorize_handler_test.go` | Matching consent → immediate redirect |
| Consent persistence | `oauth_provider_test.go` | Consent saved and retrieved correctly |
| UI redirect-to-login | `OAuthConsent.test.tsx` | 401 → redirect to login with return_to |
| UI scope display | `OAuthConsent.test.tsx` | Scope descriptions rendered correctly |
| UI approve/deny | `OAuthConsent.test.tsx` | Approve and deny interactions |

---

**Spec Version:** 2.0
**Last Updated:** 2026-02-22 (Session 040)

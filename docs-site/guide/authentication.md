# Authentication
<!-- audited 2026-03-20 -->

AYB provides built-in email/password authentication with JWT sessions, OAuth support, email verification, password reset, magic links, SMS OTP auth, SMS MFA, anonymous auth with account linking, TOTP MFA with backup codes, email MFA, and authentication assurance levels (AAL).

## Enable auth

```toml
# ayb.toml
[auth]
enabled = true
jwt_secret = "your-secret-key-at-least-32-characters-long"
```

Or via environment variables:

```bash
AYB_AUTH_ENABLED=true
AYB_AUTH_JWT_SECRET="your-secret-key-at-least-32-characters-long"
```

## Endpoints

### Register

```bash
curl -X POST http://localhost:8090/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "securepassword"}'
```

**Response** (201 Created):

```json
{
  "token": "eyJhbG...",
  "refreshToken": "eyJhbG...",
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "emailVerified": false,
    "createdAt": "2026-02-07T..."
  }
}
```

### Login

```bash
curl -X POST http://localhost:8090/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "securepassword"}'
```

Returns the same response format as register.

### Get current user

```bash
curl http://localhost:8090/api/auth/me \
  -H "Authorization: Bearer eyJhbG..."
```

### Refresh token

```bash
curl -X POST http://localhost:8090/api/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refreshToken": "eyJhbG..."}'
```

### Logout

```bash
curl -X POST http://localhost:8090/api/auth/logout \
  -H "Content-Type: application/json" \
  -d '{"refreshToken": "eyJhbG..."}'
```

`POST /api/auth/logout` revokes the provided refresh token; it does not require an
`Authorization` header.

### Session management

User sessions can be inspected and revoked:

- `GET /api/auth/sessions` — list active sessions for the current user
- `DELETE /api/auth/sessions/{id}` — revoke one session
- `DELETE /api/auth/sessions?all_except_current=true` — revoke all other sessions for the current user

## SMS OTP auth

Enable SMS auth in config:

```toml
[auth]
sms_enabled = true
sms_provider = "log" # log, twilio, plivo, telnyx, msg91, sns, vonage, webhook
sms_code_length = 6
sms_code_expiry = 300
sms_max_attempts = 3
```

Request an OTP:

```bash
curl -X POST http://localhost:8090/api/auth/sms \
  -H "Content-Type: application/json" \
  -d '{"phone": "+14155552671"}'
```

Confirm OTP:

```bash
curl -X POST http://localhost:8090/api/auth/sms/confirm \
  -H "Content-Type: application/json" \
  -d '{"phone": "+14155552671", "code": "123456"}'
```

`/api/auth/sms` always returns `200` to avoid phone-number enumeration.

## SMS MFA

When SMS auth is enabled, MFA routes are available:

- `POST /api/auth/mfa/sms/enroll`
- `POST /api/auth/mfa/sms/enroll/confirm`
- `POST /api/auth/mfa/sms/challenge`
- `POST /api/auth/mfa/sms/verify`

Enroll:

```bash
curl -X POST http://localhost:8090/api/auth/mfa/sms/enroll \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"phone": "+14155552671"}'
```

## Anonymous auth

Anonymous auth lets users start using your app without signing up. They get a real user ID and session, and can later link their account to an email/password or OAuth identity.

### Enable

```toml
[auth]
anonymous_auth_enabled = true
```

Or: `AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true`

### Create anonymous session

```bash
curl -X POST http://localhost:8090/api/auth/anonymous
```

**Response** (201 Created):

```json
{
  "token": "eyJhbG...",
  "refreshToken": "eyJhbG...",
  "user": {
    "id": "uuid",
    "email": "",
    "is_anonymous": true,
    "createdAt": "2026-02-24T..."
  }
}
```

No request body is needed. Rate limited to 30 requests per hour per IP.

### Link email + password

Convert an anonymous user to a credentialed account:

```bash
curl -X POST http://localhost:8090/api/auth/link/email \
  -H "Authorization: Bearer <anonymous-token>" \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "securepassword"}'
```

**Response** (200 OK): Returns new tokens with `is_anonymous: false`. The user ID is preserved — all data created during the anonymous session stays with the account.

If the email is already taken, returns **409 Conflict**.

### Link OAuth identity

```bash
curl -X POST http://localhost:8090/api/auth/link/oauth \
  -H "Authorization: Bearer <anonymous-token>" \
  -H "Content-Type: application/json" \
  -d '{"provider": "google", "access_token": "<provider-access-token>"}'
```

Same behavior as email linking: preserves user ID, returns new tokens, returns 409 on conflict.

### Restrictions

- Anonymous users **cannot** enroll in MFA. Link the account first.
- Unlinked anonymous accounts become eligible for cleanup after 30 days by default.
  AYB ships the cleanup helper (`CleanupAnonymousUsers`) but does not schedule that
  cleanup automatically.

## TOTP MFA (Authenticator App)

TOTP (Time-based One-Time Password) provides NIST-compliant multi-factor authentication using authenticator apps like Google Authenticator, Authy, or 1Password.

### Enable

```toml
[auth]
totp_enabled = true
```

Or: `AYB_AUTH_TOTP_ENABLED=true`

### Enrollment

**Step 1 — Start enrollment:**

```bash
curl -X POST http://localhost:8090/api/auth/mfa/totp/enroll \
  -H "Authorization: Bearer <token>"
```

**Response** (200 OK):

```json
{
  "factor_id": "uuid",
  "uri": "otpauth://totp/AllYourBase:user@example.com?secret=JBSWY3DPEHPK3PXP&issuer=AllYourBase",
  "secret": "JBSWY3DPEHPK3PXP"
}
```

Display the `uri` as a QR code for the user to scan. The `secret` is shown once for manual entry.

**Step 2 — Confirm enrollment:**

```bash
curl -X POST http://localhost:8090/api/auth/mfa/totp/enroll/confirm \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"code": "123456"}'
```

The user must enter a valid code from their authenticator app to confirm the enrollment is working.

### MFA login flow

When a user with TOTP enrolled logs in, the login response changes:

```json
{
  "mfa_pending": true,
  "mfa_token": "eyJhbG..."
}
```

Use the `mfa_token` (not a regular token) for the challenge/verify steps:

**Step 1 — Create challenge:**

```bash
curl -X POST http://localhost:8090/api/auth/mfa/totp/challenge \
  -H "Authorization: Bearer <mfa_token>"
```

**Response:** `{"challenge_id": "uuid"}`

**Step 2 — Verify code:**

```bash
curl -X POST http://localhost:8090/api/auth/mfa/totp/verify \
  -H "Authorization: Bearer <mfa_token>" \
  -H "Content-Type: application/json" \
  -d '{"challenge_id": "uuid", "code": "123456"}'
```

**Response** (200 OK): Returns full access + refresh tokens with `aal: "aal2"`.

### TOTP parameters

| Parameter | Value |
|-----------|-------|
| Algorithm | SHA-1 |
| Digits | 6 |
| Period | 30 seconds |
| Skew | ±1 window (accepts codes from adjacent 30s windows) |

These settings are compatible with all major authenticator apps.

## Email MFA

Email MFA sends a one-time code to the user's email for step-up verification.

> **Security note:** Email MFA does not meet NIST SP 800-63B requirements for out-of-band authentication because email does not prove device possession. Use TOTP for true multi-factor security. Email MFA is best suited as step-up verification for lower-risk operations.

### Enable

```toml
[auth]
email_mfa_enabled = true
```

Or: `AYB_AUTH_EMAIL_MFA_ENABLED=true`

Requires a configured email backend (SMTP or provider).

### Enrollment

```bash
# Start enrollment (sends verification code to user's email)
curl -X POST http://localhost:8090/api/auth/mfa/email/enroll \
  -H "Authorization: Bearer <token>"

# Confirm enrollment with the code from email
curl -X POST http://localhost:8090/api/auth/mfa/email/enroll/confirm \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"code": "123456"}'
```

### MFA login flow

Same pattern as TOTP — login returns `mfa_pending`, then challenge + verify:

```bash
# Create challenge (sends code to email)
curl -X POST http://localhost:8090/api/auth/mfa/email/challenge \
  -H "Authorization: Bearer <mfa_token>"

# Verify code
curl -X POST http://localhost:8090/api/auth/mfa/email/verify \
  -H "Authorization: Bearer <mfa_token>" \
  -H "Content-Type: application/json" \
  -d '{"challenge_id": "uuid", "code": "123456"}'
```

## Backup codes

Backup codes provide recovery access when the primary MFA device is unavailable. They are generated as 10 single-use codes in `xxxxx-xxxxx` format.

### Generate

Requires an AAL2 session (must have completed MFA verification):

```bash
curl -X POST http://localhost:8090/api/auth/mfa/backup/generate \
  -H "Authorization: Bearer <aal2-token>"
```

**Response** (200 OK):

```json
{
  "codes": [
    "a1b2c-d3e4f",
    "g5h6i-j7k8l",
    ...
  ]
}
```

**Store these codes securely.** They are shown once and cannot be retrieved again.

### Use a backup code

During MFA verification, use a backup code instead of TOTP/email:

```bash
curl -X POST http://localhost:8090/api/auth/mfa/backup/verify \
  -H "Authorization: Bearer <mfa_token>" \
  -H "Content-Type: application/json" \
  -d '{"code": "a1b2c-d3e4f"}'
```

Returns full AAL2 tokens. Each code can only be used once.

### Check remaining codes

```bash
curl -X GET http://localhost:8090/api/auth/mfa/backup/count \
  -H "Authorization: Bearer <token>"
```

**Response:** `{"remaining": 8}`

### Regenerate

Invalidates all existing codes and generates a new set of 10:

```bash
curl -X POST http://localhost:8090/api/auth/mfa/backup/regenerate \
  -H "Authorization: Bearer <aal2-token>"
```

## Factor selection

When a user has multiple MFA methods enrolled, list available factors:

```bash
curl -X GET http://localhost:8090/api/auth/mfa/factors \
  -H "Authorization: Bearer <mfa_token>"
```

**Response:**

```json
{
  "factors": [
    {"id": "uuid", "method": "totp"},
    {"id": "uuid", "method": "email"},
    {"id": "uuid", "method": "sms", "phone": "+1***5671"}
  ]
}
```

The client can then challenge the user's preferred factor.

## Authentication assurance levels (AAL)

AAL indicates the strength of the current session's authentication:

| Level | Meaning | When issued |
|-------|---------|-------------|
| `aal1` | Single-factor authentication | After password, OAuth, SMS OTP, or anonymous login |
| `aal2` | Multi-factor authentication | After successful MFA verification (TOTP, email, SMS, or backup code) |

### Enforce AAL2 on sensitive routes

Use the `RequireAAL2` middleware or check the `aal` claim in RLS policies:

```sql
-- Only allow AAL2 sessions to access sensitive data
CREATE POLICY sensitive_data_policy ON financial_records
  FOR ALL
  USING (current_setting('ayb.aal', true) = 'aal2');
```

### Token claims

Access tokens include these MFA-related claims:

| Claim | Type | Description |
|-------|------|-------------|
| `aal` | string | `"aal1"` or `"aal2"` |
| `amr` | string[] | Authentication methods used, e.g. `["password", "totp"]` |
| `mfa_pending` | boolean | `true` when first factor passed but MFA verification is still needed |
| `is_anonymous` | boolean | `true` for anonymous user sessions |

The `amr` (Authentication Method Reference) array records which methods were used:

| Value | Method |
|-------|--------|
| `password` | Email/password login |
| `otp` | SMS OTP login |
| `oauth` | OAuth provider login |
| `anonymous` | Anonymous sign-in |
| `totp` | TOTP authenticator app |
| `email` | Email MFA code |
| `sms` | SMS MFA code |
| `backup_code` | Backup code |

### Refresh behavior

Refreshing an AAL2 session produces a new AAL2 token. Refresh **never** elevates AAL — only MFA verification can do that.

## Security properties

| Method | NIST compliance | Notes |
|--------|----------------|-------|
| **TOTP** | Meets AAL2 (NIST SP 800-63B) | True multi-factor: proves device possession via shared secret |
| **SMS MFA** | "Restricted authenticator" per NIST | Vulnerable to SIM-swap and SS7 attacks; acceptable but not recommended |
| **Email MFA** | Does **not** meet NIST out-of-band requirements | Email does not prove device possession; use as step-up verification, not as sole MFA method |
| **Backup codes** | Recovery mechanism | Not a standing MFA method; single-use emergency access |

Internally, all MFA methods issue `aal2` tokens to keep the authorization model simple. The distinction matters for compliance reporting and risk assessment, not for API behavior.

## Operational limits

| Parameter | Default | Description |
|-----------|---------|-------------|
| TOTP code window | ±30s | Accepts codes from the current and adjacent 30-second windows |
| TOTP replay protection | Per-factor | Each code's time step is recorded; reuse of the same or earlier time step is rejected |
| Email MFA code TTL | 10 minutes | Codes expire after 10 minutes |
| Email MFA attempts per code | 5 | Code is invalidated after 5 failed attempts |
| Email MFA challenges per user | 3 per 10 minutes | Prevents inbox flooding |
| Cumulative MFA lockout | 15 failures/hour | All MFA methods lock for 30 minutes after 15 failures within 1 hour |
| MFA challenge expiry (TOTP/SMS default) | 5 minutes | TOTP challenge rows default to 5 minutes; SMS code expiry is configured via `auth.sms_code_expiry` |
| Backup code count | 10 | Each generation/regeneration produces 10 codes |
| Anonymous sign-in rate limit | 30/hour per IP | Prevents anonymous account abuse |
| Anonymous account TTL | 30 days | Default retention used by the anonymous cleanup helper; run cleanup on your own schedule |
| Unverified TOTP enrollment TTL | 10 minutes | Default TTL used by `CleanupUnverifiedTOTPEnrollments`; run cleanup on your own schedule |

## JWT structure

Access tokens are short-lived (default: 15 minutes). Refresh tokens are long-lived (default: 7 days).

Send the access token in the `Authorization` header:

```
Authorization: Bearer <token>
```

Configure token durations:

```toml
[auth]
token_duration = 900         # 15 minutes (seconds)
refresh_token_duration = 604800  # 7 days (seconds)
```

## Password reset

### Request reset

```bash
curl -X POST http://localhost:8090/api/auth/password-reset \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com"}'
```

Sends a reset link via the configured email backend.

### Confirm reset

```bash
curl -X POST http://localhost:8090/api/auth/password-reset/confirm \
  -H "Content-Type: application/json" \
  -d '{"token": "reset-token-from-email", "password": "newpassword"}'
```

## Email verification

### Verify email

```bash
curl -X POST http://localhost:8090/api/auth/verify \
  -H "Content-Type: application/json" \
  -d '{"token": "verification-token-from-email"}'
```

### Resend verification

```bash
curl -X POST http://localhost:8090/api/auth/verify/resend \
  -H "Authorization: Bearer eyJhbG..."
```

## OAuth

AYB supports built-in OAuth providers (`google`, `github`, `microsoft`, `apple`, `discord`, `twitter`, `facebook`, `linkedin`, `spotify`, `twitch`, `gitlab`, `bitbucket`, `slack`, `zoom`, `notion`, `figma`) and custom OIDC providers.

### Configure

```toml
[auth]
enabled = true
jwt_secret = "..."
oauth_redirect_url = "http://localhost:5173/oauth-callback"

[auth.oauth.google]
enabled = true
client_id = "your-google-client-id"
client_secret = "your-google-client-secret"

[auth.oauth.github]
enabled = true
client_id = "your-github-client-id"
client_secret = "your-github-client-secret"
```

### Flow

1. Redirect the user to `GET /api/auth/oauth/google` (or `github`)
2. AYB redirects to the provider's consent screen
3. After approval, the provider redirects back to AYB's callback
4. AYB redirects to your `oauth_redirect_url` with tokens as hash fragments:
   ```
   http://localhost:5173/oauth-callback#token=eyJ...&refreshToken=eyJ...
   ```

### Environment variables

```bash
AYB_AUTH_OAUTH_GOOGLE_ENABLED=true
AYB_AUTH_OAUTH_GOOGLE_CLIENT_ID=...
AYB_AUTH_OAUTH_GOOGLE_CLIENT_SECRET=...
AYB_AUTH_OAUTH_GITHUB_ENABLED=true
AYB_AUTH_OAUTH_GITHUB_CLIENT_ID=...
AYB_AUTH_OAUTH_GITHUB_CLIENT_SECRET=...
AYB_AUTH_OAUTH_REDIRECT_URL=http://localhost:5173/oauth-callback
```

## OAuth 2.0 Provider Mode

In addition to consuming external OAuth providers (Google, GitHub, and other built-in providers), AYB can act as an OAuth 2.0 authorization server itself. This lets third-party applications request scoped access to your AYB instance on behalf of users.

Use OAuth provider mode when you want to:

- Let third-party apps access your AYB data with user consent
- Issue scoped, revocable access tokens to external clients
- Support the standard authorization code flow with PKCE

Enable it in config:

```toml
[auth.oauth_provider]
enabled = true
access_token_duration = 3600     # 1 hour (seconds)
refresh_token_duration = 2592000 # 30 days (seconds)
auth_code_duration = 600         # 10 minutes (seconds)
```

Supported grant types: `authorization_code` (with PKCE S256, required for all clients) and `client_credentials`. OAuth tokens are opaque (not JWTs) and can be revoked individually.

For the full walkthrough, see the [OAuth Provider Guide](./oauth-provider.md).

## Row-Level Security (RLS)

When auth is enabled, AYB injects JWT claims into PostgreSQL session variables before each query. This lets you use standard Postgres RLS policies:

```sql
-- Enable RLS on a table
ALTER TABLE posts ENABLE ROW LEVEL SECURITY;

-- Users can only see their own posts
CREATE POLICY posts_select ON posts
  FOR SELECT
  USING (author_id = current_setting('ayb.user_id')::uuid);

-- Users can only insert posts as themselves
CREATE POLICY posts_insert ON posts
  FOR INSERT
  WITH CHECK (author_id = current_setting('ayb.user_id')::uuid);
```

Available session variables:

| Variable | Value |
|----------|-------|
| `ayb.user_id` | The authenticated user's ID |
| `ayb.user_email` | The authenticated user's email |

These are set per-request and scoped to the database connection for that query.

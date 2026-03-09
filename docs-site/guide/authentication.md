# Authentication

AYB provides built-in email/password authentication with JWT sessions, OAuth support, email verification, password reset, magic links, SMS OTP auth, and SMS MFA.

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
  -H "Authorization: Bearer eyJhbG..." \
  -H "Content-Type: application/json" \
  -d '{"refreshToken": "eyJhbG..."}'
```

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

AYB supports Google and GitHub OAuth.

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

In addition to consuming external OAuth providers (Google/GitHub), AYB can act as an OAuth 2.0 authorization server itself. This lets third-party applications request scoped access to your AYB instance on behalf of users.

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

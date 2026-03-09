# OAuth 2.0 Provider Mode

AYB can act as an OAuth 2.0 authorization server, allowing third-party applications to request scoped access to your AYB instance on behalf of users.

## Overview

OAuth provider mode adds:

- **Client registration** — Register OAuth clients linked to apps, with redirect URIs and scopes
- **Authorization code flow** — Standard user-consent-based flow with PKCE (S256 required)
- **Client credentials flow** — Machine-to-machine tokens for confidential clients (no user context)
- **Token lifecycle** — Opaque access/refresh tokens with rotation and revocation
- **Consent management** — Users approve third-party access; prior consent is remembered

## Enable

```toml
# ayb.toml
[auth]
enabled = true
jwt_secret = "your-secret-key-at-least-32-characters-long"

[auth.oauth_provider]
enabled = true
access_token_duration = 3600     # 1 hour (seconds)
refresh_token_duration = 2592000 # 30 days (seconds)
auth_code_duration = 600         # 10 minutes (seconds)
```

Or via environment variables:

```bash
AYB_AUTH_OAUTH_PROVIDER_ENABLED=true
AYB_AUTH_OAUTH_PROVIDER_ACCESS_TOKEN_DURATION=3600
AYB_AUTH_OAUTH_PROVIDER_REFRESH_TOKEN_DURATION=2592000
AYB_AUTH_OAUTH_PROVIDER_AUTH_CODE_DURATION=600
```

OAuth provider mode requires `auth.enabled = true` and `auth.jwt_secret` set (the JWT secret is used for session tokens that the consent flow depends on, not for OAuth tokens themselves).

## Client Registration

OAuth clients are registered via the admin API or CLI. Each client is linked to an AYB app (from the apps system) and inherits its rate limits.

### Via CLI

```bash
# Create a confidential client
ayb oauth clients create <app-id> \
  --name "My SPA" \
  --redirect-uris "https://myapp.com/callback" \
  --scopes "readonly" \
  --type confidential

# Output:
# Client ID: ayb_cid_a1b2c3...
# Client Secret: ayb_cs_x9y8z7... (shown once — save it!)
```

```bash
# Create a public client (no secret)
ayb oauth clients create <app-id> \
  --name "Mobile App" \
  --redirect-uris "http://localhost:3000/callback" \
  --scopes "readwrite" \
  --type public
```

```bash
# List all clients
ayb oauth clients list
ayb oauth clients list --json

# Revoke a client (soft-delete)
ayb oauth clients delete <client-id>

# Rotate a confidential client's secret
ayb oauth clients rotate-secret <client-id>
```

### Via Admin API

```bash
# Create client
curl -X POST http://localhost:8090/api/admin/oauth/clients \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "appId": "00000000-0000-0000-0000-000000000001",
    "name": "My SPA",
    "clientType": "confidential",
    "redirectUris": ["https://myapp.com/callback"],
    "scopes": ["readonly"]
  }'
```

**Response** (201 Created):

```json
{
  "clientSecret": "ayb_cs_...",
  "client": {
    "id": "uuid",
    "appId": "uuid",
    "clientId": "ayb_cid_...",
    "name": "My SPA",
    "redirectUris": ["https://myapp.com/callback"],
    "scopes": ["readonly"],
    "clientType": "confidential",
    "createdAt": "2026-02-22T...",
    "updatedAt": "2026-02-22T...",
    "revokedAt": null,
    "activeAccessTokenCount": 0,
    "activeRefreshTokenCount": 0,
    "totalGrants": 0,
    "lastTokenIssuedAt": null
  }
}
```

The `clientSecret` is only returned on creation and secret rotation. Store it securely.

## Authorization Code Flow with PKCE

This is the standard flow for web and mobile applications that need to act on behalf of a user.

### Step 1: Generate PKCE parameters

```javascript
// Generate code_verifier (43-128 chars, URL-safe random)
const verifier = crypto.randomUUID() + crypto.randomUUID();

// Generate code_challenge = base64url(SHA-256(verifier))
const encoder = new TextEncoder();
const digest = await crypto.subtle.digest("SHA-256", encoder.encode(verifier));
const challenge = btoa(String.fromCharCode(...new Uint8Array(digest)))
  .replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
```

### Step 2: Redirect to authorization endpoint

```
GET /api/auth/authorize?
  response_type=code&
  client_id=ayb_cid_...&
  redirect_uri=https://myapp.com/callback&
  scope=readonly&
  state=random-csrf-token&
  code_challenge=<challenge>&
  code_challenge_method=S256
```

The user must be logged in (session JWT). If no prior consent exists for this client+scope, the endpoint returns a consent prompt. If consent was already granted, it redirects immediately with the authorization code.

### Step 3: User grants consent

The consent page shows the requesting app name, requested permissions, and approve/deny buttons. On approval, AYB redirects to:

```
https://myapp.com/callback?code=<auth-code>&state=<state>
```

On denial:

```
https://myapp.com/callback?error=access_denied&error_description=resource+owner+denied+access&state=<state>
```

### Step 4: Exchange code for tokens

```bash
curl -X POST http://localhost:8090/api/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&\
code=<auth-code>&\
redirect_uri=https://myapp.com/callback&\
code_verifier=<verifier>&\
client_id=ayb_cid_...&\
client_secret=ayb_cs_..."
```

**Response** (200 OK):

```json
{
  "access_token": "ayb_at_...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "ayb_rt_...",
  "scope": "readonly"
}
```

Authorization codes are single-use and expire after 10 minutes (configurable).

### Step 5: Use the access token

```bash
curl http://localhost:8090/api/collections/posts \
  -H "Authorization: Bearer ayb_at_..."
```

OAuth access tokens work exactly like session tokens and API keys in the `Authorization: Bearer` header.

## Client Credentials Flow

For machine-to-machine access without a user context. Only available to confidential clients.

```bash
curl -X POST http://localhost:8090/api/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -u "ayb_cid_...:ayb_cs_..." \
  -d "grant_type=client_credentials&scope=readonly"
```

**Response** (200 OK):

```json
{
  "access_token": "ayb_at_...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "scope": "readonly"
}
```

No refresh token is issued for client credentials grants.

## Token Lifecycle

### Refresh tokens

Access tokens expire after 1 hour (configurable). Use the refresh token to get a new pair:

```bash
curl -X POST http://localhost:8090/api/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token&\
refresh_token=ayb_rt_...&\
client_id=ayb_cid_...&\
client_secret=ayb_cs_..."
```

Refresh token rotation is enforced: each refresh token can only be used once. A new access+refresh pair is issued, and the old refresh token is invalidated.

**Reuse detection:** If a previously-rotated refresh token is used again (indicating possible token theft), ALL tokens for that grant are immediately revoked.

### Token revocation

Revoke any token via the revocation endpoint (RFC 7009):

```bash
curl -X POST http://localhost:8090/api/auth/revoke \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=ayb_at_..."
```

- Revoking an access token invalidates just that token
- Revoking a refresh token invalidates all tokens for that grant (access + refresh)
- Always returns `200 OK` regardless of whether the token was found (per RFC 7009)

## Scope Model

OAuth scopes align with the existing API key scope model:

| Scope | Permissions |
|-------|-------------|
| `readonly` | GET requests only |
| `readwrite` | GET, POST, PUT, DELETE |
| `*` | Full access |

Optionally restrict access to specific tables using `allowed_tables`:

```
GET /api/auth/authorize?...&scope=readonly&allowed_tables=posts,comments
```

## Client Authentication

The token endpoint supports two methods for client authentication (per RFC 6749 §2.3):

- **HTTP Basic**: `Authorization: Basic base64(client_id:client_secret)`
- **POST body**: `client_id=...&client_secret=...` in the form body

Public clients send only `client_id` (no secret required). You cannot use both methods simultaneously.

## Redirect URI Rules

- HTTPS required (except `http://localhost` and `http://127.0.0.1` for development)
- Exact match only (no wildcards, no query parameters, no fragments)
- At least one URI must be registered
- Port wildcards allowed for localhost (native apps per RFC 8252 §7.3)

## PKCE Requirements

PKCE (Proof Key for Code Exchange) is **required for all clients** — both public and confidential — per OAuth 2.1 (RFC 9700). Only the S256 challenge method is supported; `plain` is rejected.

## Rate Limiting

OAuth clients inherit rate limits from their linked app. When the app's rate limit is exceeded, API requests using that client's tokens return `429 Too Many Requests`.

## Non-Goals (v1)

The following are not supported in the initial release:

- Dynamic client registration (RFC 7591)
- Device authorization grant (RFC 8628)
- OpenID Connect / `id_token`
- DPoP or mTLS sender-constraining
- Token introspection endpoint (RFC 7662)

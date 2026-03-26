<!-- audited 2026-03-21 -->
# Security

This guide documents security features currently shipped in AYB, based on code in `internal/vault`, `internal/auth`, and `internal/server`.

## Encryption at rest

AYB secrets are encrypted at rest through the vault subsystem.

### Vault encryption model

From `internal/vault/vault.go`:

- Encryption algorithm: **AES-256-GCM**.
- Key derivation: **HKDF-SHA256** per secret.
- Per-secret randomness: each encrypted secret gets a random HKDF salt and GCM nonce.
- Stored values: ciphertext + nonce payload (salt + nonce).

### Master key resolution order

`ResolveMasterKey` uses this precedence order:

1. `AYB_VAULT_MASTER_KEY` environment variable.
2. `vault.master_key` config value.
3. Persisted key file at `~/.ayb/vault-key`.
4. If none exist, generate a new random 32-byte key and persist it to `~/.ayb/vault-key`.

### Secret storage behavior

From `internal/vault/store.go`:

- Secrets are persisted encrypted in Postgres (`_ayb_vault_secrets`).
- `CreateSecret` fails on duplicates.
- `SetSecret` is an upsert (create-or-update).
- `UpdateSecret` and `DeleteSecret` return not-found when the secret does not exist.
- `ListSecrets` returns metadata only (name/timestamps), not plaintext values.

## Audit logs

### Platform audit log

- `GET /api/admin/audit`

Supported filters include:

- `table`
- `user_id`
- `operation` (`INSERT`, `UPDATE`, `DELETE`)
- `from`, `to`
- `limit`, `offset`

```bash
curl "http://localhost:8090/api/admin/audit?table=orders&operation=UPDATE&limit=50" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

### Tenant and org audit views

- Tenant audit: `GET /api/admin/tenants/{tenantId}/audit`
- Organization audit: `GET /api/admin/orgs/{orgId}/audit`

## Rate limiting

AYB uses sliding-window in-memory limiters in `internal/auth/ratelimit.go` and `internal/auth/app_ratelimit.go`.

### IP-based limiter

The core limiter sets these headers on responses:

- `X-RateLimit-Limit`
- `X-RateLimit-Remaining`
- `X-RateLimit-Reset`
- `Retry-After` (on 429 only)

A denied request returns HTTP `429`.

### App-based limiter

When an authenticated request has app-scoped claims with configured app RPS limits, AYB applies per-app limits and sets:

- `X-App-RateLimit-Limit`
- `X-App-RateLimit-Remaining`
- `X-App-RateLimit-Reset`
- `Retry-After` (on 429 only)

### Config knobs

Server startup wires these limiters from config (`server_init.go`, `routes_auth.go`):

- `auth.rate_limit` (general auth limiter)
- `auth.anonymous_rate_limit` (anonymous auth traffic)
- `auth.rate_limit_auth` (sensitive auth routes)
- `rate_limit.api` (authenticated API traffic)
- `rate_limit.api_anonymous` (anonymous API traffic)
- `admin.login_rate_limit` (admin login endpoint)

Config value types differ by key:

- `auth.rate_limit` and `admin.login_rate_limit` are integer "requests per minute" values.
- `auth.rate_limit_auth`, `rate_limit.api`, and `rate_limit.api_anonymous` use rate-limit spec strings parsed as `N/min` or `N/hour`.

## API key scopes

API keys are implemented in `internal/auth/apikeys.go` and enforced by middleware plus `ResolveAPIKeyTenantAccess`.

### Key format and lifecycle

- Prefix: all API keys begin with `ayb_` (`APIKeyPrefix`).
- Keys are generated as random bytes and stored hashed.
- Revoked keys are rejected (`ErrAPIKeyRevoked`).
- Expired keys are rejected if `expires_at` is set and in the past (`ErrAPIKeyExpired`).

### Scope values

Allowed scope strings (`internal/auth/auth.go`):

- `*` (full access)
- `readonly`
- `readwrite`

### Three scoping patterns

`CreateAPIKeyOptions` supports three patterns:

- **User-scoped key**: no `appId`, no `orgId` (legacy/default behavior).
- **App-scoped key**: `appId` set; app rate-limit claims may be attached.
- **Org-scoped key**: `orgId` set; access is limited to tenants belonging to that org.

`appId` and `orgId` are mutually exclusive.

### Table restrictions

`allowedTables` restricts table access. Empty means all tables are allowed.

### Org-scope enforcement

For org-scoped keys, AYB checks tenant context at request time (`routes_api.go`, `tenant_middleware.go`):

- If tenant org matches key org, request continues.
- If tenant org does not match, request is rejected with HTTP `403`.

## Secret name validation

From `internal/vault/secret_name.go`:

- Allowed characters: `[A-Za-z0-9_.-]` (letters, digits, underscore, hyphen, dot).
- Names cannot contain `..` (path traversal prevention).
- Names are trimmed of surrounding whitespace before validation.
- Empty names are rejected.

## Transport and response hardening

From `internal/server/middleware.go`:

- CORS is explicitly configured (`Access-Control-Allow-*` headers).
- Security response headers are always set:
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `Referrer-Policy: strict-origin-when-cross-origin`
- AYB does not currently set CSP, HSTS, or other browser hardening headers in this middleware path.

## Secrets management endpoints

Admin secrets endpoints:

- `GET /api/admin/secrets`
- `GET /api/admin/secrets/{name}`
- `POST /api/admin/secrets`
- `PUT /api/admin/secrets/{name}`
- `DELETE /api/admin/secrets/{name}`
- `POST /api/admin/secrets/rotate` (JWT secret rotation)

`POST /api/admin/secrets/rotate` is only registered when the auth service is configured.

```bash
curl -X POST http://localhost:8090/api/admin/secrets \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"SMTP_PASSWORD","value":"<secret>"}'
```

## Related guides

- [Organizations](/guide/organizations)
- [Admin Dashboard](/guide/admin-dashboard)
- [Authentication](/guide/authentication)
- [SAML SSO](/guide/saml)
- [Custom Domains](/guide/custom-domains)

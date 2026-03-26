<!-- audited 2026-03-20 -->
# Custom Domains

This guide covers AYB's current custom-domain lifecycle: admin API setup, DNS ownership verification, certificate provisioning/renewal, route-table behavior, and status transitions.

Source of truth:

- `internal/server/custom_domains.go`
- `internal/server/custom_domains_handler.go`
- `internal/server/custom_domains_store.go`
- `internal/server/custom_domains_verify.go`
- `internal/server/custom_domains_cert.go`
- `internal/server/custom_domains_health.go`
- `internal/server/custom_domains_route.go`
- Tests: `custom_domains_handler_test.go`, `custom_domains_verify_test.go`, `custom_domains_cert_test.go`, `custom_domains_health_test.go`, `custom_domains_route_test.go`

Persistence is in `_ayb_custom_domains`.

## Lifecycle prerequisites

- Enable the jobs service/scheduler so AYB can register domain verification, route sync, re-verification, and tombstone-cleanup jobs.
- Certificate provisioning, renewal, and health-check jobs also require a wired `CertManager`.
- Without those background services, the admin API can store domain bindings, but the automated verification/certificate/routing lifecycle described below does not run.

## Admin API

Admin-token routes:

- `GET /api/admin/domains`
- `POST /api/admin/domains`
- `GET /api/admin/domains/{id}`
- `DELETE /api/admin/domains/{id}`
- `POST /api/admin/domains/{id}/verify`

### Create domain

Request fields:

- `hostname` (required)
- `environment` (optional; defaults to `production`)
- `redirectMode` (optional; allowed: `permanent`, `temporary`)

Example:

```bash
curl -X POST http://localhost:8090/api/admin/domains \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "app.example.com",
    "environment": "production",
    "redirectMode": "permanent"
  }'
```

Hostname validation enforced by `normalizeAndValidateHostname`:

- lowercases and trims input
- rejects wildcards (`*`)
- rejects IPv4/IPv6 literals
- rejects hostnames with ports
- requires at least two DNS labels
- max total length 253, max label length 63
- labels cannot start or end with `-`
- ASCII letters/digits/hyphens only

## DNS verification

On create, AYB generates a 64-char hex verification token and returns a computed challenge record:

- `_ayb-challenge.<hostname> TXT <token>`

Example:

```text
_ayb-challenge.app.example.com TXT aabbccdd...
```

Verification worker behavior (`domain_dns_verify`):

- polls TXT records at `_ayb-challenge.<hostname>`
- success transitions to `verified`
- retries while status is `pending_verification`
- timeout window is 24 hours from first attempt

Retry cadence (`verifyRetryDelay`):

- attempts 1-20: 30-32 seconds
- attempts 21+: 5m to 5m10s

After timeout, AYB sets `verification_failed` with `lastError`.

### Manual re-trigger

`POST /api/admin/domains/{id}/verify`:

- enqueues verification when status is `pending_verification` or `verification_failed`
- returns current binding unchanged for `verified`, `active`, and other non-verifiable states
- rate limited per domain: 10 attempts/hour

Rate-limit headers on verify endpoint:

- `X-RateLimit-Limit`
- `X-RateLimit-Remaining`
- `X-RateLimit-Reset`
- `Retry-After` (on `429`)

## Certificate lifecycle

Certificate jobs use certmagic-backed `CertManager`.

- `verified` + no error enqueues `domain_cert_provision`
- provision success calls `SetDomainCert` and transitions to `active`
- active domain stores:
  - `certRef` (cert serial hex)
  - `certExpiry`
- provision failure keeps status `verified` and updates `lastError`

Renewal flow (`domain_cert_renew`):

- scans active domains with `cert_expiry < now + 30 days`
- syncs refreshed cert metadata from cert cache
- warns when cert is still within 7 days of expiry

## Health and re-verification jobs

Health check (`domain_health_check`):

- runs on active domains with certs
- sets `healthStatus` to `healthy` or `unhealthy`
- also warns when cert expiry is under 7 days

DNS re-verification (`domain_reverify`):

- runs daily on active domains
- increments `reverifyFailures` when challenge no longer matches
- resets failures when DNS is valid again
- after 3 consecutive failures, transitions to `verification_lapsed`

## Routing and status transitions

Route table includes only:

- `active`
- `verification_lapsed`
- `tombstoned`

Middleware behavior:

- unknown host: request passes through
- `tombstoned` host: `421 Misdirected Request`
- `active` and `verification_lapsed`: route entry added to request context

Status values (`DomainStatus`):

- `pending_verification`
- `verified`
- `active`
- `verification_failed`
- `verification_lapsed`
- `tombstoned`

Cleanup windows from store queries/jobs:

- `verification_lapsed` cleanup grace: 7 days (then tombstoned via reverify cleanup path)
- tombstone reaping hard delete: 7 days after `tombstoned_at`

## Schedules

Registered cron schedules (UTC):

- route sync: every 5 minutes (`*/5 * * * *`)
- health check: every 15 minutes (`*/15 * * * *`)
- cert renew: every 12 hours (`0 */12 * * *`)
- DNS reverify: daily at 04:00 (`0 4 * * *`)
- tombstone reap: daily at 03:00 (`0 3 * * *`)

## Current redirect-mode limitation

`redirectMode` is validated and stored (`permanent`/`temporary`) and carried in route entries, but there is no built-in HTTP redirect response path in `hostRouteMiddleware` yet. Treat it as routing metadata for now.

## Related guides

- [Admin Dashboard](/guide/admin-dashboard#admin)
- [Security](/guide/security)
- [Deployment](/guide/deployment)

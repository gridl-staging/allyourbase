# Push Notifications
<!-- audited 2026-03-21 -->

AYB supports push notifications via Firebase Cloud Messaging (FCM) and Apple Push Notification service (APNS). The push system handles device token lifecycle, async delivery with retries via the job queue, and automatic stale token cleanup.

## Enable push notifications

Push requires the job queue to be enabled. Enable both in your config:

```toml
[jobs]
enabled = true

[push]
enabled = true
```

AYB validates push configuration before startup. In addition, runtime provider construction uses log-provider fallback if a configured provider cannot be initialized, so push wiring can stay up while provider errors are surfaced in logs.

## Provider setup

### Firebase Cloud Messaging (FCM)

1. Create a Firebase project and download the service account JSON credentials
2. Configure the credentials file path:

```toml
[push.fcm]
credentials_file = "/path/to/firebase-service-account.json"
```

Or via environment variable:

```bash
AYB_PUSH_FCM_CREDENTIALS_FILE=/path/to/firebase-service-account.json
```

The credentials file must contain valid JSON. AYB extracts the `project_id` from the service account JSON and uses it to construct FCM API requests.

**Auth model:** AYB generates an OAuth2 access token from the service account credentials (RS256-signed JWT grant to `https://oauth2.googleapis.com/token`). The access token is cached in memory and refreshed automatically when within 5 minutes of expiry (~1 hour lifetime).

### Apple Push Notification service (APNS)

1. Generate an APNS authentication key (.p8 file) in your Apple Developer account
2. Note the Key ID, Team ID, and app Bundle ID
3. Configure:

```toml
[push.apns]
key_file = "/path/to/AuthKey_XXXXXXXXXX.p8"
team_id = "ABCDE12345"
key_id = "XXXXXXXXXX"
bundle_id = "com.example.myapp"
environment = "production"   # "production" or "sandbox"
```

Or via environment variables:

```bash
AYB_PUSH_APNS_KEY_FILE=/path/to/AuthKey.p8
AYB_PUSH_APNS_TEAM_ID=ABCDE12345
AYB_PUSH_APNS_KEY_ID=XXXXXXXXXX
AYB_PUSH_APNS_BUNDLE_ID=com.example.myapp
AYB_PUSH_APNS_ENVIRONMENT=production
```

**Auth model:** AYB generates an ES256-signed JWT with the Team ID as issuer and Key ID in the header. The JWT is cached and refreshed when within 10 minutes of expiry (APNS rejects tokens older than 1 hour).

**Environment:** Use `sandbox` for development builds (`api.sandbox.push.apple.com`) and `production` for release builds (`api.push.apple.com`).

## Device token lifecycle

### Registration

Client apps register device tokens via the user-facing API. The user's identity is extracted from JWT claims.

```bash
curl -X POST http://localhost:8090/api/push/devices \
  -H "Authorization: Bearer $USER_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "app_id": "00000000-0000-0000-0000-000000000001",
    "provider": "fcm",
    "platform": "android",
    "token": "dGVzdC10b2tlbi0xMjM0NTY3ODkw...",
    "device_name": "Pixel 8 Pro"
  }'
```

Registration uses upsert semantics: if the same `(app_id, provider, token)` combination already exists, the existing row is updated (user_id, device_name, is_active=true, last_refreshed_at=now).

### Token refresh

Firebase recommends refreshing tokens monthly. When the client re-registers an existing token, `last_refreshed_at` is updated. This timestamp drives stale token cleanup.

### Listing own tokens

```bash
curl "http://localhost:8090/api/push/devices?app_id=00000000-0000-0000-0000-000000000001" \
  -H "Authorization: Bearer $USER_JWT"
```

Returns only the authenticated user's active tokens for the specified app.

### Revoking a token

```bash
curl -X DELETE http://localhost:8090/api/push/devices/DEVICE_TOKEN_ID \
  -H "Authorization: Bearer $USER_JWT"
```

Ownership validation ensures users can only revoke their own tokens. Sets `is_active=false`.

### Automatic stale token cleanup

A `push_token_cleanup` scheduled job runs daily (`push_token_cleanup_daily`) when both `jobs.enabled` and `push.enabled` are true. It marks tokens inactive if `last_refreshed_at` is older than 270 days, aligned with Firebase's token invalidation policy for inactive devices.

## Sending push notifications

### Via admin API

Send to all active devices for a user:

```bash
curl -X POST http://localhost:8090/api/admin/push/send \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "app_id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000002",
    "title": "New comment on your post",
    "body": "Maya replied to your workout log",
    "data": {"post_id": "abc123", "type": "comment"}
  }'
```

This fans out to all active tokens for the user, creating one delivery record and one job per token.

Send to a specific token (for testing):

```bash
curl -X POST http://localhost:8090/api/admin/push/send-to-token \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "token_id": "TOKEN_UUID",
    "title": "Test notification",
    "body": "Hello from AYB",
    "data": {"key": "value"}
  }'
```

### Via CLI

```bash
ayb push send \
  --app-id 00000000-0000-0000-0000-000000000001 \
  --user-id 00000000-0000-0000-0000-000000000002 \
  --title "Challenge update" \
  --body "You're in 3rd place!" \
  --data '{"challenge_id":"xyz"}'
```

### Via Go service (programmatic)

```go
deliveries, err := pushService.SendToUser(ctx, appID, userID, "Title", "Body", map[string]string{
    "post_id": "abc123",
})
```

## Delivery pipeline

1. **Enqueue**: `SendToUser` creates a delivery record (status=`pending`) and enqueues a `push_delivery` job per active token
2. **Process**: The job handler calls `ProcessDelivery`, which loads the delivery, looks up the provider, and calls `provider.Send()`
3. **Success**: Delivery status → `sent`, provider message ID recorded, token `last_used` updated
4. **Permanent failure**: Invalid/unregistered token → delivery status → `invalid_token`, token marked inactive, no retry
5. **Transient failure**: Delivery status → `failed`, job returns error for retry with exponential backoff (max 3 attempts via job queue)
6. **Auth error**: Provider JWT/token refresh failure → job returns error, provider refreshes cached auth token on next attempt

## Delivery audit trail

Every push send attempt creates a delivery record in `_ayb_push_deliveries` with:

- Title, body, data payload
- Status (`pending`, `sent`, `failed`, `invalid_token`)
- Provider error code and message (on failure)
- Provider message ID (on success)
- Link to job queue row (`job_id`) for retry status visibility

### Querying deliveries

Admin API:

```bash
curl "http://localhost:8090/api/admin/push/deliveries?app_id=UUID&status=failed&limit=20" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

CLI:

```bash
ayb push list-deliveries --status failed --limit 20
ayb push list-deliveries --user-id UUID --json
```

## Payload size limits

- FCM limits data payload to 4KB
- APNS limits the entire notification payload to 4KB
- AYB validates payload size (title + body + data) before sending and rejects oversized payloads with a `400` error
- The database allows up to 8KB for `data_payload` to accommodate JSON overhead, but the service layer enforces the 4KB provider limit

## Error taxonomy

### FCM errors

| Error | HTTP | Classification | Action |
|---|---|---|---|
| INVALID_ARGUMENT | 400 | Permanent | No retry |
| UNREGISTERED | 404 | Permanent | Mark token inactive |
| SENDER_ID_MISMATCH | 403 | Permanent | No retry |
| THIRD_PARTY_AUTH_ERROR | 401 | Permanent | No retry |
| QUOTA_EXCEEDED | 429 | Transient | Retry with backoff |
| UNAVAILABLE | 503 | Transient | Retry with backoff |
| INTERNAL | 500 | Transient | Retry with backoff |

### APNS errors

| Reason | HTTP | Classification | Action |
|---|---|---|---|
| BadDeviceToken | 400 | Permanent | Mark token inactive |
| Unregistered | 410 | Permanent | Mark token inactive |
| ExpiredToken | 410 | Permanent | Mark token inactive |
| DeviceTokenNotForTopic | 400 | Permanent | Mark token inactive |
| ExpiredProviderToken | 403 | Auth error | Refresh JWT, retry |
| InvalidProviderToken | 403 | Auth error | Refresh JWT, retry |
| MissingProviderToken | 403 | Auth error | Refresh JWT, retry |
| TooManyRequests | 429 | Transient | Retry with backoff |
| InternalServerError | 500 | Transient | Retry with backoff |
| ServiceUnavailable | 503 | Transient | Retry with backoff |
| Shutdown | 503 | Transient | Retry with backoff |

APNS `410 Gone` responses are treated as `ErrUnregistered` even when the response body is empty or unparseable.

## Admin API

All endpoints require admin auth (`Authorization: Bearer <admin-token>`).

```
GET    /api/admin/push/devices              List device tokens
POST   /api/admin/push/devices              Register device token (for testing)
DELETE /api/admin/push/devices/{id}         Revoke device token
POST   /api/admin/push/send                 Send to user (fans out to all active tokens)
POST   /api/admin/push/send-to-token        Send to specific token
GET    /api/admin/push/deliveries            List deliveries
GET    /api/admin/push/deliveries/{id}       Get delivery details
```

## User-facing API

All user endpoints require JWT authentication. The user's identity is extracted from JWT claims.

```
POST   /api/push/devices                    Register device token
GET    /api/push/devices?app_id=UUID        List own active tokens
DELETE /api/push/devices/{id}               Revoke own device token
```

## CLI

```bash
ayb push list-devices [--app-id UUID] [--user-id UUID] [--include-inactive] [--json|--csv]
ayb push register-device --app-id UUID --user-id UUID --provider fcm|apns --platform ios|android --token TOKEN [--device-name NAME]
ayb push revoke-device DEVICE_ID
ayb push send --app-id UUID --user-id UUID --title "..." --body "..." [--data '{"key":"val"}'] [--json]
ayb push list-deliveries [--app-id UUID] [--user-id UUID] [--status pending|sent|failed|invalid_token] [--json|--csv]
```

## Admin dashboard

In the Admin Dashboard, open `Messaging -> Push Notifications` to:

- Browse device tokens with provider/platform badges, active status, and last refreshed/used timestamps
- Register test device tokens
- Revoke device tokens
- Filter devices by app ID, user ID, and active status
- Send test push notifications to a user or specific token
- Browse delivery history with status filtering
- Expand delivery rows to see full body, data payload, error details, and linked job retry info

## Configuration reference

```toml
[push]
enabled = false                # Enable push notifications (requires jobs.enabled)

[push.fcm]
credentials_file = ""         # Path to Firebase service account JSON

[push.apns]
key_file = ""                 # Path to .p8 private key file
team_id = ""                  # Apple Developer Team ID
key_id = ""                   # APNS authentication key ID
bundle_id = ""                # App bundle identifier
environment = "production"    # "production" or "sandbox"
```

Environment variables:

| Variable | Config equivalent |
|---|---|
| `AYB_PUSH_ENABLED` | `push.enabled` |
| `AYB_PUSH_FCM_CREDENTIALS_FILE` | `push.fcm.credentials_file` |
| `AYB_PUSH_APNS_KEY_FILE` | `push.apns.key_file` |
| `AYB_PUSH_APNS_TEAM_ID` | `push.apns.team_id` |
| `AYB_PUSH_APNS_KEY_ID` | `push.apns.key_id` |
| `AYB_PUSH_APNS_BUNDLE_ID` | `push.apns.bundle_id` |
| `AYB_PUSH_APNS_ENVIRONMENT` | `push.apns.environment` |

Validation rules:

- `push.enabled` requires `jobs.enabled` (delivery uses the job queue)
- Config validation requires at least one provider (FCM or APNS) to be fully configured
- `push.fcm.credentials_file` must exist and contain valid JSON
- APNS requires all four fields (`key_file`, `team_id`, `key_id`, `bundle_id`) when `key_file` is set
- `push.apns.environment` must be `"production"` or `"sandbox"`
- During service wiring, provider initialization failures fall back to log providers (`buildPushProviders`) instead of disabling the push service

## Compatibility note

When `push.enabled = false`, push endpoints return `503 Service Unavailable`, no push workers start, and the push cleanup schedule is not registered.

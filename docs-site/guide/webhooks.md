# Webhooks

<!-- audited 2026-03-20 -->

AYB exposes two different webhook surfaces:

1. Outbound webhook management under `/api/webhooks` (admin-only).
2. Inbound callback endpoints under `/api/webhooks/*` (provider callbacks into AYB).

## Outbound webhook management (`/api/webhooks`)

The outbound management API is mounted in the JSON API under `/api/webhooks` and requires an admin token.

Routes:

- `GET /api/webhooks`
- `POST /api/webhooks`
- `GET /api/webhooks/{id}`
- `PATCH /api/webhooks/{id}`
- `DELETE /api/webhooks/{id}`
- `POST /api/webhooks/{id}/test`
- `GET /api/webhooks/{id}/deliveries`
- `GET /api/webhooks/{id}/deliveries/{deliveryId}`

`GET /api/webhooks` returns an object with an `items` array:

```json
{
  "items": [
    {
      "id": "...",
      "url": "https://example.com/hook",
      "hasSecret": true,
      "events": ["create", "update", "delete"],
      "tables": ["posts"],
      "enabled": true,
      "createdAt": "2026-03-20T12:00:00Z",
      "updatedAt": "2026-03-20T12:00:00Z"
    }
  ]
}
```

## Create or update outbound webhooks

Create:

```bash
curl -X POST http://localhost:8090/api/webhooks \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/ayb/webhooks",
    "secret": "whsec_abc123",
    "events": ["create", "update", "delete"],
    "tables": ["posts", "comments"],
    "enabled": true
  }'
```

Create defaults:

- `enabled` defaults to `true` when omitted.
- `events` defaults to `["create", "update", "delete"]` when omitted.
- `tables` defaults to `[]` (all tables) when omitted.

Update is partial (`PATCH /api/webhooks/{id}`): omitted fields are left unchanged.

## Outbound payloads and signatures

Outbound deliveries use realtime-style event payloads:

```json
{
  "action": "update",
  "table": "posts",
  "record": { "id": 42, "title": "Updated" },
  "oldRecord": { "id": 42, "title": "Old" }
}
```

If `secret` is configured, AYB signs the raw request body with HMAC-SHA256 and sends:

- Header: `X-AYB-Signature`
- Value: lowercase hex digest

Node.js verification example:

```ts
import crypto from "node:crypto";

export function verifyAYBSignature(rawBody: Buffer, signature: string, secret: string): boolean {
  const expected = crypto.createHmac("sha256", secret).update(rawBody).digest("hex");
  const a = Buffer.from(signature, "utf8");
  const b = Buffer.from(expected, "utf8");
  return a.length === b.length && crypto.timingSafeEqual(a, b);
}
```

Always verify against the exact raw request bytes.

## Delivery retries and logs

Dispatcher behavior:

- Timeout per attempt: 10 seconds
- Maximum attempts: 4 (1 initial attempt + 3 retries)
- Retry backoff schedule: 1s, 5s, 25s
- Retries occur on non-2xx responses and network errors

Delivery log listing (`GET /api/webhooks/{id}/deliveries`) supports `page` and `perPage`:

- `page` default: `1`
- `perPage` default: `20`
- `perPage` maximum: `100`

Response shape:

```json
{
  "items": [],
  "page": 1,
  "perPage": 20,
  "totalItems": 0,
  "totalPages": 0
}
```

You can also fetch one delivery with `GET /api/webhooks/{id}/deliveries/{deliveryId}`.

## Test an outbound webhook

`POST /api/webhooks/{id}/test` sends a synthetic event and returns delivery status and duration.

```bash
curl -X POST http://localhost:8090/api/webhooks/<id>/test \
  -H "Authorization: Bearer <admin-token>"
```

Current behavior:

- The request is sent even if the webhook is disabled.
- AYB makes a single attempt with a 10-second timeout and does not retry test deliveries.
- Test deliveries are not persisted in `GET /api/webhooks/{id}/deliveries` history.

## Inbound callback routes (`/api/webhooks/*`)

These routes are separate from outbound management:

- `POST /api/webhooks/sms/status`
- `POST /api/webhooks/support/email` (when support is enabled)
- `POST /api/webhooks/stripe` (when Stripe billing webhook config is enabled)

Use these only for providers calling into AYB.

## Related guides

- [Realtime](/guide/realtime)
- [Edge Functions](/guide/edge-functions)
- [Security](/guide/security)

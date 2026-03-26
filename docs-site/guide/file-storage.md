# File Storage

<!-- audited 2026-03-20 -->

AYB storage supports bucket administration, object CRUD, signed URLs, resumable uploads (TUS), and optional CDN URL rewrite/purge.

## Enable storage

```toml
[storage]
enabled = true
backend = "local"           # "local" or "s3"
local_path = "./ayb_storage"
max_file_size = "10MB"
```

## Route surfaces and auth

### Bucket admin routes (admin token required)

- `POST /api/storage/buckets`
- `GET /api/storage/buckets`
- `PUT /api/storage/buckets/{name}`
- `DELETE /api/storage/buckets/{name}`

Notes:

- Bucket CRUD is admin-only.
- Deleting a non-empty bucket returns a conflict unless `force=true` is passed.

### Object routes

- `GET /api/storage/{bucket}`
- `GET /api/storage/{bucket}/{name}`
- `POST /api/storage/{bucket}`
- `DELETE /api/storage/{bucket}/{name}`
- `POST /api/storage/{bucket}/{name}/sign`

When auth is enabled:

- Read routes (`GET`) use optional auth.
- Write routes (`POST` upload, `DELETE`, `POST .../sign`) require admin-or-user auth.

When auth is disabled, routes are mounted directly without auth middleware.

## Bucket visibility and signed URL behavior

- Buckets with metadata `public=true` expose public object URLs.
- Buckets with no metadata record are treated as implicitly public.
- Signed URL validation on `GET /api/storage/{bucket}/{name}` bypasses auth if signature is valid.

Signed URL creation:

- Endpoint: `POST /api/storage/{bucket}/{name}/sign`
- Request: `{ "expiresIn": <seconds> }`
- Default expiry: `3600` seconds
- Maximum expiry: `604800` seconds (7 days)
- Response shape:

```json
{
  "url": "/api/storage/<bucket>/<name>?exp=...&sig=..."
}
```

## Upload and multipart behavior

Upload endpoint: `POST /api/storage/{bucket}`

Rules:

- Multipart form field `file` is required.
- Optional form field `name` overrides filename.
- Request body is capped by `storage.max_file_size`.
- Content type is inferred from filename extension, then multipart header, else `application/octet-stream`.

## Resumable uploads (TUS)

Routes:

- `OPTIONS /api/storage/upload/resumable`
- `POST /api/storage/upload/resumable?bucket={bucket}&name={name}`
- `HEAD /api/storage/upload/resumable/{id}`
- `PATCH /api/storage/upload/resumable/{id}`

Behavior:

- `OPTIONS` is intentionally unauthenticated for browser preflight.
- `POST`, `HEAD`, and `PATCH` require admin-or-user auth when auth is enabled.
- Required TUS version header: `Tus-Resumable: 1.0.0` (create/head/patch).
- `POST` requires `Upload-Length` and bucket/name (query or metadata fallback for name).
- `PATCH` requires `Content-Type: application/offset+octet-stream` and `Upload-Offset`.
- `HEAD` returns current `Upload-Offset` and total `Upload-Length`.

## Object storage backends

Local:

```toml
[storage]
enabled = true
backend = "local"
local_path = "/var/lib/ayb/storage"
```

S3-compatible:

```toml
[storage]
enabled = true
backend = "s3"
s3_endpoint = "https://ACCOUNT_ID.r2.cloudflarestorage.com"
s3_bucket = "my-bucket"
s3_region = "auto"
s3_access_key = "..."
s3_secret_key = "..."
```

## CDN rewrite and purge providers

URL rewriting uses `storage.cdn_url`.

```toml
[storage]
cdn_url = "https://cdn.example.com"

[storage.cdn]
provider = "cloudflare" # or "cloudfront" or "webhook"
```

Supported provider keys:

- `storage.cdn.cloudflare.zone_id`
- `storage.cdn.cloudflare.api_token`
- `storage.cdn.cloudfront.distribution_id`
- `storage.cdn.webhook.endpoint`
- `storage.cdn.webhook.signing_secret`

Runtime validation requires `storage.cdn_url` when any CDN provider is configured.

## JavaScript SDK examples

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");
await ayb.auth.login("user@example.com", "password");

const file = document.querySelector("input[type=file]")!.files![0];
const uploaded = await ayb.storage.upload("avatars", file);

const publicOrOriginURL = ayb.storage.downloadURL("avatars", uploaded.name);
const { items, totalItems } = await ayb.storage.list("avatars", { limit: 50, offset: 0 });
const { url: signedURL } = await ayb.storage.getSignedURL("avatars", uploaded.name, 3600);
await ayb.storage.delete("avatars", uploaded.name);
```

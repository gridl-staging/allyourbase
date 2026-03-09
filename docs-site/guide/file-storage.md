# File Storage

AYB provides file upload, download, and deletion with local filesystem or S3-compatible object storage backends.

## Enable storage

```toml
# ayb.toml
[storage]
enabled = true
backend = "local"           # "local" or "s3" (any S3-compatible object store)
local_path = "./ayb_storage"
max_file_size = "10MB"
```

## Endpoints

```
GET    /api/storage/{bucket}                List files in a bucket
POST   /api/storage/{bucket}                Upload a file to a bucket
GET    /api/storage/{bucket}/{name}         Download a file
DELETE /api/storage/{bucket}/{name}         Delete a file
POST   /api/storage/{bucket}/{name}/sign    Get a signed URL
```

### Upload

```bash
curl -X POST http://localhost:8090/api/storage/avatars \
  -H "Authorization: Bearer eyJhbG..." \
  -F "file=@photo.jpg"
```

The bucket name is in the URL path. The file is sent as multipart form data.

**Response** (201 Created):

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "bucket": "avatars",
  "name": "photo.jpg",
  "size": 245678,
  "contentType": "image/jpeg",
  "createdAt": "2026-02-07T22:00:00Z"
}
```

### List files in a bucket

```bash
curl http://localhost:8090/api/storage/avatars
```

### Download

```bash
curl http://localhost:8090/api/storage/avatars/photo.jpg
```

Returns the file with the correct `Content-Type` header.

### Delete

```bash
curl -X DELETE http://localhost:8090/api/storage/avatars/photo.jpg \
  -H "Authorization: Bearer eyJhbG..."
```

Returns `204 No Content` on success.

## Local storage

Files are stored on the filesystem at the path specified in `local_path` (default: `./ayb_storage`).

```toml
[storage]
enabled = true
backend = "local"
local_path = "/var/lib/ayb/storage"
max_file_size = "50MB"
```

## S3-compatible object storage

Works with any S3-compatible service — Cloudflare R2, MinIO, DigitalOcean Spaces, AWS S3, Backblaze B2, and more. The `"s3"` backend uses the standard S3 protocol, so you're never locked into a single provider.

### Cloudflare R2

Zero egress fees. Set `s3_region = "auto"`.

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

### MinIO (self-hosted)

Run your own S3-compatible object store. Great for air-gapped or on-prem deployments.

```toml
[storage]
enabled = true
backend = "s3"
s3_endpoint = "localhost:9000"
s3_bucket = "ayb"
s3_region = "us-east-1"
s3_access_key = "minioadmin"
s3_secret_key = "minioadmin"
s3_use_ssl = false
```

### AWS S3

```toml
[storage]
enabled = true
backend = "s3"
s3_endpoint = "s3.amazonaws.com"
s3_bucket = "my-ayb-bucket"
s3_region = "us-east-1"
s3_access_key = "AKIAIOSFODNN7EXAMPLE"
s3_secret_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
s3_use_ssl = true
max_file_size = "100MB"
```

## JavaScript SDK

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");
await ayb.auth.login("user@example.com", "password");

// Upload a file to a bucket
const file = document.querySelector("input[type=file]").files[0];
const result = await ayb.storage.upload("avatars", file);

// Get download URL (bucket + name)
const url = ayb.storage.downloadURL("avatars", result.name);

// List files in a bucket
const { items } = await ayb.storage.list("avatars");

// Get a signed URL for time-limited access
const { url: signedUrl } = await ayb.storage.getSignedURL("avatars", "photo.jpg", 3600);

// Delete (bucket + name)
await ayb.storage.delete("avatars", result.name);
```

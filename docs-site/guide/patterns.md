<!-- audited 2026-03-20 -->

# Patterns

Use this page as a compact recipe collection. Every snippet here is aligned to current code/tests/OpenAPI, and each recipe links to the canonical guide for full behavior and edge cases.

## 1. Row-Level Security with user context

### Problem

Enforce per-user row access in SQL policies without passing user IDs in every query.

### Snippet

```sql
CREATE POLICY user_owns_posts ON posts
FOR SELECT
USING (author_id::text = current_setting('ayb.user_id', true));

-- Use this variant only for tenant-aware tokens where ayb.tenant_id is present.
CREATE POLICY tenant_user_owns_posts ON posts
FOR SELECT
USING (
  author_id::text = current_setting('ayb.user_id', true)
  AND tenant_id::text = current_setting('ayb.tenant_id', true)
);
```

### Why this works

Authenticated requests set `ayb.user_id` and `ayb.user_email` for the transaction, and set `ayb.tenant_id` only when the JWT claim is non-empty.

### Canonical guide

[Authentication](/guide/authentication)

## 2. Multi-tenant data isolation

### Problem

Resolve tenant context consistently when JWT claims, route params, and headers might all be present.

### Snippet

```text
Tenant context in request middleware (resolveTenantContext + tenantIDFromRequest):
1) JWT claim TenantID (if non-empty)
2) URL param tenantId
3) X-Tenant-ID header (only when auth claims already exist)
4) If tenant is still empty and request uses an admin token, X-Tenant-ID fallback is allowed

Related helper behavior (tenantIDFromContextOrRequest):
- When tenant context is still empty, unauthenticated /api/* paths may read X-Tenant-ID.
- Non-API unauthenticated paths intentionally ignore X-Tenant-ID.
```

### Why this works

The server keeps claim-first precedence for request middleware, then limits weaker header fallbacks to specific authenticated/admin paths; the `/api/*` unauthenticated header fallback is scoped to a separate helper path.

### Canonical guide

[Organizations](/guide/organizations)

## 3. Realtime table subscription with clean unsubscribe

### Problem

Subscribe to table updates with SDK defaults that match SSE query semantics.

### Snippet

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");
await ayb.auth.login("user@example.com", "password");

const unsubscribe = ayb.realtime.subscribe(["posts", "comments"], (event) => {
  console.log(event.action, event.table, event.record);
  if (event.oldRecord) {
    console.log("before update:", event.oldRecord);
  }
});

// Later:
unsubscribe();
```

### Why this works

`subscribe()` sends `tables=...` in the URL, appends `token=...` when the client has a token, normalizes payloads, and returns a closure that closes the EventSource stream.

### Canonical guide

[Realtime](/guide/realtime)

## 4. Storage upload with signed and direct download URLs

### Problem

Upload a file, then share it either with a time-limited URL or with the canonical download route.

### Snippet

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

const fileInput = document.querySelector<HTMLInputElement>("#avatar");
const file = fileInput?.files?.[0];
if (!file) throw new Error("file required");

const uploaded = await ayb.storage.upload("avatars", file, "profile.png");

const { url: signedURL } = await ayb.storage.getSignedURL(
  uploaded.bucket,
  uploaded.name,
  3600,
);
const directURL = ayb.storage.downloadURL(uploaded.bucket, uploaded.name);
```

### Why this works

The SDK maps directly to `/api/storage/{bucket}` upload, `/api/storage/{bucket}/{name}/sign` signed URL generation, and `/api/storage/{bucket}/{name}` direct download.

### Canonical guide

[File Storage](/guide/file-storage)

## 5. Edge function database query chain

### Problem

Query Postgres inside an edge function using the runtime-provided `ayb` namespace.

### Snippet

```js
function handler(request) {
  var rows = ayb.db.from("users").select("id, name").eq("id", 1).execute();
  return {
    statusCode: 200,
    body: JSON.stringify(rows),
  };
}
```

### Why this works

The edge runtime registers `ayb.db.from(...)` and supports chained filters ending in `.execute()`.

### Canonical guide

[Edge Functions](/guide/edge-functions)

## 6. RPC function calls from the SDK

### Problem

Call PostgreSQL functions with named args (or no args) without hand-rolling request paths.

### Snippet

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

const total = await ayb.rpc<number>("get_total", { user_id: "abc" });
await ayb.rpc("cleanup_old_data", { days: 30 });
await ayb.rpc("no_args_fn");
```

### Why this works

`rpc()` sends `POST /api/rpc/{function}` and only includes a JSON body when args are provided.

### Canonical guide

[Database RPC](/guide/database-rpc)

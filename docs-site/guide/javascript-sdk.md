<!-- audited 2026-03-20 -->

# JavaScript SDK

The `@allyourbase/js` package provides a typed client for the AYB REST API with support for records, auth, OAuth, storage, realtime subscriptions, and RPC calls.

## Install

```bash
npm install @allyourbase/js
```

## Quick start

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

const { items } = await ayb.records.list("posts", {
  filter: "published=true",
  sort: "-created_at",
});
```

## Client

```ts
const ayb = new AYBClient("http://localhost:8090");

// With custom fetch (e.g. undici, node-fetch)
const ayb = new AYBClient("http://localhost:8090", { fetch: customFetch });
```

## Records

### List

```ts
const result = await ayb.records.list<Post>("posts", {
  search: "postgres",
  filter: "status='active'",
  sort: "-created_at,+title",
  page: 1,
  perPage: 50,
  fields: "id,title,status",
  expand: "author",
  skipTotal: true,
});
// result.items: Post[]
// result.page, result.perPage, result.totalItems, result.totalPages
```

### Get

```ts
const post = await ayb.records.get<Post>("posts", "42", {
  expand: "author",
});
```

### Create

```ts
const post = await ayb.records.create<Post>("posts", {
  title: "New Post",
  body: "Content",
});
```

### Update

```ts
const updated = await ayb.records.update<Post>("posts", "42", {
  title: "Updated",
});
```

### Delete

```ts
await ayb.records.delete("posts", "42");
```

### Batch

Perform multiple operations in a single atomic transaction:

```ts
const results = await ayb.records.batch<Post>("posts", [
  { method: "create", body: { title: "Post A", published: true } },
  { method: "create", body: { title: "Post B", published: false } },
  { method: "update", id: "42", body: { published: true } },
  { method: "delete", id: "99" },
]);
// results: Array<{ index: number, status: number, body?: Post }>
```

All operations succeed or fail together. Max 1000 operations per request.

## Auth

```ts
// Register
const { token, user } = await ayb.auth.register("user@example.com", "pass");

// Login (stores tokens automatically)
await ayb.auth.login("user@example.com", "pass");

// Authenticated requests work automatically after login
const me = await ayb.auth.me();

// Refresh
await ayb.auth.refresh();

// Logout
await ayb.auth.logout();

// Password reset
await ayb.auth.requestPasswordReset("user@example.com");
await ayb.auth.confirmPasswordReset(token, "newpass");

// Email verification
await ayb.auth.verifyEmail(token);
await ayb.auth.resendVerification();

// Delete current user's account
await ayb.auth.deleteAccount();
```

### OAuth

Sign in with an OAuth provider via popup (default) or redirect flow:

```ts
// Popup flow (default)
const { token, user } = await ayb.auth.signInWithOAuth("google");

// With extra scopes
await ayb.auth.signInWithOAuth("github", { scopes: ["repo"] });

// Redirect flow (for PWAs or when popups are blocked)
await ayb.auth.signInWithOAuth("google", {
  urlCallback: (url) => { window.location.href = url; },
});
```

After a redirect-based OAuth flow, parse tokens from the URL hash:

```ts
const result = ayb.auth.handleOAuthRedirect();
if (result) {
  // result.token, result.refreshToken are now set on the client
}
```

Anonymous auth, account-linking, and MFA flows are not currently exposed as
first-class methods on `ayb.auth`. See [Authentication](/guide/authentication)
for the REST endpoint-level flows and payloads.

### Token management

```ts
// Read current tokens
console.log(ayb.token);        // access token or null
console.log(ayb.refreshToken); // refresh token or null

// Restore from localStorage
const saved = localStorage.getItem("ayb_tokens");
if (saved) {
  const { token, refreshToken } = JSON.parse(saved);
  ayb.setTokens(token, refreshToken);
}

// Clear tokens
ayb.clearTokens();

// Use an API key instead of JWT tokens
ayb.setApiKey("ak_...");
ayb.clearApiKey();

// Listen for auth state changes
const unsub = ayb.onAuthStateChange((event, session) => {
  // event: "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED"
  // session: { token, refreshToken } | null
});
unsub(); // stop listening
```

## Storage

```ts
// Upload a file to a bucket
const file = document.querySelector("input[type=file]").files[0];
const result = await ayb.storage.upload("avatars", file);

// Or with a custom filename
await ayb.storage.upload("documents", blob, "report.pdf");

// Download URL (bucket + name)
const url = ayb.storage.downloadURL("avatars", "photo.jpg");
// → "http://localhost:8090/api/storage/avatars/photo.jpg"

// List files in a bucket (supports prefix, limit, offset)
const { items } = await ayb.storage.list("avatars", { prefix: "user_", limit: 20, offset: 0 });

// Get a signed URL (time-limited access)
const { url: signedUrl } = await ayb.storage.getSignedURL("avatars", "photo.jpg", 3600);

// Delete (bucket + name)
await ayb.storage.delete("avatars", "photo.jpg");
```

## RPC

Call PostgreSQL functions via the RPC endpoint:

```ts
// Simple call
const result = await ayb.rpc("my_function", { arg1: "value" });

// Void functions return undefined; scalar functions return unwrapped values

// Trigger a realtime event alongside the RPC call
await ayb.rpc("approve_order", { id: 42 }, {
  notify: { table: "orders", action: "update" },
});
```

## Realtime

```ts
const unsubscribe = ayb.realtime.subscribe(
  ["posts", "comments"],
  (event) => {
    // event.action: "create" | "update" | "delete"
    // event.table: string
    // event.record: Record<string, unknown>
    // event.oldRecord?: Record<string, unknown> (present on updates)
    console.log(event.action, event.table, event.record);
  },
);

// Stop listening
unsubscribe();
```

Auth tokens are sent automatically if the client is authenticated.

## TypeScript

All methods accept generic type parameters:

```ts
interface Post {
  id: number;
  title: string;
  published: boolean;
  created_at: string;
}

const { items } = await ayb.records.list<Post>("posts");
// items: Post[]
```

### Exported types

```ts
import type {
  // Records
  ListResponse,
  ListParams,
  GetParams,
  BatchOperation,
  BatchResult,
  // Auth
  AuthResponse,
  User,
  AuthStateEvent,
  AuthStateListener,
  OAuthProvider,
  OAuthOptions,
  // RPC
  RpcOptions,
  RpcNotifyOption,
  // Realtime
  RealtimeEvent,
  // Storage
  StorageObject,
  // Client
  ClientOptions,
} from "@allyourbase/js";
```

## Error handling

```ts
import { AYBClient, AYBError } from "@allyourbase/js";

try {
  await ayb.records.get("posts", "nonexistent");
} catch (err) {
  if (err instanceof AYBError) {
    console.log(err.status);  // 404
    console.log(err.message); // "record not found"
    console.log(err.code);    // machine-readable error code (optional)
    console.log(err.data);    // field-level validation detail (optional)
    console.log(err.docUrl);  // link to relevant documentation (optional)
  }
}
```

`AYBError` extends `Error` and includes the HTTP `status` code, an optional
machine-readable `code`, optional `data` for field-level detail, and an optional
`docUrl` pointing to relevant documentation.

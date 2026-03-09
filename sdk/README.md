# @allyourbase/js

JavaScript/TypeScript client SDK for [Allyourbase](https://github.com/gridlhq-staging/allyourbase) — the PostgreSQL Backend-as-a-Service.

## Install

```bash
npm install @allyourbase/js
```

## Quick Start

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

// Create a record
const post = await ayb.records.create("posts", {
  title: "Hello World",
  published: true,
});

// List records with filtering and sorting
const posts = await ayb.records.list("posts", {
  filter: "published=true",
  sort: "-created_at",
  perPage: 20,
});

// Auth
await ayb.auth.login("user@example.com", "password");
const me = await ayb.auth.me();
```

## API Reference

### `new AYBClient(baseURL, options?)`

Create a client instance.

```ts
const ayb = new AYBClient("http://localhost:8090");

// With custom fetch (e.g. for Node.js < 18)
const ayb = new AYBClient("http://localhost:8090", { fetch: myFetch });
```

### Records

```ts
// List with filtering, sorting, pagination
const result = await ayb.records.list<Post>("posts", {
  filter: "status='active' AND views>100",
  sort: "-created_at,+title",
  page: 1,
  perPage: 50,
  fields: "id,title,status",
  expand: "author,category",
  skipTotal: true,
});
// result: { items: Post[], page, perPage, totalItems, totalPages }

// Get by ID
const post = await ayb.records.get<Post>("posts", "abc123", {
  expand: "author",
});

// Create
const post = await ayb.records.create<Post>("posts", {
  title: "New Post",
  body: "Content here",
});

// Update (partial)
const updated = await ayb.records.update<Post>("posts", "abc123", {
  title: "Updated Title",
});

// Delete
await ayb.records.delete("posts", "abc123");
```

### Auth

```ts
// Register
const { token, refreshToken, user } = await ayb.auth.register(
  "user@example.com",
  "password123",
);

// Login
await ayb.auth.login("user@example.com", "password123");

// Current user
const me = await ayb.auth.me();

// Refresh token
await ayb.auth.refresh();

// Logout
await ayb.auth.logout();

// Password reset
await ayb.auth.requestPasswordReset("user@example.com");
await ayb.auth.confirmPasswordReset(token, "newpassword");

// Email verification
await ayb.auth.verifyEmail(token);
await ayb.auth.resendVerification();

// Restore tokens from storage
ayb.setTokens(savedToken, savedRefreshToken);
```

### Storage

```ts
// Upload a file to a bucket
const file = document.querySelector("input[type=file]").files[0];
const result = await ayb.storage.upload("avatars", file);
// result: { id, bucket, name, size, contentType, createdAt, updatedAt }

// Upload with a custom filename
await ayb.storage.upload("documents", blob, "report.pdf");

// Get download URL
const url = ayb.storage.downloadURL("avatars", "photo.jpg");
// → "http://localhost:8090/api/storage/avatars/photo.jpg"

// List files in a bucket
const files = await ayb.storage.list("avatars", { prefix: "user_", limit: 20 });

// Get a signed URL (time-limited access, default 1 hour)
const { url: signedUrl } = await ayb.storage.getSignedURL("avatars", "photo.jpg", 3600);

// Delete
await ayb.storage.delete("avatars", "photo.jpg");
```

### Realtime

```ts
// Subscribe to table changes (Server-Sent Events)
const unsubscribe = ayb.realtime.subscribe(
  ["posts", "comments"],
  (event) => {
    console.log(event.action, event.table, event.record);
    // action: "create" | "update" | "delete"
  },
);

// Stop listening
unsubscribe();
```

## TypeScript

All methods accept generic type parameters for full type safety:

```ts
interface Post {
  id: string;
  title: string;
  published: boolean;
  created_at: string;
}

const posts = await ayb.records.list<Post>("posts");
// posts.items is Post[]
```

Exported types: `ListResponse`, `ListParams`, `GetParams`, `AuthResponse`, `User`, `RealtimeEvent`, `StorageObject`, `ClientOptions`.

## Error Handling

All API errors throw `AYBError` with the HTTP status code:

```ts
import { AYBClient, AYBError } from "@allyourbase/js";

try {
  await ayb.records.get("posts", "nonexistent");
} catch (err) {
  if (err instanceof AYBError) {
    console.log(err.status);  // 404
    console.log(err.message); // "record not found"
  }
}
```

## License

MIT

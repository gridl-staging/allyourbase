# Realtime

AYB streams database changes in real time using Server-Sent Events (SSE). Subscribe to specific tables and receive create, update, and delete events filtered by Row-Level Security policies.

## Endpoint

```
GET /api/realtime?tables=posts,comments
```

### With authentication

Pass the JWT token as a query parameter:

```
GET /api/realtime?tables=posts&token=eyJhbG...
```

## Event format

Each SSE event contains a JSON payload:

```json
{
  "action": "create",
  "table": "posts",
  "record": {
    "id": 42,
    "title": "New Post",
    "published": true,
    "created_at": "2026-02-07T22:00:00Z"
  }
}
```

Actions: `create`, `update`, `delete`.

## Browser usage

```js
const params = new URLSearchParams({
  tables: "posts,comments",
  token: "eyJhbG...", // optional, for authenticated streams
});

const es = new EventSource(`http://localhost:8090/api/realtime?${params}`);

es.onmessage = (e) => {
  const event = JSON.parse(e.data);
  console.log(event.action, event.table, event.record);
};

es.onerror = () => {
  console.error("Connection lost, reconnecting...");
};

// Close when done
es.close();
```

`EventSource` automatically reconnects on connection loss.

## JavaScript SDK

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");
await ayb.auth.login("user@example.com", "password");

const unsubscribe = ayb.realtime.subscribe(
  ["posts", "comments"],
  (event) => {
    switch (event.action) {
      case "create":
        console.log(`New ${event.table}:`, event.record);
        break;
      case "update":
        console.log(`Updated ${event.table}:`, event.record);
        break;
      case "delete":
        console.log(`Deleted from ${event.table}:`, event.record);
        break;
    }
  },
);

// Stop listening
unsubscribe();
```

## RLS filtering

When auth is enabled, realtime events are filtered per-client based on PostgreSQL RLS policies. Each connected client only receives events for records they have permission to see.

For example, if you have an RLS policy that restricts `posts` to the author:

```sql
CREATE POLICY posts_select ON posts
  FOR SELECT
  USING (author_id = current_setting('ayb.user_id')::uuid);
```

Then each SSE client will only receive events for posts they authored.

### Joined-table policies are supported

The realtime filter runs a per-event `SELECT 1 FROM ... WHERE pk = ...` inside an `ayb_authenticated` RLS context. PostgreSQL evaluates the full table policy expression for that row, including join/`EXISTS` policies against related membership tables.

That means policies like:

```sql
USING (
  EXISTS (
    SELECT 1
    FROM project_memberships pm
    WHERE pm.project_id = secure_docs.project_id
      AND pm.user_id = current_setting('ayb.user_id', true)
  )
)
```

are enforced correctly for SSE visibility checks.

### Permissions are evaluated per event (not per subscription)

RLS checks happen when each event is delivered, not only when a client subscribes. If a user's membership is granted or revoked, visibility updates immediately for subsequent events on that stream.

### Delete-event pass-through semantics

Delete events are intentionally delivered without a row-visibility query because the row no longer exists to evaluate with `SELECT ... WHERE pk = ...`.

This behavior is intentional and safe for AYB realtime payloads:
- Delete payloads include key identifying data, not full sensitive row content.
- Attempting a "pre-delete visibility check" in the delivery path introduces race conditions and can still become stale before dispatch.

<!-- audited 2026-03-20 -->

# Realtime

AYB streams database changes in real time using Server-Sent Events (SSE) and WebSockets. Subscribe to specific tables and receive create, update, and delete events filtered by Row-Level Security policies.

## Endpoint

```
GET /api/realtime?tables=posts,comments
GET /api/realtime/ws
```

### With authentication

Prefer `Authorization: Bearer ...` when the client can set headers.
Use the `token` query parameter only for clients such as browser
`EventSource` that cannot attach custom auth headers, and avoid putting
long-lived or admin tokens in URLs because they can leak via browser
history, logs, and intermediaries.

For SSE, you can authenticate with either an `Authorization` header or a
`token` query parameter:

```
GET /api/realtime?tables=posts&token=eyJhbG...
```

### Optional SSE filter parameter

You can narrow already-authorized events with `filter`:

```
GET /api/realtime?tables=posts&filter=status=eq.published
```

Filter format: `column=operator.value` (comma-separated for AND conditions). Supported operators: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`.

## Configuration

Realtime behavior can be tuned in `ayb.toml` under `[realtime]`:

```toml
[realtime]
max_connections_per_user = 100
heartbeat_interval_seconds = 25
broadcast_rate_limit_per_second = 100
broadcast_max_message_bytes = 262144
presence_leave_timeout_seconds = 10
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
const params = new URLSearchParams({ tables: "posts,comments" });

// If the stream must be authenticated in a browser EventSource client,
// add a short-lived user token here because custom auth headers are not available.
params.set("token", "eyJhbG...");

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

## WebSocket usage

```js
const ws = new WebSocket("ws://localhost:8090/api/realtime/ws");

ws.onopen = () => {
  ws.send(JSON.stringify({ type: "auth", token: "eyJhbG..." }));
  ws.send(JSON.stringify({
    type: "subscribe",
    tables: ["posts", "comments"],
    filter: "status=eq.published",  // optional
    ref: "sub1",                     // optional — server echoes ref in reply
  }));
};

ws.onmessage = (e) => {
  const msg = JSON.parse(e.data);
  switch (msg.type) {
    case "connected":
      console.log("Connected, client ID:", msg.client_id);
      break;
    case "reply":
      console.log(msg.ref, msg.status, msg.message);  // "ok" or "error"
      break;
    case "event":
      console.log(msg.action, msg.table, msg.record);
      break;
  }
};
```

### Client→server message types

| Type | Fields | Purpose |
|------|--------|---------|
| `auth` | `token` | Authenticate after connect |
| `subscribe` | `tables`, `filter?`, `ref?` | Subscribe to table events |
| `unsubscribe` | `tables`, `ref?` | Unsubscribe from tables |

### Server→client message types

| Type | Fields | Purpose |
|------|--------|---------|
| `connected` | `client_id` | Sent immediately on connect |
| `reply` | `ref`, `status`, `message?` | Acknowledgement (`"ok"` or `"error"`) |
| `event` | `action`, `table`, `record` | Database change event |
| `error` | `message` | Top-level error |

Post-connect auth keeps tokens out of the URL and is the preferred browser
WebSocket pattern. Query-string tokens should be reserved for clients that
cannot send an auth message after connect.

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

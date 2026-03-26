# REST API Reference
<!-- audited 2026-03-20 -->

AYB auto-generates REST endpoints for every table in your PostgreSQL database.

## Collections (CRUD)

```
GET    /api/collections/{table}          List records
POST   /api/collections/{table}          Create record
POST   /api/collections/{table}/batch    Batch operations
GET    /api/collections/{table}/{id}     Get record
PATCH  /api/collections/{table}/{id}     Update record (partial)
DELETE /api/collections/{table}/{id}     Delete record
```

### List records

```bash
curl "http://localhost:8090/api/collections/posts?filter=status='active'&sort=-created_at&page=1&perPage=20"
```

**Response:**

```json
{
  "items": [
    { "id": 1, "title": "Hello", "published": true, "created_at": "2026-02-07T..." }
  ],
  "page": 1,
  "perPage": 20,
  "totalItems": 42,
  "totalPages": 3
}
```

### Query parameters

| Parameter | Example | Description |
|-----------|---------|-------------|
| `search` | `?search=hello world` | Full-text search across all text columns |
| `filter` | `?filter=status='active' AND age>21` | SQL-safe parameterized filtering |
| `sort` | `?sort=-created_at,+title` | Sort by fields (`-` desc, `+` asc) |
| `page` | `?page=2` | Page number (default: 1) |
| `perPage` | `?perPage=50` | Items per page (default: 20, max: 500) |
| `fields` | `?fields=id,name,email` | Select specific columns |
| `expand` | `?expand=author,category` | Expand foreign key relationships |
| `skipTotal` | `?skipTotal=true` | Skip COUNT query for faster responses |

### Filter syntax

Filters use a safe, parameterized syntax. All values are bound as query parameters — no SQL injection risk.

```
# Equality
?filter=status='active'

# Comparison
?filter=age>21
?filter=price<=100

# AND / OR (keywords or symbols)
?filter=status='active' AND category='tech'
?filter=role='admin' OR role='editor'
?filter=status='active' && category='tech'
?filter=role='admin' || role='editor'

# NULL checks (use =null or !=null)
?filter=deleted_at=null
?filter=email!=null

# Pattern matching (use ~ for LIKE, !~ for NOT LIKE)
?filter=name~'%john%'
?filter=name!~'%admin%'

# NOT equal
?filter=status!='draft'

# IN list
?filter=status IN ('active','pending','review')

# Grouping with parentheses
?filter=(status='active' OR status='pending') AND category='tech'

# Boolean and numeric values
?filter=published=true
?filter=age>21 AND score<=100
```

#### Operator reference

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equal (or `IS NULL` when value is `null`) | `status='active'` |
| `!=` | Not equal (or `IS NOT NULL` when value is `null`) | `status!='draft'` |
| `>` | Greater than | `age>21` |
| `>=` | Greater than or equal | `score>=90` |
| `<` | Less than | `price<100` |
| `<=` | Less than or equal | `price<=50` |
| `~` | LIKE (pattern match) | `name~'%john%'` |
| `!~` | NOT LIKE | `name!~'%test%'` |
| `IN` | In list | `status IN ('a','b')` |
| `AND` / `&&` | Logical AND | `a='x' AND b='y'` |
| `OR` / `\|\|` | Logical OR | `a='x' OR a='y'` |

Values: strings in single quotes (`'hello'`), numbers (`42`, `3.14`), booleans (`true`, `false`), `null`.

### Full-text search

Use `?search=` to search across all text columns (`text`, `varchar`, `char`) in a table:

```bash
curl "http://localhost:8090/api/collections/posts?search=postgres database"
```

Search uses PostgreSQL's `websearch_to_tsquery`, so it supports natural search syntax:

```
# Simple search
?search=postgres

# Multi-word (AND by default)
?search=postgres database

# Exact phrase
?search="full text search"

# OR
?search=postgres or mysql

# Exclude terms
?search=postgres -mysql
```

Results are automatically ranked by relevance when no explicit `sort` is provided.

Search can be combined with filters:

```bash
curl "http://localhost:8090/api/collections/posts?search=postgres&filter=status='active'&perPage=10"
```

::: tip Performance
For tables with many rows, add a GIN index on text columns for faster search:

```sql
CREATE INDEX posts_fts_idx ON posts USING GIN (
  to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(body, ''))
);
```
:::

### Vector and semantic search parameters

Collection list endpoints also support vector-search query modes:

- `nearest=[...]` for raw nearest-neighbor search
- `semantic_query=<text>` for embedding-based nearest-neighbor search
- `search=<text>&semantic=true` for hybrid full-text + vector fusion

Shared vector params:

- `vector_column=<column_name>` when needed
- `distance=cosine|l2|inner_product` (default: `cosine`)

See [AI and Vector Search](/guide/ai-vector) for supported payloads, response fields, and admin vector index endpoints.

### Batch operations

Perform multiple create, update, and delete operations in a single atomic transaction. If any operation fails, all changes are rolled back.

```bash
curl -X POST http://localhost:8090/api/collections/posts/batch \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"method": "create", "body": {"title": "Post A", "published": true}},
      {"method": "create", "body": {"title": "Post B", "published": false}},
      {"method": "update", "id": "42", "body": {"published": true}},
      {"method": "delete", "id": "99"}
    ]
  }'
```

**Request body:**

| Field | Type | Description |
|-------|------|-------------|
| `operations` | array | Array of operations (max 1000) |
| `operations[].method` | string | `"create"`, `"update"`, or `"delete"` |
| `operations[].id` | string | Record ID (required for update/delete) |
| `operations[].body` | object | Record data (required for create/update) |

**Response** (200 OK):

```json
[
  {"index": 0, "status": 201, "body": {"id": 100, "title": "Post A", "published": true}},
  {"index": 1, "status": 201, "body": {"id": 101, "title": "Post B", "published": false}},
  {"index": 2, "status": 200, "body": {"id": 42, "title": "Existing", "published": true}},
  {"index": 3, "status": 204}
]
```

All operations run in a single database transaction. RLS policies apply. Realtime and webhook events are published after successful commit.

### Create a record

```bash
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "New Post", "body": "Content", "published": false}'
```

**Response** (201 Created):

```json
{
  "id": 42,
  "title": "New Post",
  "body": "Content",
  "published": false,
  "created_at": "2026-02-07T22:00:00Z"
}
```

### Get a record

```bash
curl http://localhost:8090/api/collections/posts/42
```

**Response:**

```json
{
  "id": 42,
  "title": "New Post",
  "body": "Content",
  "published": false,
  "created_at": "2026-02-07T22:00:00Z"
}
```

Supports `?fields=` and `?expand=` query parameters.

### Update a record

```bash
curl -X PATCH http://localhost:8090/api/collections/posts/42 \
  -H "Content-Type: application/json" \
  -d '{"published": true}'
```

**Response:**

```json
{
  "id": 42,
  "title": "New Post",
  "body": "Content",
  "published": true,
  "created_at": "2026-02-07T22:00:00Z"
}
```

Only the specified fields are updated (partial update). The full updated record is returned.

### Delete a record

```bash
curl -X DELETE http://localhost:8090/api/collections/posts/42
```

Returns `204 No Content` on success.

### Expand foreign keys

If your `posts` table has an `author_id` column referencing `users(id)`:

```bash
curl "http://localhost:8090/api/collections/posts?expand=author"
```

**Response:**

```json
{
  "items": [
    {
      "id": 1,
      "title": "Hello",
      "author_id": 42,
      "expand": {
        "author": {
          "id": 42,
          "name": "Jane",
          "email": "jane@example.com"
        }
      }
    }
  ],
  "page": 1,
  "perPage": 20,
  "totalItems": 1,
  "totalPages": 1
}
```

Related records are nested under an `expand` key. For many-to-one relationships, the expanded value is a single object. For one-to-many, it's an array.

## Admin: Apps

Admin app-management endpoints are available under `/api/admin/apps` and require a valid admin token (`Authorization: Bearer <admin-token>`).

```
GET    /api/admin/apps          List apps (paginated)
POST   /api/admin/apps          Create app
GET    /api/admin/apps/{id}     Get app by id
PUT    /api/admin/apps/{id}     Update app
DELETE /api/admin/apps/{id}     Delete app
```

### Create app

```bash
curl -X POST http://localhost:8090/api/admin/apps \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "sigil-web",
    "description": "Sigil web client",
    "ownerUserId": "00000000-0000-0000-0000-000000000001"
  }'
```

**Response** (201 Created):

```json
{
  "id": "00000000-0000-0000-0000-000000000010",
  "name": "sigil-web",
  "description": "Sigil web client",
  "ownerUserId": "00000000-0000-0000-0000-000000000001",
  "rateLimitRps": 0,
  "rateLimitWindowSeconds": 0,
  "createdAt": "2026-02-22T00:00:00Z",
  "updatedAt": "2026-02-22T00:00:00Z"
}
```

## Admin: Sites

Admin hosted-site endpoints are available under `/api/admin/sites` and require a valid admin token.

```
GET    /api/admin/sites                              List sites (paginated)
POST   /api/admin/sites                              Create site
GET    /api/admin/sites/{siteId}                     Get site
PUT    /api/admin/sites/{siteId}                     Update site
DELETE /api/admin/sites/{siteId}                     Delete site
GET    /api/admin/sites/{siteId}/deploys             List deploys
POST   /api/admin/sites/{siteId}/deploys             Create deploy
GET    /api/admin/sites/{siteId}/deploys/{deployId}  Get deploy
POST   /api/admin/sites/{siteId}/deploys/{deployId}/files   Upload deploy file (multipart)
POST   /api/admin/sites/{siteId}/deploys/{deployId}/promote Promote deploy live
POST   /api/admin/sites/{siteId}/deploys/{deployId}/fail    Mark deploy failed
POST   /api/admin/sites/{siteId}/deploys/rollback           Roll back to previous live deploy
```

`/files` is mounted when storage services are configured (typical for hosted static-site deploy flows).

## Admin: API keys

Admin API-key endpoints are available under `/api/admin/api-keys` and require a valid admin token.

```
GET    /api/admin/api-keys          List API keys (paginated)
POST   /api/admin/api-keys          Create API key
DELETE /api/admin/api-keys/{id}     Revoke API key
```

### Create app-scoped API key

```bash
curl -X POST http://localhost:8090/api/admin/api-keys \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "00000000-0000-0000-0000-000000000001",
    "name": "sigil-ingestor",
    "scope": "readonly",
    "allowedTables": ["workouts", "clubs"],
    "appId": "00000000-0000-0000-0000-000000000010"
  }'
```

`"appId"` is optional. Omit it to create a legacy user-scoped API key (`appId = null` in responses).

### App rate limiting

If an API key is scoped to an app with a configured rate limit, exceeding the limit returns `429 Too Many Requests`:

```json
{
  "code": 429,
  "message": "app rate limit exceeded",
  "doc_url": "https://allyourbase.io/guide/api-reference"
}
```

The response also includes a `Retry-After` header with the number of seconds until the next allowed request window.

## Admin: Usage Metering

Admin usage-metering endpoints are available under `/api/admin/usage` and require a valid admin token.

```
GET /api/admin/usage                      List usage rows (paginated/filterable)
GET /api/admin/usage/trends               Usage trend series
GET /api/admin/usage/breakdown            Usage grouped breakdowns
GET /api/admin/usage/{tenant_id}          Tenant usage summary
GET /api/admin/usage/{tenant_id}/limits   Tenant usage limits
```

## Admin: Logs

Admin log endpoints require a valid admin token.

```
GET /api/admin/logs   Operational admin logs feed
GET /api/admin/audit  Admin audit trail query
```

## Admin: Storage CDN Purge

Storage CDN purge endpoint requires a valid admin token.

```
POST /api/admin/storage/cdn/purge
```

Request body supports exactly one mode:

- `{"urls":["https://cdn.example.com/a"]}` for targeted URL invalidation
- `{"purge_all":true}` for full-cache purge (rate limited)

## Admin: Jobs

Admin job endpoints are available under `/api/admin/jobs`, require a valid admin token, and require `jobs.enabled = true`.

```
GET  /api/admin/jobs                List jobs (filters: state, type, limit, offset)
GET  /api/admin/jobs/stats          Queue stats
GET  /api/admin/jobs/{id}           Get job
POST /api/admin/jobs/{id}/retry     Retry failed job (sets state to queued)
POST /api/admin/jobs/{id}/cancel    Cancel queued job
```

If jobs are not enabled, these endpoints return `503 Service Unavailable` with message `job queue is not enabled`.

### List jobs

```bash
curl "http://localhost:8090/api/admin/jobs?state=failed&type=webhook_delivery_prune&limit=20" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "items": [
    {
      "id": "33333333-3333-3333-3333-333333333333",
      "type": "webhook_delivery_prune",
      "state": "failed",
      "attempts": 3,
      "maxAttempts": 3,
      "lastError": "connection refused",
      "createdAt": "2026-02-22T00:00:00Z",
      "updatedAt": "2026-02-22T00:03:00Z"
    }
  ],
  "count": 1
}
```

### Queue stats

```bash
curl http://localhost:8090/api/admin/jobs/stats \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "queued": 2,
  "running": 1,
  "completed": 12,
  "failed": 1,
  "canceled": 0,
  "oldestQueuedAgeSec": 18.5
}
```

## Admin: Schedules

Admin schedule endpoints are available under `/api/admin/schedules`, require a valid admin token, and require `jobs.enabled = true`.

```
GET    /api/admin/schedules             List schedules
POST   /api/admin/schedules             Create schedule
PUT    /api/admin/schedules/{id}        Update schedule
DELETE /api/admin/schedules/{id}        Delete schedule
POST   /api/admin/schedules/{id}/enable Enable schedule
POST   /api/admin/schedules/{id}/disable Disable schedule
```

### Create schedule

```bash
curl -X POST http://localhost:8090/api/admin/schedules \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "session_cleanup_hourly",
    "jobType": "stale_session_cleanup",
    "cronExpr": "0 * * * *",
    "timezone": "UTC",
    "payload": {},
    "enabled": true
  }'
```

**Response** (201 Created):

```json
{
  "id": "aaaa1111-1111-1111-1111-111111111111",
  "name": "session_cleanup_hourly",
  "jobType": "stale_session_cleanup",
  "cronExpr": "0 * * * *",
  "timezone": "UTC",
  "enabled": true,
  "maxAttempts": 3,
  "nextRunAt": "2026-02-22T11:00:00Z",
  "createdAt": "2026-02-22T10:20:00Z",
  "updatedAt": "2026-02-22T10:20:00Z"
}
```

Validation notes:

- `cronExpr` must be a valid 5-field cron expression.
- `timezone` must be a valid IANA timezone.
- `name` and `jobType` are required.

## Admin: Materialized Views

Admin materialized view endpoints are available under `/api/admin/matviews` and require a valid admin token.

```
GET    /api/admin/matviews              List registered matviews
POST   /api/admin/matviews              Register a matview
GET    /api/admin/matviews/{id}         Get matview registration
PUT    /api/admin/matviews/{id}         Update refresh mode
DELETE /api/admin/matviews/{id}         Unregister matview
POST   /api/admin/matviews/{id}/refresh Trigger immediate refresh
```

### Register a materialized view

```bash
curl -X POST http://localhost:8090/api/admin/matviews \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "viewName": "leaderboard",
    "schema": "public",
    "refreshMode": "standard"
  }'
```

`schema` defaults to `"public"`. `refreshMode` defaults to `"standard"` (alternative: `"concurrent"`).

**Response** (201 Created):

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "schemaName": "public",
  "viewName": "leaderboard",
  "refreshMode": "standard",
  "lastRefreshAt": null,
  "lastRefreshStatus": null,
  "createdAt": "2026-02-22T08:00:00Z",
  "updatedAt": "2026-02-22T08:00:00Z"
}
```

Returns `404` if the target is not a materialized view, `409` if already registered, `400` for invalid identifiers.

### Trigger immediate refresh

```bash
curl -X POST http://localhost:8090/api/admin/matviews/550e8400-e29b-41d4-a716-446655440000/refresh \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "registration": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "schemaName": "public",
    "viewName": "leaderboard",
    "refreshMode": "standard",
    "lastRefreshAt": "2026-02-22T10:00:00Z",
    "lastRefreshDurationMs": 342,
    "lastRefreshStatus": "success"
  },
  "durationMs": 342
}
```

Returns `409` if a refresh is already in progress, if concurrent mode is missing a unique index, or if the view is not yet populated for concurrent refresh. Returns `404` if the registration or view no longer exists.

### List registered matviews

```bash
curl http://localhost:8090/api/admin/matviews \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "items": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "schemaName": "public",
      "viewName": "leaderboard",
      "refreshMode": "standard",
      "lastRefreshAt": "2026-02-22T10:00:00Z",
      "lastRefreshDurationMs": 342,
      "lastRefreshStatus": "success",
      "lastRefreshError": null
    }
  ],
  "count": 1
}
```

## Admin: Email Templates

Admin email-template endpoints are available under `/api/admin/email` and require a valid admin token.

```
GET    /api/admin/email/templates                 List effective template rows (system + custom)
GET    /api/admin/email/templates/{key}           Get effective template for key
PUT    /api/admin/email/templates/{key}           Create/update custom override
PATCH  /api/admin/email/templates/{key}           Enable/disable custom override
DELETE /api/admin/email/templates/{key}           Delete custom override
POST   /api/admin/email/templates/{key}/preview   Render preview (does not save)
POST   /api/admin/email/send                      Render and send email by template key
```

`{key}` accepts dot-notation keys like `auth.password_reset` and `app.club_invite`.

### List templates

```bash
curl http://localhost:8090/api/admin/email/templates \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "items": [
    {
      "templateKey": "auth.password_reset",
      "source": "builtin",
      "subjectTemplate": "Reset your password",
      "enabled": true
    },
    {
      "templateKey": "app.club_invite",
      "source": "custom",
      "subjectTemplate": "You're invited to {{.ClubName}}",
      "enabled": true,
      "updatedAt": "2026-02-22T12:00:00Z"
    }
  ],
  "count": 2
}
```

### Get effective template

```bash
curl http://localhost:8090/api/admin/email/templates/auth.password_reset \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "source": "builtin",
  "templateKey": "auth.password_reset",
  "subjectTemplate": "Reset your password",
  "htmlTemplate": "<p>...</p>",
  "enabled": true,
  "variables": ["AppName", "ActionURL"]
}
```

### Upsert custom override

```bash
curl -X PUT http://localhost:8090/api/admin/email/templates/auth.password_reset \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "subjectTemplate": "Reset your {{.AppName}} password",
    "htmlTemplate": "<p>Click <a href=\"{{.ActionURL}}\">{{.ActionURL}}</a></p>"
  }'
```

Returns `400` for invalid key format, template parse errors, or oversized payload.

### Preview template

```bash
curl -X POST http://localhost:8090/api/admin/email/templates/auth.password_reset/preview \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "subjectTemplate": "Reset your {{.AppName}} password",
    "htmlTemplate": "<p>Reset link: {{.ActionURL}}</p>",
    "variables": {
      "AppName": "Sigil",
      "ActionURL": "https://sigil.example/reset/abc"
    }
  }'
```

**Response** (200 OK):

```json
{
  "subject": "Reset your Sigil password",
  "html": "<p>Reset link: https://sigil.example/reset/abc</p>",
  "text": "Reset link: https://sigil.example/reset/abc"
}
```

### Toggle custom override

```bash
curl -X PATCH http://localhost:8090/api/admin/email/templates/auth.password_reset \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'
```

**Response** (200 OK):

```json
{
  "templateKey": "auth.password_reset",
  "enabled": false
}
```

### Delete custom override

```bash
curl -X DELETE http://localhost:8090/api/admin/email/templates/auth.password_reset \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

Returns `204 No Content` on success.

### Send email by template key

```bash
curl -X POST http://localhost:8090/api/admin/email/send \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "templateKey": "app.club_invite",
    "to": "user@example.com",
    "variables": {
      "ClubName": "Sunrise Runners",
      "InviteURL": "https://sigil.example/invite/abc123"
    }
  }'
```

**Response** (200 OK):

```json
{
  "status": "sent"
}
```

Error mapping highlights:

- `400`: invalid key, parse/render errors, missing required fields, invalid recipient format
- `404`: template key has no custom or built-in template
- `500`: internal send failure

System template variables:

- `auth.password_reset`: `AppName`, `ActionURL`
- `auth.email_verification`: `AppName`, `ActionURL`
- `auth.magic_link`: `AppName`, `ActionURL`

## Admin: Push Notifications

Admin push notification endpoints are available under `/api/admin/push` and require a valid admin token. Push must be enabled (`push.enabled = true`).

```
GET    /api/admin/push/devices              List device tokens
POST   /api/admin/push/devices              Register device token
DELETE /api/admin/push/devices/:id          Revoke device token
POST   /api/admin/push/send                 Send push to user
POST   /api/admin/push/send-to-token        Send to specific token
GET    /api/admin/push/deliveries            List deliveries
GET    /api/admin/push/deliveries/:id        Get delivery details
```

If push is not enabled, these endpoints return `503 Service Unavailable` with message `push notifications are not enabled`.

### List device tokens

```bash
curl "http://localhost:8090/api/admin/push/devices?app_id=UUID&user_id=UUID&include_inactive=true" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "items": [
    {
      "id": "uuid",
      "app_id": "uuid",
      "user_id": "uuid",
      "provider": "fcm",
      "platform": "android",
      "token": "dGVzdC10b2tlbi0x...",
      "device_name": "Pixel 8 Pro",
      "is_active": true,
      "last_refreshed_at": "2026-02-22T00:00:00Z",
      "last_used": "2026-02-22T01:00:00Z",
      "created_at": "2026-02-22T00:00:00Z",
      "updated_at": "2026-02-22T00:00:00Z"
    }
  ]
}
```

Query parameters: `app_id`, `user_id` (optional filters), `include_inactive` (default false).

### Register device token

```bash
curl -X POST http://localhost:8090/api/admin/push/devices \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "app_id": "uuid",
    "user_id": "uuid",
    "provider": "fcm",
    "platform": "android",
    "token": "device-token-string",
    "device_name": "Test Device"
  }'
```

**Response** (201 Created): Returns the created/updated device token object.

`provider` must be `fcm` or `apns`. `platform` must be `android` or `ios`.

### Send push notification

```bash
curl -X POST http://localhost:8090/api/admin/push/send \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "app_id": "uuid",
    "user_id": "uuid",
    "title": "New comment",
    "body": "Maya replied to your post",
    "data": {"post_id": "abc123"}
  }'
```

**Response** (200 OK):

```json
{
  "deliveries": [
    {
      "id": "uuid",
      "device_token_id": "uuid",
      "status": "pending",
      "title": "New comment",
      "body": "Maya replied to your post"
    }
  ]
}
```

Fans out to all active tokens for the user. Each delivery is processed asynchronously via the job queue.

### List deliveries

```bash
curl "http://localhost:8090/api/admin/push/deliveries?status=failed&limit=20" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "items": [
    {
      "id": "uuid",
      "provider": "fcm",
      "title": "New comment",
      "status": "failed",
      "error_code": "UNAVAILABLE",
      "error_message": "FCM service unavailable",
      "created_at": "2026-02-22T00:00:00Z"
    }
  ]
}
```

Query parameters: `app_id`, `user_id`, `status` (`pending`, `sent`, `failed`, `invalid_token`), `limit` (default 50), `offset` (default 0).

### Get delivery details

```bash
curl http://localhost:8090/api/admin/push/deliveries/DELIVERY_UUID \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

Returns the full delivery record including body, data payload, error details, and linked job ID.

## User-facing: Push Devices

User-facing push device endpoints are available under `/api/push/devices` and require JWT authentication. The user's identity is extracted from JWT claims.

```
POST   /api/push/devices                    Register device token
GET    /api/push/devices?app_id=UUID        List own active tokens
DELETE /api/push/devices/:id                Revoke own device token
```

### Register device token

```bash
curl -X POST http://localhost:8090/api/push/devices \
  -H "Authorization: Bearer $USER_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "app_id": "uuid",
    "provider": "fcm",
    "platform": "android",
    "token": "device-token-string",
    "device_name": "My Phone"
  }'
```

**Response** (201 Created): Returns the created/updated device token object.

The `user_id` is extracted from JWT claims — clients do not specify it.

### List own tokens

```bash
curl "http://localhost:8090/api/push/devices?app_id=UUID" \
  -H "Authorization: Bearer $USER_JWT"
```

Returns only the authenticated user's active tokens for the specified app.

### Revoke own token

```bash
curl -X DELETE http://localhost:8090/api/push/devices/DEVICE_TOKEN_ID \
  -H "Authorization: Bearer $USER_JWT"
```

Returns `204 No Content`. Ownership validation ensures users can only revoke their own tokens — attempting to revoke another user's token returns `404`.

## Admin: OAuth Clients

Admin OAuth client endpoints are available under `/api/admin/oauth/clients` and require a valid admin token. OAuth provider mode must be enabled.

```
GET    /api/admin/oauth/clients                        List OAuth clients (paginated)
POST   /api/admin/oauth/clients                        Create OAuth client
GET    /api/admin/oauth/clients/{clientId}              Get OAuth client
PUT    /api/admin/oauth/clients/{clientId}              Update OAuth client
DELETE /api/admin/oauth/clients/{clientId}              Revoke OAuth client
POST   /api/admin/oauth/clients/{clientId}/rotate-secret  Rotate client secret
```

### Create OAuth client

```bash
curl -X POST http://localhost:8090/api/admin/oauth/clients \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "appId": "00000000-0000-0000-0000-000000000001",
    "name": "My SPA",
    "clientType": "confidential",
    "redirectUris": ["https://myapp.com/callback"],
    "scopes": ["readonly"]
  }'
```

**Response** (201 Created):

```json
{
  "clientSecret": "ayb_cs_...",
  "client": {
    "id": "uuid",
    "appId": "uuid",
    "clientId": "ayb_cid_...",
    "name": "My SPA",
    "redirectUris": ["https://myapp.com/callback"],
    "scopes": ["readonly"],
    "clientType": "confidential",
    "createdAt": "2026-02-22T...",
    "updatedAt": "2026-02-22T...",
    "revokedAt": null,
    "activeAccessTokenCount": 0,
    "activeRefreshTokenCount": 0,
    "totalGrants": 0,
    "lastTokenIssuedAt": null
  }
}
```

### Rotate client secret

```bash
curl -X POST http://localhost:8090/api/admin/oauth/clients/ayb_cid_.../rotate-secret \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

**Response** (200 OK):

```json
{
  "clientSecret": "ayb_cs_..."
}
```

## OAuth Endpoints

OAuth 2.0 authorization server endpoints. See the [OAuth Provider Guide](./oauth-provider.md) for the full flow.

```
GET  /api/auth/authorize             Authorization endpoint (requires session)
POST /api/auth/authorize/consent     Consent decision endpoint (requires session)
POST /api/auth/token                 Token endpoint
POST /api/auth/revoke                Token revocation endpoint (RFC 7009)
```

### Authorization

```
GET /api/auth/authorize?response_type=code&client_id=ayb_cid_...&redirect_uri=https://...&scope=readonly&state=...&code_challenge=...&code_challenge_method=S256
```

Requires an authenticated user session. Returns either a consent prompt (JSON) or redirects with an authorization code.

### Token exchange

```bash
curl -X POST http://localhost:8090/api/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=...&redirect_uri=...&code_verifier=...&client_id=...&client_secret=..."
```

Supported `grant_type` values: `authorization_code`, `client_credentials`, `refresh_token`.

**Response** (200 OK):

```json
{
  "access_token": "ayb_at_...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "ayb_rt_...",
  "scope": "readonly"
}
```

**Error response** (RFC 6749 §5.2):

```json
{
  "error": "invalid_grant",
  "error_description": "authorization code expired"
}
```

### Token revocation

```bash
curl -X POST http://localhost:8090/api/auth/revoke \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=ayb_at_..."
```

Always returns `200 OK` per RFC 7009. Accepts optional `token_type_hint` parameter.

## GeoJSON columns (PostGIS)

When your database has PostGIS installed, geometry and geography columns are automatically serialized as [GeoJSON](https://datatracker.ietf.org/doc/html/rfc7946) objects in API responses and accepted as GeoJSON in request bodies.

### Create with GeoJSON

```bash
curl -X POST http://localhost:8090/api/collections/places \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Central Park",
    "location": { "type": "Point", "coordinates": [-73.9654, 40.7829] }
  }'
```

### Read — GeoJSON in responses

```json
{
  "id": "550e8400-...",
  "name": "Central Park",
  "location": { "type": "Point", "coordinates": [-73.9654, 40.7829] },
  "created_at": "2026-02-22T10:00:00Z"
}
```

GeoJSON serialization applies to all read paths: single record, list, expand, batch responses, and SSE realtime events.

See the [PostGIS guide](/guide/postgis) for setup, spatial query patterns, and deployment options.

## Schema

```bash
curl http://localhost:8090/api/schema
```

Returns the full database schema as JSON including tables, columns, types, primary keys, and foreign key relationships. When PostGIS is installed, the response includes `hasPostGIS`, `postGISVersion`, and geometry column metadata (`isGeometry`, `geometryType`, `srid`).

## Health check

```bash
curl http://localhost:8090/health
```

Returns `200 OK` when the server is running and the database is reachable.

## Error format

REST endpoints return a consistent JSON format:

```json
{
  "code": 404,
  "message": "collection not found: nonexistent",
  "doc_url": "https://allyourbase.io/guide/api-reference"
}
```

For validation errors (constraint violations), the response includes a `data` field with per-field detail:

```json
{
  "code": 409,
  "message": "unique constraint violation",
  "data": {
    "users_email_key": {
      "code": "unique_violation",
      "message": "Key (email)=(test@example.com) already exists."
    }
  },
  "doc_url": "https://allyourbase.io/guide/api-reference#error-format"
}
```

The `doc_url` field links to relevant documentation when available.

GraphQL endpoints use a different envelope (`{ "errors": [...] }`) and are documented in [GraphQL](/guide/graphql).

Common HTTP status codes:

| Status | Meaning |
|--------|---------|
| `400` | Invalid request (bad filter syntax, invalid JSON, invalid type/constraint payloads) |
| `401` | Unauthorized (missing or invalid JWT) |
| `403` | Forbidden (insufficient permissions / RLS policy denial) |
| `404` | Collection or record not found |
| `409` | Conflict (unique constraint violation) |
| `500` | Internal server error |

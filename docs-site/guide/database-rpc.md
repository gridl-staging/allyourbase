# Database RPC
<!-- audited 2026-03-20 -->

Call PostgreSQL functions directly via the REST API using the RPC endpoint.

## Endpoint

```
POST /api/rpc/{function_name}
```

## Route auth and scope behavior

- RPC is mounted under the same `/api` auth gates as collection routes.
- When auth is enabled, requests require the same user/admin bearer auth used by REST routes.
- API keys with readonly scope are rejected for RPC with `403` (`api key scope does not permit write operations`).

## Notify headers for realtime publish

RPC calls can optionally publish realtime events when both notify headers are present:

- `X-Notify-Table`: target table name for the event metadata
- `X-Notify-Action`: one of `create`, `update`, or `delete`

If either header is missing or `X-Notify-Action` is invalid, the RPC still executes normally and notify publishing is disabled for that request (no `400` is returned for notify contract issues).

Publish behavior depends on function result shape:

- `RETURNS VOID`: no event is published
- Scalar return: one event is published only when the decoded record is non-null; null results publish nothing
- Single-row object return: one event is published (null results are skipped, same as scalar)
- Set-returning array result: one event is published per returned row

## Create a function

```sql
CREATE OR REPLACE FUNCTION hello(name TEXT)
RETURNS TEXT AS $$
BEGIN
  RETURN 'Hello, ' || name || '!';
END;
$$ LANGUAGE plpgsql;
```

## Call it

```bash
curl -X POST http://localhost:8090/api/rpc/hello \
  -H "Content-Type: application/json" \
  -d '{"name": "World"}'
```

**Response:**

```json
"Hello, World!"
```

AYB returns the function result directly. RPC responses are not wrapped in a `{ "result": ... }` envelope.

## Return types

### Scalar (single value)

```sql
CREATE FUNCTION count_active_users() RETURNS INTEGER AS $$
  SELECT count(*)::integer FROM users WHERE active = true;
$$ LANGUAGE sql;
```

```json
42
```

### Set-returning (multiple rows)

```sql
CREATE FUNCTION recent_posts(n INTEGER)
RETURNS SETOF posts AS $$
  SELECT * FROM posts ORDER BY created_at DESC LIMIT n;
$$ LANGUAGE sql;
```

```json
[
  { "id": 1, "title": "Latest Post", "created_at": "..." },
  { "id": 2, "title": "Previous Post", "created_at": "..." }
]
```

### Void (no return value)

```sql
CREATE FUNCTION cleanup_old_sessions() RETURNS VOID AS $$
  DELETE FROM sessions WHERE expires_at < now();
$$ LANGUAGE sql;
```

Returns `204 No Content`.

## RLS support

When auth is enabled, RPC calls execute with the same RLS session variables (`ayb.user_id`, `ayb.user_email`) as regular API calls. Your functions can use `current_setting('ayb.user_id')` to access the authenticated user.

```sql
CREATE FUNCTION my_posts()
RETURNS SETOF posts AS $$
  SELECT * FROM posts
  WHERE author_id = current_setting('ayb.user_id')::uuid;
$$ LANGUAGE sql SECURITY DEFINER;
```

## Spatial queries with PostGIS

When your database has PostGIS installed, use RPC functions for spatial operations like nearby search, bounding box queries, and distance calculations.

### Nearby search

```sql
CREATE OR REPLACE FUNCTION find_nearby(
  lat FLOAT8,
  lng FLOAT8,
  radius_m FLOAT8
)
RETURNS TABLE(
  id UUID,
  name TEXT,
  location JSONB,
  distance_m FLOAT8
) AS $$
  SELECT
    p.id,
    p.name,
    ST_AsGeoJSON(p.location)::jsonb AS location,
    ST_Distance(p.location::geography, ST_MakePoint(lng, lat)::geography) AS distance_m
  FROM places p
  WHERE ST_DWithin(p.location::geography, ST_MakePoint(lng, lat)::geography, radius_m)
  ORDER BY distance_m
$$ LANGUAGE sql;
```

```bash
curl -X POST http://localhost:8090/api/rpc/find_nearby \
  -H "Content-Type: application/json" \
  -d '{"lat": 40.7829, "lng": -73.9654, "radius_m": 1000}'
```

::: tip GeoJSON in RPC results
Use `ST_AsGeoJSON(col)::jsonb` in your function body for geometry columns. AYB auto-wraps geometry in standard CRUD endpoints, but cannot introspect arbitrary function return schemas — you control the output format in RPC functions.
:::

### Distance calculation

```sql
CREATE OR REPLACE FUNCTION distance_between(
  lat1 FLOAT8, lng1 FLOAT8,
  lat2 FLOAT8, lng2 FLOAT8
)
RETURNS FLOAT8 AS $$
  SELECT ST_Distance(
    ST_MakePoint(lng1, lat1)::geography,
    ST_MakePoint(lng2, lat2)::geography
  )
$$ LANGUAGE sql;
```

### Bounding box search

```sql
CREATE OR REPLACE FUNCTION places_in_bbox(
  min_lng FLOAT8, min_lat FLOAT8,
  max_lng FLOAT8, max_lat FLOAT8
)
RETURNS TABLE(id UUID, name TEXT, location JSONB) AS $$
  SELECT id, name, ST_AsGeoJSON(location)::jsonb
  FROM places
  WHERE location && ST_MakeEnvelope(min_lng, min_lat, max_lng, max_lat, 4326)
$$ LANGUAGE sql;
```

See the [PostGIS guide](/guide/postgis) for more spatial patterns and deployment setup.

## Function discovery

AYB introspects `pg_proc` in non-system schemas to find available functions. Unqualified lookups prefer `public.<function_name>`, then match other loaded schemas by name.

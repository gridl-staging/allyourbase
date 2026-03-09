# Database RPC

Call PostgreSQL functions directly via the REST API using the RPC endpoint.

## Endpoint

```
POST /api/rpc/{function_name}
```

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
{
  "result": "Hello, World!"
}
```

## Return types

### Scalar (single value)

```sql
CREATE FUNCTION count_active_users() RETURNS INTEGER AS $$
  SELECT count(*)::integer FROM users WHERE active = true;
$$ LANGUAGE sql;
```

```json
{ "result": 42 }
```

### Set-returning (multiple rows)

```sql
CREATE FUNCTION recent_posts(n INTEGER)
RETURNS SETOF posts AS $$
  SELECT * FROM posts ORDER BY created_at DESC LIMIT n;
$$ LANGUAGE sql;
```

```json
{
  "result": [
    { "id": 1, "title": "Latest Post", "created_at": "..." },
    { "id": 2, "title": "Previous Post", "created_at": "..." }
  ]
}
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

## Function discovery

AYB introspects `pg_proc` to find available functions. Only functions in the `public` schema are exposed.

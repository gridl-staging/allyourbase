# GraphQL
<!-- audited 2026-03-20 -->

AYB exposes a GraphQL API backed by live database schema metadata.

## Endpoint

- HTTP query/mutation: `POST /api/graphql`
- WebSocket subscriptions (`graphql-transport-ws`): `GET /api/graphql` with WebSocket upgrade

`GET /api/graphql` without WebSocket upgrade is rejected with `405 Method Not Allowed` (`websocket upgrade required for GET /graphql`).

## Authentication

When auth is enabled:

- `POST /api/graphql` is mounted with the same admin-or-user auth middleware used by REST API routes.
- WebSocket auth is enforced in the `graphql-transport-ws` protocol init step (`connection_init`) via bearer token or API key validation.
- Invalid/missing auth during init closes the socket with close code `4401`.

```bash
curl -X POST http://localhost:8090/api/graphql \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"query":"{ posts(limit: 5) { id title } }"}'
```

## Query example

Table names become root fields.

```graphql
query Posts {
  posts(
    where: { published: { _eq: true } }
    order_by: { created_at: DESC }
    limit: 20
    offset: 0
  ) {
    id
    title
    published
  }
}
```

## Mutation examples

For table `posts`, AYB generates:

- `insert_posts`
- `update_posts`
- `delete_posts`

```graphql
mutation CreatePost {
  insert_posts(objects: [{ title: "Hello", published: true }]) {
    affected_rows
    returning {
      id
      title
    }
  }
}
```

```graphql
mutation UpdatePost {
  update_posts(
    where: { id: { _eq: 42 } }
    _set: { title: "Updated" }
  ) {
    affected_rows
    returning {
      id
      title
    }
  }
}
```

```graphql
mutation DeletePost {
  delete_posts(where: { id: { _eq: 42 } }) {
    affected_rows
  }
}
```

## Subscriptions

Subscriptions are table-based and stream row changes.

```graphql
subscription WatchPosts {
  posts(where: { published: { _eq: true } }) {
    id
    title
    published
  }
}
```

Use `graphql-transport-ws` protocol on `ws://localhost:8090/api/graphql`.

Transport behavior:

- Subprotocol `graphql-transport-ws` is required.
- `connection_init` must be sent before `subscribe`.
- Sending `subscribe` before `connection_init` closes with `4401`.

## Schema introspection

```graphql
query Introspect {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
  }
}
```

Introspection behavior is controlled by `graphql.introspection` config:

- `""` (default): admin-gated
- `"open"`: no introspection gate
- `"disabled"`: blocked

## Error envelope differences vs REST

- GraphQL request/validation/execution errors are returned as GraphQL errors (`{ "errors": [...] }`), commonly with HTTP `200`.
- Malformed JSON body returns HTTP `400` with GraphQL-style error envelope.
- Introspection blocked by policy returns HTTP `403` with GraphQL-style error envelope.
- REST endpoints use `{ code, message, data?, doc_url? }` via shared `httputil` helpers.

## Practical notes

- Tables prefixed with `_ayb_` are excluded.
- Views/materialized views are queryable but not mutation targets.
- `limit` is capped server-side (default max 1000 rows).
- Filters support `_eq`, `_neq`, `_gt`, `_gte`, `_lt`, `_lte`, `_in`, `_like`, `_ilike`, `_is_null`.

## Related guides

- [API Reference](/guide/api-reference)
- [Authentication](/guide/authentication)
- [Realtime](/guide/realtime)

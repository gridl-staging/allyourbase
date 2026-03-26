<!-- audited 2026-03-21 -->
# Python SDK

Use the `allyourbase` Python package for async access to auth, records, realtime, and storage.

## Install

```bash
pip install allyourbase
```

## Initialize

`AYBClient` takes `base_url` plus optional keyword-only `http_client` (`httpx.AsyncClient`).

```python
import asyncio
import httpx
from allyourbase import AYBClient

async def main() -> None:
    http_client = httpx.AsyncClient(timeout=10.0)
    async with AYBClient("http://localhost:8090", http_client=http_client) as ayb:
        result = await ayb.records.list("posts", per_page=20)
        print(len(result.items))

asyncio.run(main())
```

`AYBClient` exposes sub-clients `auth`, `records`, `storage`, `realtime`, plus `rpc()`.

## Exports

The package exports these symbols from `allyourbase`:

- `AYBClient`
- `AYBError`
- `AuthResponse`
- `BatchOperation`
- `BatchResult`
- `ListResponse`
- `RealtimeEvent`
- `StorageListResponse`
- `StorageObject`
- `User`

## Auth

```python
await ayb.auth.login("user@example.com", "password")

me = await ayb.auth.me()
await ayb.auth.refresh()
await ayb.auth.logout()
```

Also available:

- `register`
- `delete_account`
- `request_password_reset`
- `confirm_password_reset`
- `verify_email`
- `resend_verification`

Listen for auth state changes on the client:

```python
def handle_auth(event, session):
    print(event, session)

unsubscribe = ayb.on_auth_state_change(handle_auth)

# later
unsubscribe()
```

## Records

```python
created = await ayb.records.create("posts", {"title": "Hello"})

item = await ayb.records.get("posts", str(created["id"]))

updated = await ayb.records.update("posts", str(created["id"]), {"title": "Updated"})

listing = await ayb.records.list(
    "posts",
    filter="published=true",
    sort="-created_at",
    page=1,
    per_page=20,
)

await ayb.records.delete("posts", str(created["id"]))
```

### Batch

```python
from allyourbase import BatchOperation

results = await ayb.records.batch(
    "posts",
    [
        BatchOperation(method="create", body={"title": "A"}),
        BatchOperation(method="update", id="42", body={"title": "B"}),
    ],
)
```

## Realtime (SSE)

`subscribe` returns an unsubscribe callback.

```python
unsubscribe = await ayb.realtime.subscribe(
    ["posts", "comments"],
    lambda event: print(event.action, event.table, event.record),
)

# later
unsubscribe()
```

## Storage

`download_url()` is synchronous and returns a string URL immediately.

```python
uploaded = await ayb.storage.upload(
    "avatars",
    b"binary-file-bytes",
    "avatar.png",
    content_type="image/png",
)

public_url = ayb.storage.download_url("avatars", uploaded.name)

signed_url = await ayb.storage.get_signed_url("avatars", uploaded.name, expires_in=3600)

objects = await ayb.storage.list("avatars", prefix="user_", limit=20)
await ayb.storage.delete("avatars", uploaded.name)
```

## RPC

```python
result = await ayb.rpc("leaderboard_totals", {"org_id": "org-1"})
```

## Errors

`AYBError` fields: `status`, `message`, `code`, `data`, `doc_url`.

```python
from allyourbase import AYBError

try:
    await ayb.records.get("posts", "missing")
except AYBError as err:
    print(err.status, err.code, err.message, err.data, err.doc_url)
```

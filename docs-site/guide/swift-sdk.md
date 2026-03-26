<!-- audited 2026-03-21 -->
# Swift SDK

Use the Swift `Allyourbase` SDK for iOS/macOS clients with auth, records, realtime (SSE + WebSocket), and storage.

## Install

`sdk_swift/Package.swift` defines package name `Allyourbase` and library product `Allyourbase`.

Add `Allyourbase` to your Swift package dependencies from a local checkout of this repository (`Package.swift` is in `sdk_swift/`, not repo root).

Example (local path):

```swift
.package(path: "../sdk_swift")
```

Then depend on product `Allyourbase`.

## Initialize

`AYBClient` initializer:

- `AYBClient(_ baseURL: String, apiKey: String? = nil, transport: HTTPTransport? = nil, sseTransport: SSETransport? = nil, wsTransport: WebSocketTransport? = nil, tokenStore: TokenStore? = nil, timeout: TimeInterval = 30, maxRetries: Int = 0, retryDelay: TimeInterval = 0)`

```swift
import Allyourbase

let ayb = AYBClient("http://localhost:8090")
```

You can also pass `apiKey`, custom transports, token store, timeout, and retry settings through `AYBClient` initializer parameters.

## Auth

```swift
let registered = try await ayb.auth.register(email: "new@example.com", password: "password")
try await ayb.auth.login(email: "user@example.com", password: "password")

let me = try await ayb.auth.me()
let refreshed = try await ayb.auth.refresh()
try await ayb.auth.logout()
```

Listen for auth state updates:

```swift
let unsubscribe = ayb.onAuthStateChange { event, session in
    print(event.rawValue, session != nil ? "session-present" : "signed-out")
}

// later
unsubscribe()
```

## Records

```swift
let created = try await ayb.records.create("posts", data: ["title": "Hello"])

let fetched = try await ayb.records.get("posts", "42")

let updated = try await ayb.records.update("posts", id: "42", data: ["title": "Updated"])

let listing = try await ayb.records.list(
    "posts",
    params: ListParams(filter: "published=true", sort: "-created_at", perPage: 20)
)

try await ayb.records.delete("posts", id: "42")
```

Batch operations:

```swift
let results = try await ayb.records.batch("posts", operations: [
    BatchOperation(method: "create", body: ["title": "A"]),
    BatchOperation(method: "update", id: "42", body: ["title": "B"]),
])
```

## Realtime (SSE)

```swift
let stop = ayb.realtime.subscribe(tables: ["posts", "comments"]) { event in
    print(event.action, event.table, event.record)
}

// later
stop()
```

## Realtime (WebSocket)

The SDK also supports WebSocket realtime with table subscriptions, channel broadcast, and presence.

```swift
try await ayb.realtime.connectWebSocket()

let stopRows = try await ayb.realtime.subscribeWS(tables: ["posts"]) { event in
    print(event.action, event.record)
}

let leaveChannel = try await ayb.realtime.channelSubscribe("room:lobby")
let removeBroadcastListener = ayb.realtime.onBroadcast(channel: "room:lobby") { event, payload in
    print(event, payload)
}

try await ayb.realtime.broadcast(
    channel: "room:lobby",
    event: "chat.message",
    payload: ["text": "hello"],
    self: true
)

try await ayb.realtime.presenceTrack(channel: "room:lobby", state: ["name": "iOS client"])
let presences = try await ayb.realtime.presenceSync(channel: "room:lobby")
try await ayb.realtime.presenceUntrack(channel: "room:lobby")

_ = presences

// cleanup
stopRows()
leaveChannel()
removeBroadcastListener()
ayb.realtime.disconnectWebSocket()
```

## Storage

```swift
let data = Data("hello".utf8)
let uploaded = try await ayb.storage.upload(bucket: "docs", data: data, name: "hello.txt", contentType: "text/plain")

let downloadURL = ayb.storage.downloadUrl(bucket: "docs", name: uploaded.name)

let signedURL = try await ayb.storage.getSignedUrl(bucket: "docs", name: uploaded.name, expiresIn: 3600)

let list = try await ayb.storage.list(bucket: "docs", prefix: "hel", limit: 20)
try await ayb.storage.delete(bucket: "docs", name: uploaded.name)

_ = downloadURL
_ = signedURL
_ = list
```

## Errors

`AYBError` fields: `status`, `message`, `code`, `data`, `docUrl`.

```swift
do {
    _ = try await ayb.records.get("posts", "missing")
} catch let err as AYBError {
    print(err.status, err.code ?? "", err.message)
    print(err.data as Any, err.docUrl as Any)
}
```

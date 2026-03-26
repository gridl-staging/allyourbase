<!-- audited 2026-03-21 -->
# Kotlin SDK

Use the Kotlin SDK for AYB auth, records, storage, and realtime (SSE + WebSocket).

## Install

The SDK currently lives in the repository as `sdk_kotlin`.

```kotlin
// settings.gradle.kts
include(":sdk_kotlin")

// app/build.gradle.kts
dependencies {
    implementation(project(":sdk_kotlin"))
}
```

## Initialize

```kotlin
import dev.allyourbase.AYBClient

val ayb = AYBClient("http://localhost:8090")
```

You can pass `apiKey`, custom transports (`transport`, `sseTransport`, `wsTransport`), `tokenStore`, `timeout`, `maxRetries`, and `retryDelay` via constructor params.
Most SDK methods are `suspend` functions, so call them from a coroutine (for example, `runBlocking` or your app's existing coroutine scope).

Key model/data classes include `AuthResponse`, `User`, `ListParams`, `ListResponse`, `BatchOperation`, `BatchResult`, `StorageObject`, `StorageListResponse`, and `RealtimeEvent`.

## Auth

```kotlin
import kotlinx.coroutines.runBlocking

runBlocking {
    val registered = ayb.auth.register("new@example.com", "password")
    ayb.auth.login("user@example.com", "password")

    val me = ayb.auth.me()
    ayb.auth.refresh()
    ayb.auth.logout()
}
```

Listen for auth changes:

```kotlin
val unsubscribe = ayb.onAuthStateChange { event, session ->
    println("$event sessionPresent=${session != null}")
}

// later
unsubscribe()
```

## Records

```kotlin
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.coroutines.runBlocking

runBlocking {
    val created = ayb.records.create(
        "posts",
        buildJsonObject { put("title", "Hello") }
    )

    val post = ayb.records.get("posts", "42")

    val updated = ayb.records.update(
        "posts",
        "42",
        buildJsonObject { put("title", "Updated") }
    )

    val list = ayb.records.list(
        "posts",
        params = dev.allyourbase.ListParams(filter = "published=true", sort = "-created_at", perPage = 20)
    )

    ayb.records.delete("posts", "42")
}
```

Batch:

```kotlin
import dev.allyourbase.BatchOperation
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.coroutines.runBlocking

runBlocking {
    val batchResults = ayb.records.batch(
        "posts",
        listOf(
            BatchOperation(method = "create", body = buildJsonObject { put("title", "A") }),
            BatchOperation(method = "update", id = "42", body = buildJsonObject { put("title", "B") }),
        )
    )
}
```

## Realtime (SSE)

```kotlin
val stop = ayb.realtime.subscribe(listOf("posts", "comments")) { event ->
    println("${event.action} ${event.table}")
}

// later
stop()
```

You can also consume as Flow:

```kotlin
val flow = ayb.realtime.subscribeFlow(listOf("posts"))
```

## Realtime (WebSocket)

```kotlin
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.coroutines.runBlocking

runBlocking {
    ayb.realtime.connectWebSocket()

    val stopRows = ayb.realtime.subscribeWS(listOf("posts")) { event ->
        println(event.record)
    }

    val leaveChannel = ayb.realtime.channelSubscribe("room:lobby")

    val removeBroadcastListener = ayb.realtime.onBroadcast("room:lobby") { event, payload ->
        println("$event $payload")
    }

    ayb.realtime.broadcast(
        channel = "room:lobby",
        event = "chat.message",
        payload = buildJsonObject { put("text", "hello") },
        includeSelf = true,
    )

    ayb.realtime.presenceTrack(
        channel = "room:lobby",
        state = buildJsonObject { put("name", "android") },
    )

    val presences = ayb.realtime.presenceSync("room:lobby")
    ayb.realtime.presenceUntrack("room:lobby")

    // cleanup
    stopRows()
    leaveChannel()
    removeBroadcastListener()
    ayb.realtime.disconnectWebSocket()
}
```

## Storage

```kotlin
import kotlinx.coroutines.runBlocking

runBlocking {
    val uploaded = ayb.storage.upload(
        bucket = "docs",
        data = "hello".encodeToByteArray(),
        name = "hello.txt",
        contentType = "text/plain",
    )

    val downloadUrl = ayb.storage.downloadUrl("docs", uploaded.name)
    val signedUrl = ayb.storage.getSignedUrl("docs", uploaded.name, expiresIn = 3600)
    val listing = ayb.storage.list("docs", prefix = "hel", limit = 20)
    ayb.storage.delete("docs", uploaded.name)
}
```

## Errors

`AYBException` fields: `status`, `message`, `code`, `data`, `docUrl`.

```kotlin
import dev.allyourbase.AYBException
import kotlinx.coroutines.runBlocking

try {
    runBlocking {
        ayb.records.get("posts", "missing")
    }
} catch (e: AYBException) {
    println("${e.status} ${e.code} ${e.message}")
    println(e.data)
    println(e.docUrl)
}
```

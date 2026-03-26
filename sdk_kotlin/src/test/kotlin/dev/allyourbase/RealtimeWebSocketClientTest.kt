package dev.allyourbase

import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class RealtimeWebSocketClientTest {
    private val json = Json

    @Test
    fun `connect websocket validates connected handshake and url`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\",\"client_id\":\"c1\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }

        val client = AYBClient("https://api.example.com/base", wsTransport = wsTransport)
        client.setTokens("jwt_1", "refresh_1")

        client.realtime.connectWebSocket()

        assertEquals(1, wsTransport.connectRequests.size)
        assertEquals("wss://api.example.com/base/api/realtime/ws?token=jwt_1", wsTransport.connectRequests.first().url)

        client.realtime.disconnectWebSocket()
        client.realtime.close()
    }

    @Test
    fun `concurrent connect websocket is idempotent`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
            enqueueReceive("{\"type\":\"connected\"}")
        }
        val wsTransport = DelayedConnectWebSocketTransport(wsConnection, 25)
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        val first = async { client.realtime.connectWebSocket() }
        val second = async { client.realtime.connectWebSocket() }
        first.await()
        second.await()

        assertEquals(1, wsTransport.connectCount)
        client.realtime.close()
    }

    @Test
    fun `close disconnects websocket connection`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        client.realtime.connectWebSocket()
        assertFalse(wsConnection.isClosed)

        client.realtime.close()

        assertTrue(wsConnection.isClosed)
    }

    @Test
    fun `subscribe ws sends subscribe and unsubscribe messages`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r1\",\"status\":\"ok\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        val unsubscribe = client.realtime.subscribeWS(listOf("posts"), filter = "status='pub'") { }

        val subscribePayload = json.parseToJsonElement(wsConnection.sentTexts.first()).jsonObject
        assertEquals("subscribe", subscribePayload["type"]?.jsonPrimitive?.content)
        assertEquals("posts", subscribePayload["tables"]?.jsonObjectOrArrayFirst())
        assertEquals("status='pub'", subscribePayload["filter"]?.jsonPrimitive?.content)

        unsubscribe()
        waitUntil { wsConnection.sentTexts.size >= 2 }

        assertEquals(2, wsConnection.sentTexts.size)
        val unsubscribePayload = json.parseToJsonElement(wsConnection.sentTexts[1]).jsonObject
        assertEquals("unsubscribe", unsubscribePayload["type"]?.jsonPrimitive?.content)
        client.realtime.close()
    }

    @Test
    fun `event routing dispatches only matching table callbacks`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r1\",\"status\":\"ok\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        val received = mutableListOf<RealtimeEvent>()
        val unsubscribe = client.realtime.subscribeWS(listOf("posts")) { event -> received.add(event) }
        wsConnection.enqueueReceive("{\"type\":\"event\",\"table\":\"posts\",\"action\":\"INSERT\",\"record\":{\"id\":\"rec_1\"}}")
        wsConnection.enqueueReceive("{\"type\":\"event\",\"table\":\"comments\",\"action\":\"INSERT\",\"record\":{\"id\":\"rec_2\"}}")

        waitUntil { received.isNotEmpty() }
        unsubscribe()

        assertEquals(1, received.size)
        assertEquals("posts", received[0].table)
        client.realtime.close()
    }

    @Test
    fun `broadcast requires channel subscription and dispatches callbacks`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r1\",\"status\":\"ok\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r2\",\"status\":\"ok\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        runCatching {
            client.realtime.broadcast("room-1", "updated", buildJsonObject { put("id", "p1") })
        }.onSuccess {
            throw AssertionError("expected not-subscribed failure")
        }.onFailure { error ->
            val ayb = error as AYBException
            assertEquals("realtime/not-subscribed", ayb.code)
        }

        client.realtime.channelSubscribe("room-1")

        val received = mutableListOf<Pair<String, JsonObject>>()
        val removeListener = client.realtime.onBroadcast("room-1") { event, payload ->
            received += event to payload
        }

        wsConnection.enqueueReceive("{\"type\":\"broadcast\",\"channel\":\"room-1\",\"event\":\"updated\",\"payload\":{\"id\":\"p1\"}}")
        client.realtime.broadcast("room-1", "updated", buildJsonObject { put("id", "p1") }, includeSelf = true)

        waitUntil { received.isNotEmpty() }
        removeListener()

        assertEquals("updated", received[0].first)
        assertEquals("p1", received[0].second["id"]?.jsonPrimitive?.content)

        val payload = json.parseToJsonElement(wsConnection.sentTexts[1]).jsonObject
        assertEquals("broadcast", payload["type"]?.jsonPrimitive?.content)
        assertEquals("true", payload["self"]?.jsonPrimitive?.content)
        client.realtime.close()
    }

    @Test
    fun `presence track sync untrack works and sync result is cached`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r1\",\"status\":\"ok\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r2\",\"status\":\"ok\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r3\",\"status\":\"ok\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        client.realtime.channelSubscribe("room-1")
        client.realtime.presenceTrack("room-1", buildJsonObject { put("online", true) })

        val firstSyncDeferred = async {
            client.realtime.presenceSync("room-1")
        }
        waitUntil {
            wsConnection.sentTexts.any { text ->
                json.parseToJsonElement(text).jsonObject["type"]?.jsonPrimitive?.content == "presence_sync"
            }
        }
        wsConnection.enqueueReceive("{\"type\":\"presence\",\"channel\":\"room-1\",\"presence_action\":\"sync\",\"presences\":{\"u1\":{\"online\":true}}}")
        val firstSync = firstSyncDeferred.await()
        val secondSync = client.realtime.presenceSync("room-1")

        wsConnection.enqueueReceive("{\"type\":\"reply\",\"ref\":\"r4\",\"status\":\"ok\"}")
        client.realtime.presenceUntrack("room-1")

        assertEquals("true", firstSync["u1"]?.get("online")?.jsonPrimitive?.content)
        assertEquals(firstSync, secondSync)

        val syncMessages = wsConnection.sentTexts.map { json.parseToJsonElement(it).jsonObject }
            .filter { it["type"]?.jsonPrimitive?.content == "presence_sync" }
        assertEquals(1, syncMessages.size)
        client.realtime.close()
    }

    @Test
    fun `disconnect fails pending replies`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        val pending = async {
            runCatching { client.realtime.channelSubscribe("room-1") }.exceptionOrNull()
        }

        delay(10)
        client.realtime.disconnectWebSocket()

        val error = pending.await()
        assertTrue(error is AYBException)
        assertTrue((error as AYBException).message.contains("websocket"))
        client.realtime.close()
    }

    @Test
    fun `websocket state buffers reply race and resolves on register`() = runTest {
        val state = WebSocketState()
        val reply = WebSocketServerMessage(
            type = WebSocketServerMessageType.REPLY,
            ref = "r42",
            status = "ok",
        )

        state.bufferReply(reply)
        val deferred = state.registerPendingReply("r42")

        assertTrue(deferred.isCompleted)
        assertEquals("ok", deferred.await().status)
    }

    @Test
    fun `websocket state clears presence cache on unsubscribe and disconnect cleanup`() = runTest {
        val state = WebSocketState()
        val presences = mapOf("u1" to buildJsonObject { put("online", true) })

        state.addChannelSubscription("room-1")
        state.resolvePresenceSync("room-1", presences)
        assertEquals(presences, state.cachedPresenceSync("room-1"))

        state.removeChannelSubscription("room-1")
        assertNull(state.cachedPresenceSync("room-1"))

        state.addChannelSubscription("room-1")
        state.resolvePresenceSync("room-1", presences)
        assertEquals(presences, state.cachedPresenceSync("room-1"))

        state.clearAllCallbacksAndSubscriptions()
        assertNull(state.cachedPresenceSync("room-1"))
    }

    @Test
    fun `unsubscribe cleanup prevents future callback delivery`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r1\",\"status\":\"ok\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        val received = mutableListOf<RealtimeEvent>()
        val unsubscribe = client.realtime.subscribeWS(listOf("posts")) { event -> received.add(event) }
        unsubscribe()
        wsConnection.enqueueReceive("{\"type\":\"event\",\"table\":\"posts\",\"action\":\"INSERT\",\"record\":{\"id\":\"rec_1\"}}")

        delay(10)
        assertTrue(received.isEmpty())
        client.realtime.close()
    }

    private fun kotlinx.serialization.json.JsonElement.jsonObjectOrArrayFirst(): String {
        return (this as? kotlinx.serialization.json.JsonArray)
            ?.firstOrNull()
            ?.jsonPrimitive
            ?.content
            .orEmpty()
    }

    private suspend fun waitUntil(timeoutMs: Long = 500, condition: () -> Boolean) {
        repeat(timeoutMs.toInt()) {
            if (condition()) {
                return
            }
            delay(1)
        }
        throw AssertionError("condition was not met within ${timeoutMs}ms")
    }
}

private class DelayedConnectWebSocketTransport(
    private val connection: MockWebSocketConnection,
    private val delayMs: Long,
) : WebSocketTransport {
    var connectCount: Int = 0
        private set

    override suspend fun connect(url: String, headers: Map<String, String>): WebSocketConnection {
        connectCount += 1
        delay(delayMs)
        return connection
    }
}

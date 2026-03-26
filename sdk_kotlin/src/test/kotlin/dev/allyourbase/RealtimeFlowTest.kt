package dev.allyourbase

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class RealtimeFlowTest {
    @Test
    fun `subscribeFlow emits realtime events from sse subscription`() = runTest {
        val sse = MockSseTransport()
        sse.enqueue(
            listOf(
                "event: connected",
                "",
                "event: message",
                "data: {\"action\":\"INSERT\",\"table\":\"posts\",\"record\":{\"id\":\"rec_1\"}}",
                "",
            ),
            keepOpen = true,
        )

        val client = AYBClient("https://api.example.com", sseTransport = sse)
        val deferred = CompletableDeferred<RealtimeEvent>()
        val job = client.realtime.subscribeFlow(listOf("posts"))
            .onEach { event -> deferred.complete(event) }
            .launchIn(this)

        val event = deferred.await()

        assertEquals("INSERT", event.action)
        assertEquals("posts", event.table)
        assertEquals("rec_1", event.record["id"]?.jsonPrimitive?.content)
        job.cancelAndJoin()
        client.realtime.close()
    }

    @Test
    fun `subscribeWSFlow emits websocket table events`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
            enqueueReceive("{\"type\":\"reply\",\"ref\":\"r1\",\"status\":\"ok\"}")
            enqueueReceive("{\"type\":\"event\",\"table\":\"posts\",\"action\":\"UPDATE\",\"record\":{\"id\":\"rec_1\"}}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)
        val deferred = CompletableDeferred<RealtimeEvent>()
        val job = client.realtime.subscribeWSFlow(listOf("posts"))
            .onEach { event -> deferred.complete(event) }
            .launchIn(this)

        val event = deferred.await()

        assertEquals("UPDATE", event.action)
        assertEquals("posts", event.table)
        job.cancelAndJoin()
        client.realtime.close()
    }

    @Test
    @OptIn(ExperimentalCoroutinesApi::class)
    fun `broadcastFlow emits broadcast channel payloads`() = runTest {
        val wsConnection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
        }
        val wsTransport = MockWebSocketTransport().apply { enqueue(wsConnection) }
        val client = AYBClient("https://api.example.com", wsTransport = wsTransport)

        client.realtime.connectWebSocket()
        val flow = client.realtime.broadcastFlow("room-1")
        val deferred = CompletableDeferred<Pair<String, kotlinx.serialization.json.JsonObject>>()
        val job = flow.onEach { payload -> deferred.complete(payload) }.launchIn(this)
        runCurrent()
        wsConnection.enqueueReceive("{\"type\":\"broadcast\",\"channel\":\"room-1\",\"event\":\"updated\",\"payload\":{\"id\":\"p1\"}}")
        val (event, payload) = deferred.await()

        assertEquals("updated", event)
        assertEquals("p1", payload["id"]?.jsonPrimitive?.content)
        job.cancelAndJoin()
        client.realtime.close()
    }
}

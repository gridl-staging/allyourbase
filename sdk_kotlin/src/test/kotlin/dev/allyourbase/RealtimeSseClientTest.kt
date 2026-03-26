package dev.allyourbase

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.net.URI
import java.net.URLDecoder
import java.nio.charset.StandardCharsets
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds

class RealtimeSseClientTest {
    @Test
    fun `subscribe builds url with tables token filter and delivers events`() = runTest {
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

        val client = AYBClient(
            baseURL = "https://api.example.com",
            sseTransport = sse,
        )
        client.setTokens("jwt_1", "refresh_1")

        val received = CompletableDeferred<RealtimeEvent>()
        val unsubscribe = client.realtime.subscribe(listOf("posts", "comments"), filter = "status='pub'") { event ->
            received.complete(event)
        }

        val event = received.await()
        unsubscribe()

        assertEquals("INSERT", event.action)
        assertEquals("posts", event.table)
        assertEquals("rec_1", event.record["id"]?.jsonPrimitive?.content)

        val request = sse.requests.first()
        val uri = URI(request.url)
        val query = uri.query.split("&").associate { token ->
            val parts = token.split("=", limit = 2)
            URLDecoder.decode(parts[0], StandardCharsets.UTF_8) to URLDecoder.decode(parts.getOrElse(1) { "" }, StandardCharsets.UTF_8)
        }

        assertEquals("posts,comments", query["tables"])
        assertEquals("jwt_1", query["token"])
        assertEquals("status='pub'", query["filter"])
        assertEquals("text/event-stream", request.headers["Accept"])
        assertEquals("no-cache", request.headers["Cache-Control"])
    }

    @Test
    fun `connected and malformed data events are ignored`() = runTest {
        val sse = MockSseTransport()
        sse.enqueue(
            listOf(
                "event: connected",
                "",
                "event: message",
                "data: not-json",
                "",
                "event: message",
                "data: {\"action\":\"UPDATE\",\"table\":\"posts\",\"record\":{\"id\":\"rec_1\"}}",
                "",
            ),
            keepOpen = true,
        )

        val client = AYBClient("https://api.example.com", sseTransport = sse)
        val received = mutableListOf<RealtimeEvent>()
        val unsubscribe = client.realtime.subscribe(listOf("posts")) { event ->
            received.add(event)
        }

        while (received.isEmpty()) {
            delay(1)
        }
        unsubscribe()

        assertEquals(1, received.size)
        assertEquals("UPDATE", received[0].action)
    }

    @Test
    fun `unsubscribe cancels active sse connection`() = runTest {
        val sse = MockSseTransport()
        sse.enqueue(emptyList(), keepOpen = true)

        val client = AYBClient("https://api.example.com", sseTransport = sse)
        val unsubscribe = client.realtime.subscribe(listOf("posts")) { }

        while (sse.connections.isEmpty()) {
            delay(1)
        }
        assertFalse(sse.connections.first().isCancelled)

        unsubscribe()

        while (!sse.connections.first().isCancelled) {
            delay(1)
        }
        assertTrue(sse.connections.first().isCancelled)
    }

    @Test
    fun `reconnection uses stepped delays and jitter`() = runTest {
        val sse = MockSseTransport()
        sse.enqueue(emptyList())
        sse.enqueue(emptyList())
        sse.enqueue(
            listOf(
                "event: message",
                "data: {\"action\":\"INSERT\",\"table\":\"posts\",\"record\":{\"id\":\"rec_1\"}}",
                "",
            ),
            keepOpen = true,
        )

        val slept = mutableListOf<Duration>()
        val client = AYBClient("https://api.example.com", sseTransport = sse)
        val realtime = RealtimeClient(
            client = client,
            options = RealtimeOptions(
                maxReconnectAttempts = 5,
                reconnectDelays = listOf(100.milliseconds, 200.milliseconds),
                jitterMax = 50.milliseconds,
            ),
            jitterProvider = { 50.milliseconds },
            sleepFn = { duration -> slept.add(duration) },
        )

        val received = CompletableDeferred<RealtimeEvent>()
        val unsubscribe = realtime.subscribe(listOf("posts")) { event ->
            received.complete(event)
        }

        received.await()
        unsubscribe()

        assertEquals(3, sse.requests.size)
        assertEquals(listOf(150.milliseconds, 250.milliseconds), slept)
    }

    @Test
    fun `auth failure stops reconnection`() = runTest {
        val sse = MockSseTransport()
        sse.enqueue(AYBException(status = 401, message = "unauthorized", code = "auth/unauthorized"))

        val slept = mutableListOf<Duration>()
        val realtime = RealtimeClient(
            client = AYBClient("https://api.example.com", sseTransport = sse),
            jitterProvider = { 0.milliseconds },
            sleepFn = { duration -> slept.add(duration) },
        )

        val unsubscribe = realtime.subscribe(listOf("posts")) { }

        while (sse.requests.isEmpty()) {
            delay(1)
        }
        delay(10)
        unsubscribe()

        assertEquals(1, sse.requests.size)
        assertTrue(slept.isEmpty())
    }

    @Test
    fun `reconnect request omits token when none present`() = runTest {
        val sse = MockSseTransport()
        sse.enqueue(emptyList(), keepOpen = true)

        val client = AYBClient("https://api.example.com", sseTransport = sse)
        val unsubscribe = client.realtime.subscribe(listOf("posts")) { }

        while (sse.requests.isEmpty()) {
            delay(1)
        }

        val query = URI(sse.requests.first().url).query
            .split("&")
            .associate {
                val parts = it.split("=", limit = 2)
                URLDecoder.decode(parts[0], StandardCharsets.UTF_8) to URLDecoder.decode(parts.getOrElse(1) { "" }, StandardCharsets.UTF_8)
            }

        unsubscribe()

        assertEquals("posts", query["tables"])
        assertNull(query["token"])
    }
}

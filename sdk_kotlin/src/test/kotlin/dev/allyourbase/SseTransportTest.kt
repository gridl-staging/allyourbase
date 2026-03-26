package dev.allyourbase

import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class SseTransportTest {
    @Test
    fun `mock sse transport enqueues line sequences and captures requests`() = runTest {
        val transport = MockSseTransport()
        transport.enqueue(listOf("event: connected", "", "data: one", ""))

        val connection = transport.connect(
            HttpRequest(
                url = "https://api.example.com/api/realtime?tables=posts",
                method = HttpMethod.GET,
                headers = mapOf("Accept" to "text/event-stream"),
                body = null,
            ),
        )

        val lines = connection.lines().toList()
        assertEquals(listOf("event: connected", "", "data: one", ""), lines)
        assertEquals(1, transport.requests.size)
        assertEquals("https://api.example.com/api/realtime?tables=posts", transport.requests.first().url)
    }

    @Test
    fun `mock sse transport can enqueue failures`() = runTest {
        val transport = MockSseTransport()
        transport.enqueue(AYBException(status = 401, message = "unauthorized", code = "auth/unauthorized"))

        runCatching {
            transport.connect(
                HttpRequest(
                    url = "https://api.example.com/api/realtime?tables=posts",
                    method = HttpMethod.GET,
                    headers = emptyMap(),
                    body = null,
                ),
            )
        }.onSuccess {
            throw AssertionError("expected failure")
        }.onFailure { error ->
            val ayb = error as AYBException
            assertEquals(401, ayb.status)
            assertEquals("auth/unauthorized", ayb.code)
        }
    }

    @Test
    fun `mock sse connection cancel stops line emission`() = runTest {
        val connection = MockSseConnection(listOf("a", "b", "c"))
        connection.cancel()

        val lines = connection.lines().toList()
        assertTrue(lines.isEmpty())
    }
}

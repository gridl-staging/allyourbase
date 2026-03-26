package dev.allyourbase

import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class WebSocketTransportTest {
    @Test
    fun `mock websocket transport captures connect request and returns queued connection`() = runTest {
        val transport = MockWebSocketTransport()
        val connection = MockWebSocketConnection().apply {
            enqueueReceive("{\"type\":\"connected\"}")
        }
        transport.enqueue(connection)

        val connected = transport.connect(
            url = "wss://api.example.com/api/realtime/ws?token=jwt",
            headers = mapOf("Authorization" to "Bearer jwt"),
        ) as MockWebSocketConnection

        assertEquals(1, transport.connectRequests.size)
        assertEquals("wss://api.example.com/api/realtime/ws?token=jwt", transport.connectRequests.first().url)
        assertEquals("Bearer jwt", transport.connectRequests.first().headers["Authorization"])
        assertEquals("{\"type\":\"connected\"}", connected.receive())
    }

    @Test
    fun `mock websocket connection scripts send receive ping and close`() = runTest {
        val connection = MockWebSocketConnection()
        connection.enqueueReceive("first")
        connection.enqueueReceive("second")

        connection.send("hello")
        connection.send("world")

        assertEquals(listOf("hello", "world"), connection.sentTexts)
        assertEquals("first", connection.receive())
        assertEquals("second", connection.receive())

        connection.ping()
        assertEquals(1, connection.pingCount)

        connection.close()
        assertTrue(connection.isClosed)
    }
}

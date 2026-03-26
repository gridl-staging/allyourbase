package dev.allyourbase

import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

class RealtimeModelTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `realtime event decodes old_record alias`() {
        val payload = """
            {
              "action": "UPDATE",
              "table": "posts",
              "record": {"id": "rec_1"},
              "old_record": {"id": "rec_1", "title": "before"}
            }
        """.trimIndent()

        val decoded = json.decodeFromString<RealtimeEvent>(payload)

        assertEquals("UPDATE", decoded.action)
        assertEquals("posts", decoded.table)
        assertEquals("rec_1", decoded.record["id"]?.toString()?.trim('"'))
        assertEquals("before", decoded.oldRecord?.get("title")?.toString()?.trim('"'))
    }

    @Test
    fun `realtime event allows null old record`() {
        val payload = """
            {
              "action": "INSERT",
              "table": "posts",
              "record": {"id": "rec_1"}
            }
        """.trimIndent()

        val decoded = json.decodeFromString<RealtimeEvent>(payload)
        assertNull(decoded.oldRecord)
    }

    @Test
    fun `realtime options clamp negatives and default empty delays`() {
        val options = RealtimeOptions(
            maxReconnectAttempts = -1,
            reconnectDelays = emptyList(),
            jitterMax = (-10).milliseconds,
        )

        assertEquals(0, options.maxReconnectAttempts)
        assertEquals(listOf(250.milliseconds, 500.milliseconds, 1.seconds, 2.seconds, 4.seconds), options.reconnectDelays)
        assertEquals(0.milliseconds, options.jitterMax)
    }

    @Test
    fun `realtime options clamp negative reconnect delays`() {
        val options = RealtimeOptions(
            reconnectDelays = listOf((-1).milliseconds, 100.milliseconds),
        )

        assertEquals(0.milliseconds, options.reconnectDelays[0])
        assertEquals(100.milliseconds, options.reconnectDelays[1])
        assertTrue(options.reconnectDelays.isNotEmpty())
    }
}

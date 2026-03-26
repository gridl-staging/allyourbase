package dev.allyourbase

import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class SseParserTest {
    @Test
    fun `parser ignores comments and malformed lines`() = runTest {
        val messages = SseParser.parse(
            flowOf(
                ":comment",
                "garbage",
                "event: connected",
                "data: ready",
                "",
            ),
        ).toList()

        assertEquals(1, messages.size)
        assertEquals("connected", messages[0].event)
        assertEquals("ready", messages[0].data)
    }

    @Test
    fun `parser concatenates multi-line data with newline`() = runTest {
        val messages = SseParser.parse(
            flowOf(
                "event: message",
                "data: one",
                "data: two",
                "id: 5",
                "retry: 1000",
                "",
            ),
        ).toList()

        assertEquals(1, messages.size)
        assertEquals("message", messages[0].event)
        assertEquals("one\ntwo", messages[0].data)
        assertEquals("5", messages[0].id)
        assertEquals("1000", messages[0].retry)
    }

    @Test
    fun `parser strips one leading space from field value`() = runTest {
        val messages = SseParser.parse(
            flowOf(
                "event:  update",
                "data:  {\"ok\":true}",
                "",
            ),
        ).toList()

        assertEquals(" update", messages[0].event)
        assertEquals(" {\"ok\":true}", messages[0].data)
    }

    @Test
    fun `parser flushes trailing message on stream completion`() = runTest {
        val messages = SseParser.parse(
            flowOf(
                "event: message",
                "data: trailing",
            ),
        ).toList()

        assertEquals(1, messages.size)
        assertEquals("message", messages[0].event)
        assertEquals("trailing", messages[0].data)
    }
}

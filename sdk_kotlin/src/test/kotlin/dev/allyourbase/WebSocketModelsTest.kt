package dev.allyourbase

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class WebSocketModelsTest {
    @Test
    fun `client message serializes expected keys and omits null empty`() {
        val message = WebSocketClientMessage(
            type = WebSocketClientMessageType.BROADCAST,
            channel = "room:1",
            event = "new_message",
            payload = buildJsonObject { put("id", "m1") },
            selfBroadcast = true,
        )

        val encoded = message.toJsonObject()

        assertEquals("broadcast", encoded["type"]?.jsonPrimitive?.content)
        assertEquals("room:1", encoded["channel"]?.jsonPrimitive?.content)
        assertEquals("new_message", encoded["event"]?.jsonPrimitive?.content)
        assertEquals("m1", encoded["payload"]?.jsonObject?.get("id")?.jsonPrimitive?.content)
        assertEquals("true", encoded["self"]?.jsonPrimitive?.content)
        assertFalse(encoded.containsKey("tables"))
        assertFalse(encoded.containsKey("filter"))
    }

    @Test
    fun `server message parses fallback alias keys`() {
        val input = buildJsonObject {
            put("type", "event")
            put("client_id", "c1")
            put("action", "UPDATE")
            put("table", "posts")
            putJsonObject("record") { put("id", "rec_1") }
            putJsonObject("old_record") { put("id", "rec_1") }
            put("presence_action", "sync")
            put("presence_conn_id", "conn_1")
        }

        val parsed = WebSocketServerMessage.from(input)

        assertEquals(WebSocketServerMessageType.EVENT, parsed.type)
        assertEquals("c1", parsed.clientId)
        assertEquals("UPDATE", parsed.action)
        assertEquals("posts", parsed.table)
        assertEquals("rec_1", parsed.record?.get("id")?.jsonPrimitive?.content)
        assertEquals("rec_1", parsed.oldRecord?.get("id")?.jsonPrimitive?.content)
        assertEquals("sync", parsed.presenceAction)
        assertEquals("conn_1", parsed.presenceConnId)
    }

    @Test
    fun `server message rejects unknown type`() {
        val input = buildJsonObject {
            put("type", "unknown")
        }

        runCatching { WebSocketServerMessage.from(input) }
            .onSuccess { throw AssertionError("expected failure") }
            .onFailure { error ->
                val ayb = error as AYBException
                assertEquals(500, ayb.status)
                assertTrue(ayb.message.contains("unknown websocket message type"))
            }
    }
}

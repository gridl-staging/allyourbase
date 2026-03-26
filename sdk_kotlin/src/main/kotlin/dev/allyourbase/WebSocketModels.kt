package dev.allyourbase

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

enum class WebSocketClientMessageType(val wireValue: String) {
    AUTH("auth"),
    SUBSCRIBE("subscribe"),
    UNSUBSCRIBE("unsubscribe"),
    CHANNEL_SUBSCRIBE("channel_subscribe"),
    CHANNEL_UNSUBSCRIBE("channel_unsubscribe"),
    BROADCAST("broadcast"),
    PRESENCE_TRACK("presence_track"),
    PRESENCE_UNTRACK("presence_untrack"),
    PRESENCE_SYNC("presence_sync"),
}

enum class WebSocketServerMessageType(val wireValue: String) {
    CONNECTED("connected"),
    REPLY("reply"),
    EVENT("event"),
    BROADCAST("broadcast"),
    PRESENCE("presence"),
    ERROR("error"),
    SYSTEM("system"),
}

data class WebSocketClientMessage(
    val type: WebSocketClientMessageType,
    val ref: String? = null,
    val tables: List<String> = emptyList(),
    val filter: String? = null,
    val channel: String? = null,
    val event: String? = null,
    val payload: JsonObject? = null,
    val selfBroadcast: Boolean? = null,
    val state: JsonObject? = null,
) {
    fun toJsonObject(): JsonObject = buildJsonObject {
        put("type", type.wireValue)
        ref?.let { put("ref", it) }
        if (tables.isNotEmpty()) {
            put("tables", buildJsonArray { tables.forEach { add(JsonPrimitive(it)) } })
        }
        filter?.let { put("filter", it) }
        channel?.let { put("channel", it) }
        event?.let { put("event", it) }
        payload?.let { put("payload", it) }
        selfBroadcast?.let { put("self", it) }
        state?.let { put("state", it) }
    }
}

data class WebSocketServerMessage(
    val type: WebSocketServerMessageType,
    val ref: String? = null,
    val status: String? = null,
    val message: String? = null,
    val action: String? = null,
    val table: String? = null,
    val record: JsonObject? = null,
    val oldRecord: JsonObject? = null,
    val channel: String? = null,
    val event: String? = null,
    val payload: JsonObject? = null,
    val presenceAction: String? = null,
    val presenceConnId: String? = null,
    val presences: Map<String, JsonObject>? = null,
    val clientId: String? = null,
) {
    companion object {
        fun from(json: JsonObject): WebSocketServerMessage {
            val typeRaw = json.stringOrNull("type")
                ?: throw AYBException(status = 500, message = "websocket message missing type")
            val type = WebSocketServerMessageType.entries.firstOrNull { it.wireValue == typeRaw }
                ?: throw AYBException(status = 500, message = "unknown websocket message type: $typeRaw")

            return WebSocketServerMessage(
                type = type,
                ref = json.stringOrNull("ref"),
                status = json.stringOrNull("status"),
                message = json.stringOrNull("message"),
                action = json.stringOrNull("action"),
                table = json.stringOrNull("table"),
                record = json.objectOrNull("record"),
                oldRecord = json.objectOrNull("oldRecord") ?: json.objectOrNull("old_record"),
                channel = json.stringOrNull("channel"),
                event = json.stringOrNull("event"),
                payload = json.objectOrNull("payload"),
                presenceAction = json.stringOrNull("presenceAction") ?: json.stringOrNull("presence_action"),
                presenceConnId = json.stringOrNull("presenceConnId") ?: json.stringOrNull("presence_conn_id"),
                presences = json.objectOrNull("presences")?.entries?.mapNotNull { (key, value) ->
                    (value as? JsonObject)?.let { key to it }
                }?.toMap(),
                clientId = json.stringOrNull("clientId") ?: json.stringOrNull("client_id"),
            )
        }
    }
}

private fun JsonObject.stringOrNull(key: String): String? {
    val primitive = this[key] as? JsonPrimitive ?: return null
    if (!primitive.isString && primitive.content == "null") {
        return null
    }
    return primitive.content
}

private fun JsonObject.objectOrNull(key: String): JsonObject? =
    this[key] as? JsonObject

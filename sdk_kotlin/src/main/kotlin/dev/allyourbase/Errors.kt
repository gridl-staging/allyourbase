package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

class AYBException(
    val status: Int,
    override val message: String,
    val code: String? = null,
    val data: JsonObject? = null,
    val docUrl: String? = null,
) : Exception(message) {
    companion object {
        private val json = Json { ignoreUnknownKeys = true }

        fun from(response: HttpResponse): AYBException {
            val parsed = parseObject(response.body)
            val message = parsed?.coercedStringValue("message") ?: response.statusText
            val code = parsed?.coercedStringValue("code")
            val data = parsed?.get("data") as? JsonObject
            val docUrl = parsed?.coercedStringValue("doc_url") ?: parsed?.coercedStringValue("docUrl")

            return AYBException(
                status = response.statusCode,
                message = message,
                code = code,
                data = data,
                docUrl = docUrl,
            )
        }

        private fun parseObject(body: ByteArray): JsonObject? {
            if (body.isEmpty()) {
                return null
            }
            val payload = body.decodeToString()
            return runCatching {
                json.parseToJsonElement(payload).jsonObject
            }.getOrNull()
        }

        private fun JsonObject.coercedStringValue(key: String): String? {
            val primitive = this[key] as? JsonPrimitive ?: return null
            if (!primitive.isString && primitive.content == "null") {
                return null
            }
            return primitive.content
        }
    }
}

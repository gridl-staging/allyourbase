package dev.allyourbase

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

data class ListParams(
    val page: Int? = null,
    val perPage: Int? = null,
    val sort: String? = null,
    val filter: String? = null,
    val search: String? = null,
    val fields: String? = null,
    val expand: String? = null,
    val skipTotal: Boolean? = null,
) {
    fun toQueryItems(): List<Pair<String, String>> {
        val items = mutableListOf<Pair<String, String>>()
        page?.let { items += "page" to it.toString() }
        perPage?.let { items += "perPage" to it.toString() }
        sort?.let { items += "sort" to it }
        filter?.let { items += "filter" to it }
        search?.let { items += "search" to it }
        fields?.let { items += "fields" to it }
        expand?.let { items += "expand" to it }
        if (skipTotal == true) {
            items += "skipTotal" to "true"
        }
        return items
    }
}

data class GetParams(
    val fields: String? = null,
    val expand: String? = null,
) {
    fun toQueryItems(): List<Pair<String, String>> {
        val items = mutableListOf<Pair<String, String>>()
        fields?.let { items += "fields" to it }
        expand?.let { items += "expand" to it }
        return items
    }
}

data class ListMetadata(
    val page: Int,
    val perPage: Int,
    val totalItems: Int,
    val totalPages: Int,
)

data class ListResponse<T>(
    val items: List<T>,
    val metadata: ListMetadata,
) {
    companion object {
        fun <T> decode(
            json: JsonElement?,
            decodeItem: (JsonObject) -> T,
        ): ListResponse<T> {
            val obj = json as? JsonObject
                ?: throw AYBException(status = 500, message = "ListResponse expected object")
            val itemsRaw = obj["items"] as? JsonArray
                ?: throw AYBException(status = 500, message = "ListResponse missing items")

            val items = itemsRaw.map { raw ->
                val item = raw as? JsonObject
                    ?: throw AYBException(status = 500, message = "ListResponse item expected object")
                decodeItem(item)
            }

            return ListResponse(
                items = items,
                metadata = ListMetadata(
                    page = obj.requiredInt("page"),
                    perPage = obj.requiredInt("perPage"),
                    totalItems = obj.requiredInt("totalItems"),
                    totalPages = obj.requiredInt("totalPages"),
                ),
            )
        }
    }
}

data class BatchOperation(
    val method: String,
    val id: String? = null,
    val body: JsonObject? = null,
) {
    fun toDictionary(): JsonObject = buildJsonObject {
        put("method", method)
        id?.let { put("id", it) }
        body?.let { put("body", it) }
    }
}

data class BatchResult<T>(
    val index: Int,
    val status: Int,
    val body: T?,
) {
    companion object {
        fun <T> decode(
            json: JsonElement,
            decodeBody: (JsonObject?) -> T?,
        ): BatchResult<T> {
            val obj = json as? JsonObject
                ?: throw AYBException(status = 500, message = "BatchResult expected object")
            val body = (obj["body"] as? JsonObject)
            return BatchResult(
                index = obj.requiredInt("index"),
                status = obj.requiredInt("status"),
                body = decodeBody(body),
            )
        }
    }
}

private fun JsonObject.requiredInt(key: String): Int {
    val primitive = this[key]?.jsonPrimitive ?: throw AYBException(status = 500, message = "missing $key")
    return primitive.intOrNull ?: primitive.content.toIntOrNull()
    ?: throw AYBException(status = 500, message = "invalid int for $key")
}

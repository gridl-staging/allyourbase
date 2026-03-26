package dev.allyourbase

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

class RecordsClient internal constructor(
    private val client: AYBClient,
) {
    suspend fun list(
        collection: String,
        params: ListParams? = null,
    ): ListResponse<JsonObject> {
        val queryItems = params?.toQueryItems().orEmpty()
        return client.request(
            path = "/api/collections/$collection",
            method = HttpMethod.GET,
            queryItems = queryItems,
            decode = { payload -> ListResponse.decode(payload) { it } },
        )
    }

    suspend fun get(
        collection: String,
        id: String,
        params: GetParams? = null,
    ): JsonObject {
        val queryItems = params?.toQueryItems().orEmpty()
        return client.request(
            path = "/api/collections/$collection/$id",
            method = HttpMethod.GET,
            queryItems = queryItems,
            decode = { payload -> payload.requireObject("records.get") },
        )
    }

    suspend fun create(collection: String, data: JsonObject): JsonObject =
        client.request(
            path = "/api/collections/$collection",
            method = HttpMethod.POST,
            body = data,
            decode = { payload -> payload.requireObject("records.create") },
        )

    suspend fun update(collection: String, id: String, data: JsonObject): JsonObject =
        client.request(
            path = "/api/collections/$collection/$id",
            method = HttpMethod.PATCH,
            body = data,
            decode = { payload -> payload.requireObject("records.update") },
        )

    suspend fun delete(collection: String, id: String) {
        client.request(
            path = "/api/collections/$collection/$id",
            method = HttpMethod.DELETE,
            decode = { Unit },
        )
    }

    suspend fun batch(
        collection: String,
        operations: List<BatchOperation>,
    ): List<BatchResult<JsonObject>> {
        val payload = buildJsonObject {
            put(
                "operations",
                buildJsonArray {
                    operations.forEach { operation -> add(operation.toDictionary()) }
                },
            )
        }

        return client.request(
            path = "/api/collections/$collection/batch",
            method = HttpMethod.POST,
            body = payload,
            decode = { raw ->
                val items = raw as? JsonArray
                    ?: throw AYBException(status = 500, message = "records.batch expected array")
                items.map { item ->
                    BatchResult.decode(item) { body -> body }
                }
            },
        )
    }
}

private fun JsonElement?.requireObject(context: String): JsonObject {
    return this as? JsonObject
        ?: throw AYBException(status = 500, message = "$context expected object")
}

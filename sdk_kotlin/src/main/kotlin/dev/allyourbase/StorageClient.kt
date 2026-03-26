package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.serializer

class StorageClient internal constructor(
    private val client: AYBClient,
) {
    private val json = Json { ignoreUnknownKeys = true }

    suspend fun upload(
        bucket: String,
        data: ByteArray,
        name: String? = null,
        contentType: String? = null,
    ): StorageObject {
        val multipart = MultipartBody.build(
            fieldName = "file",
            data = data,
            filename = name,
            contentType = contentType,
        )

        return client.request(
            path = "/api/storage/$bucket",
            method = HttpMethod.POST,
            rawBody = multipart.body,
            rawContentType = multipart.contentType,
            decode = { payload -> decodePayload(payload) },
        )
    }

    fun downloadUrl(bucket: String, name: String): String =
        "${client.configuration.baseURL}/api/storage/$bucket/$name"

    suspend fun delete(bucket: String, name: String) {
        client.request(
            path = "/api/storage/$bucket/$name",
            method = HttpMethod.DELETE,
            decode = { Unit },
        )
    }

    suspend fun list(
        bucket: String,
        prefix: String? = null,
        limit: Int? = null,
        offset: Int? = null,
    ): StorageListResponse {
        val query = linkedMapOf<String, String>()
        if (!prefix.isNullOrEmpty()) {
            query["prefix"] = prefix
        }
        if (limit != null) {
            query["limit"] = limit.toString()
        }
        if (offset != null) {
            query["offset"] = offset.toString()
        }

        return client.request(
            path = "/api/storage/$bucket",
            method = HttpMethod.GET,
            query = query,
            decode = { payload -> decodePayload(payload) },
        )
    }

    suspend fun getSignedUrl(bucket: String, name: String, expiresIn: Int = 3600): String {
        val relative = client.request(
            path = "/api/storage/$bucket/$name/sign",
            method = HttpMethod.POST,
            body = buildJsonObject { put("expiresIn", expiresIn) },
            decode = { payload ->
                val obj = payload as? kotlinx.serialization.json.JsonObject
                    ?: throw AYBException(status = 500, message = "storage.sign expected object")
                obj["url"]?.jsonPrimitive?.content
                    ?: throw AYBException(status = 500, message = "storage.sign missing url")
            },
        )

        return if (relative.startsWith("/")) {
            "${client.configuration.baseURL}$relative"
        } else {
            relative
        }
    }

    private inline fun <reified T> decodePayload(payload: JsonElement?): T {
        if (payload == null) {
            throw AYBException(status = 500, message = "Empty response payload")
        }
        return json.decodeFromJsonElement(serializer<T>(), payload)
    }
}

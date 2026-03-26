package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

class RequestBuilder(
    private val baseURL: String,
) {
    private val json = Json { encodeDefaults = false }

    fun buildUrl(
        path: String,
        query: Map<String, String> = emptyMap(),
        queryItems: List<Pair<String, String>> = emptyList(),
    ): String {
        val normalizedPath = when {
            path.isEmpty() -> "/"
            path.startsWith("/") -> path
            else -> "/$path"
        }

        val base = baseURL.trimEnd('/')
        val url = "$base$normalizedPath"

        val pairs = if (queryItems.isNotEmpty()) {
            queryItems
        } else {
            query.toSortedMap().map { (name, value) -> name to value }
        }

        if (pairs.isEmpty()) {
            return url
        }

        val queryString = pairs.joinToString("&") { (name, value) ->
            "${name.urlEncode()}=${value.urlEncode()}"
        }
        return "$url?$queryString"
    }

    fun buildRequest(
        path: String,
        method: HttpMethod,
        query: Map<String, String> = emptyMap(),
        queryItems: List<Pair<String, String>> = emptyList(),
        headers: Map<String, String> = emptyMap(),
        body: JsonElement? = null,
        rawBody: ByteArray? = null,
        rawContentType: String? = null,
        bearerToken: String? = null,
    ): HttpRequest {
        val url = buildUrl(path = path, query = query, queryItems = queryItems)
        val mergedHeaders = linkedMapOf<String, String>()
        headers.forEach { (name, value) -> mergedHeaders[name] = value }
        mergedHeaders.putIfMissingHeader("Accept", "application/json")

        if (!bearerToken.isNullOrEmpty()) {
            mergedHeaders.putHeader("Authorization", "Bearer $bearerToken")
        }

        val encodedBody = rawBody ?: body?.let {
            mergedHeaders.putIfMissingHeader("Content-Type", "application/json")
            json.encodeToString(JsonElement.serializer(), it).encodeToByteArray()
        }
        if (rawBody != null && !rawContentType.isNullOrEmpty()) {
            mergedHeaders.putHeader("Content-Type", rawContentType)
        }

        return HttpRequest(
            url = url,
            method = method,
            headers = mergedHeaders,
            body = encodedBody,
        )
    }
}

private fun String.urlEncode(): String = URLEncoder.encode(this, StandardCharsets.UTF_8)

private fun MutableMap<String, String>.putIfMissingHeader(name: String, value: String) {
    if (!containsHeader(name)) {
        this[name] = value
    }
}

private fun MutableMap<String, String>.putHeader(name: String, value: String) {
    val existing = keys.firstOrNull { it.equals(name, ignoreCase = true) }
    if (existing != null) {
        remove(existing)
    }
    this[name] = value
}

private fun Map<String, String>.containsHeader(name: String): Boolean =
    keys.any { it.equals(name, ignoreCase = true) }

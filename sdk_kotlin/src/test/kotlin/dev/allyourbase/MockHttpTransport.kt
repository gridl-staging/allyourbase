package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

private val testJson = Json { encodeDefaults = false }

data class StubResponse(
    val status: Int,
    val statusText: String = when (status) {
        in 200..299 -> "OK"
        400 -> "Bad Request"
        401 -> "Unauthorized"
        403 -> "Forbidden"
        404 -> "Not Found"
        else -> "Error"
    },
    val headers: Map<String, String> = emptyMap(),
    val body: ByteArray? = null,
    val json: JsonElement? = null,
) {
    fun toHttpResponse(): HttpResponse {
        val payload = body ?: json?.let(::jsonToBytes) ?: ByteArray(0)
        return HttpResponse(
            statusCode = status,
            statusText = statusText,
            headers = headers,
            body = payload,
        )
    }
}

sealed class MockTransportBehavior {
    data class Respond(val response: StubResponse) : MockTransportBehavior()
    data class Fail(val error: Throwable) : MockTransportBehavior()
}

class MockHttpTransport : HttpTransport {
    val requests: MutableList<HttpRequest> = mutableListOf()
    private val queue: ArrayDeque<MockTransportBehavior> = ArrayDeque()

    fun enqueue(response: StubResponse) {
        queue.addLast(MockTransportBehavior.Respond(response))
    }

    fun enqueue(error: Throwable) {
        queue.addLast(MockTransportBehavior.Fail(error))
    }

    override suspend fun send(request: HttpRequest): HttpResponse {
        requests.add(request)
        val next = queue.removeFirstOrNull() ?: throw IllegalStateException("MockHttpTransport queue is empty")
        return when (next) {
            is MockTransportBehavior.Respond -> next.response.toHttpResponse()
            is MockTransportBehavior.Fail -> throw next.error
        }
    }
}

fun jsonToBytes(value: JsonElement): ByteArray =
    testJson.encodeToString(JsonElement.serializer(), value).encodeToByteArray()

fun lowercasedLookup(headers: Map<String, String>, key: String): String? =
    headers.entries.firstOrNull { (name, _) -> name.lowercase() == key.lowercase() }?.value

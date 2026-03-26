package dev.allyourbase

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.okhttp.OkHttp
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.request.header
import io.ktor.client.request.request
import io.ktor.client.request.setBody
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

enum class HttpMethod {
    GET,
    POST,
    PATCH,
    DELETE,
}

data class HttpRequest(
    val url: String,
    val method: HttpMethod,
    val headers: Map<String, String>,
    val body: ByteArray?,
)

data class HttpResponse(
    val statusCode: Int,
    val statusText: String,
    val headers: Map<String, String>,
    val body: ByteArray,
)

interface HttpTransport {
    suspend fun send(request: HttpRequest): HttpResponse
}

class KtorHttpTransport(
    timeout: Duration = 30.seconds,
    client: HttpClient? = null,
) : HttpTransport {
    private val client: HttpClient = client ?: HttpClient(OkHttp) {
        install(HttpTimeout) {
            requestTimeoutMillis = timeout.inWholeMilliseconds
            connectTimeoutMillis = timeout.inWholeMilliseconds
            socketTimeoutMillis = timeout.inWholeMilliseconds
        }
    }

    override suspend fun send(request: HttpRequest): HttpResponse {
        val response = client.request(request.url) {
            method = request.method.toKtor()
            request.headers.forEach { (name, value) ->
                header(name, value)
            }
            request.body?.let { setBody(it) }
        }

        val headers = response.headers.entries().associate { (name, values) ->
            name to values.joinToString(",")
        }

        return HttpResponse(
            statusCode = response.status.value,
            statusText = response.status.description,
            headers = headers,
            body = response.body(),
        )
    }
}

private fun HttpMethod.toKtor(): io.ktor.http.HttpMethod = when (this) {
    HttpMethod.GET -> io.ktor.http.HttpMethod.Get
    HttpMethod.POST -> io.ktor.http.HttpMethod.Post
    HttpMethod.PATCH -> io.ktor.http.HttpMethod.Patch
    HttpMethod.DELETE -> io.ktor.http.HttpMethod.Delete
}

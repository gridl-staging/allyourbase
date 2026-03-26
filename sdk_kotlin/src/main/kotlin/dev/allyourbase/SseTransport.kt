package dev.allyourbase

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.util.concurrent.atomic.AtomicBoolean

interface SseConnection {
    fun lines(): Flow<String>
    fun cancel()
}

interface SseTransport {
    suspend fun connect(request: HttpRequest): SseConnection
}

class OkHttpSseTransport(
    private val client: OkHttpClient = OkHttpClient(),
) : SseTransport {
    override suspend fun connect(request: HttpRequest): SseConnection = withContext(Dispatchers.IO) {
        val okRequest = Request.Builder().url(request.url).apply {
            request.headers.forEach { (name, value) ->
                header(name, value)
            }
        }.method(request.method.toOkHttpMethod(), request.body.toOkHttpRequestBody(request.method)).build()

        val call = client.newCall(okRequest)
        val response = call.execute()
        if (!response.isSuccessful) {
            val responseBody = response.body?.bytes() ?: ByteArray(0)
            response.close()
            throw AYBException.from(
                HttpResponse(
                    statusCode = response.code,
                    statusText = response.message,
                    headers = response.headers.toMultimap().mapValues { (_, values) -> values.joinToString(",") },
                    body = responseBody,
                ),
            )
        }

        OkHttpSseConnection(call = call, response = response)
    }
}

class OkHttpSseConnection(
    private val call: Call,
    private val response: Response,
) : SseConnection {
    private val cancelled = AtomicBoolean(false)

    override fun lines(): Flow<String> = flow {
        val source = response.body?.source() ?: return@flow
        try {
            while (!cancelled.get()) {
                val line = source.readUtf8Line() ?: break
                emit(line)
            }
        } catch (error: Throwable) {
            if (!cancelled.get()) {
                throw error
            }
        } finally {
            response.close()
        }
    }.flowOn(Dispatchers.IO)

    override fun cancel() {
        cancelled.set(true)
        call.cancel()
        response.close()
    }
}

private fun HttpMethod.toOkHttpMethod(): String = when (this) {
    HttpMethod.GET -> "GET"
    HttpMethod.POST -> "POST"
    HttpMethod.PATCH -> "PATCH"
    HttpMethod.DELETE -> "DELETE"
}

private fun ByteArray?.toOkHttpRequestBody(method: HttpMethod): RequestBody? {
    if (this == null && (method == HttpMethod.GET || method == HttpMethod.DELETE)) {
        return null
    }
    return (this ?: ByteArray(0)).toRequestBody(null)
}

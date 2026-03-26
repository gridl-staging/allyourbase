package dev.allyourbase

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.channels.Channel
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString

interface WebSocketConnection {
    suspend fun send(text: String)
    suspend fun receive(): String
    suspend fun ping()
    suspend fun close()
}

interface WebSocketTransport {
    suspend fun connect(url: String, headers: Map<String, String>): WebSocketConnection
}

class OkHttpWebSocketTransport(
    private val client: OkHttpClient = OkHttpClient(),
) : WebSocketTransport {
    override suspend fun connect(url: String, headers: Map<String, String>): WebSocketConnection {
        val connected = CompletableDeferred<WebSocket>()
        val incoming = Channel<String>(Channel.UNLIMITED)
        val closeError = CompletableDeferred<Throwable?>()

        val request = Request.Builder().url(url).apply {
            headers.forEach { (name, value) ->
                header(name, value)
            }
        }.build()

        val listener = object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                connected.complete(webSocket)
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                incoming.trySend(text)
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(code, reason)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (!closeError.isCompleted) {
                    closeError.complete(null)
                }
                incoming.close()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (!connected.isCompleted) {
                    connected.completeExceptionally(t)
                }
                if (!closeError.isCompleted) {
                    closeError.complete(t)
                }
                incoming.close(t)
            }
        }

        client.newWebSocket(request, listener)
        val socket = connected.await()
        return OkHttpWebSocketConnection(socket, incoming, closeError)
    }
}

class OkHttpWebSocketConnection(
    private val socket: WebSocket,
    private val incoming: Channel<String>,
    private val closeError: CompletableDeferred<Throwable?>,
) : WebSocketConnection {
    override suspend fun send(text: String) {
        val accepted = socket.send(text)
        if (!accepted) {
            throw AYBException(status = 500, message = "websocket send failed")
        }
    }

    override suspend fun receive(): String {
        val result = incoming.receiveCatching()
        val value = result.getOrNull()
        if (value != null) {
            return value
        }

        val closeThrowable = if (closeError.isCompleted) closeError.await() else null
        throw closeThrowable ?: result.exceptionOrNull() ?: AYBException(status = 500, message = "websocket receive failed")
    }

    override suspend fun ping() {
        val accepted = socket.send(ByteString.EMPTY)
        if (!accepted) {
            throw AYBException(status = 500, message = "websocket ping failed")
        }
    }

    override suspend fun close() {
        socket.close(1000, "normal")
        incoming.close()
    }
}

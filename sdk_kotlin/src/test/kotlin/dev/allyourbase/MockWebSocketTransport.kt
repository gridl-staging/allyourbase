package dev.allyourbase

import kotlinx.coroutines.channels.Channel
import java.util.concurrent.atomic.AtomicBoolean

data class WebSocketConnectRequest(
    val url: String,
    val headers: Map<String, String>,
)

class MockWebSocketTransport : WebSocketTransport {
    val connectRequests: MutableList<WebSocketConnectRequest> = mutableListOf()
    private val queue: ArrayDeque<MockWebSocketConnection> = ArrayDeque()

    fun enqueue(connection: MockWebSocketConnection) {
        queue.addLast(connection)
    }

    override suspend fun connect(url: String, headers: Map<String, String>): WebSocketConnection {
        connectRequests += WebSocketConnectRequest(url = url, headers = headers)
        return queue.removeFirstOrNull() ?: MockWebSocketConnection()
    }
}

sealed class MockWebSocketReceive {
    data class Text(val value: String) : MockWebSocketReceive()
    data class Fail(val error: Throwable) : MockWebSocketReceive()
}

class MockWebSocketConnection : WebSocketConnection {
    val sentTexts: MutableList<String> = mutableListOf()
    var pingCount: Int = 0
        private set

    private val closed = AtomicBoolean(false)
    private val receiveQueue: Channel<MockWebSocketReceive> = Channel(Channel.UNLIMITED)

    fun enqueueReceive(text: String) {
        receiveQueue.trySend(MockWebSocketReceive.Text(text))
    }

    fun enqueueReceiveError(error: Throwable) {
        receiveQueue.trySend(MockWebSocketReceive.Fail(error))
    }

    val isClosed: Boolean
        get() = closed.get()

    override suspend fun send(text: String) {
        if (closed.get()) {
            throw AYBException(status = 500, message = "websocket closed")
        }
        sentTexts += text
    }

    override suspend fun receive(): String {
        val result = receiveQueue.receiveCatching()
        val next = result.getOrNull() ?: throw AYBException(status = 500, message = "websocket closed")
        return when (next) {
            is MockWebSocketReceive.Text -> next.value
            is MockWebSocketReceive.Fail -> throw next.error
        }
    }

    override suspend fun ping() {
        if (closed.get()) {
            throw AYBException(status = 500, message = "websocket closed")
        }
        pingCount += 1
    }

    override suspend fun close() {
        closed.set(true)
        receiveQueue.close()
    }
}

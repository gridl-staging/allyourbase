package dev.allyourbase

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.flow
import java.util.concurrent.atomic.AtomicBoolean

sealed class MockSseBehavior {
    data class Connect(val lines: List<String>, val keepOpen: Boolean) : MockSseBehavior()
    data class Fail(val error: Throwable) : MockSseBehavior()
}

class MockSseTransport : SseTransport {
    val requests: MutableList<HttpRequest> = mutableListOf()
    val connections: MutableList<MockSseConnection> = mutableListOf()
    private val queue: ArrayDeque<MockSseBehavior> = ArrayDeque()

    fun enqueue(lines: List<String>, keepOpen: Boolean = false) {
        queue.addLast(MockSseBehavior.Connect(lines, keepOpen))
    }

    fun enqueue(error: Throwable) {
        queue.addLast(MockSseBehavior.Fail(error))
    }

    override suspend fun connect(request: HttpRequest): SseConnection {
        requests.add(request)
        return when (val next = queue.removeFirstOrNull() ?: throw IllegalStateException("MockSseTransport queue is empty")) {
            is MockSseBehavior.Connect -> MockSseConnection(next.lines, next.keepOpen).also { connections.add(it) }
            is MockSseBehavior.Fail -> throw next.error
        }
    }
}

class MockSseConnection(
    private val sourceLines: List<String>,
    private val keepOpen: Boolean = false,
) : SseConnection {
    private val cancelled = AtomicBoolean(false)
    val isCancelled: Boolean
        get() = cancelled.get()

    override fun lines(): Flow<String> = flow {
        for (line in sourceLines) {
            if (cancelled.get()) {
                break
            }
            emit(line)
        }
        if (keepOpen && !cancelled.get()) {
            try {
                awaitCancellation()
            } catch (_: Throwable) {
                // noop
            }
        }
    }

    override fun cancel() {
        cancelled.set(true)
    }
}

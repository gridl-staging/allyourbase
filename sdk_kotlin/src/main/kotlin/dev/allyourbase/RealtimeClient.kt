package dev.allyourbase

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.isActive
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import java.net.URI
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import kotlin.math.min
import kotlin.random.Random
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

class RealtimeClient internal constructor(
    private val client: AYBClient,
    private val options: RealtimeOptions = RealtimeOptions(),
    private val sseTransport: SseTransport = client.sseTransport,
    private val wsTransport: WebSocketTransport = client.wsTransport,
    private val jitterProvider: (Duration) -> Duration = { max -> defaultJitter(max) },
    private val sleepFn: suspend (Duration) -> Unit = { duration -> delay(duration) },
) {
    private val json = Json { ignoreUnknownKeys = true }
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    private val wsState = WebSocketState()
    private var wsReceiveJob: Job? = null
    private var wsPingJob: Job? = null
    private val wsLifecycleLock = Mutex()

    fun close() {
        runCatching {
            runBlocking {
                disconnectWebSocket()
            }
        }
        scope.cancel()
    }

    fun subscribe(
        tables: List<String>,
        filter: String? = null,
        callback: (RealtimeEvent) -> Unit,
    ): () -> Unit {
        val job = scope.launch {
            var reconnectAttempt = 0

            while (true) {
                val token = client.tokenStore.accessToken()
                val request = HttpRequest(
                    url = buildSseUrl(tables = tables, filter = filter, token = token),
                    method = HttpMethod.GET,
                    headers = mapOf(
                        "Accept" to "text/event-stream",
                        "Cache-Control" to "no-cache",
                    ),
                    body = null,
                )

                var connection: SseConnection? = null
                var sawValidEvent = false
                try {
                    connection = sseTransport.connect(request)
                    SseParser.parse(connection.lines()).collect { message ->
                        if (message.event == "connected") {
                            return@collect
                        }

                        val data = message.data ?: return@collect
                        val event = runCatching {
                            json.decodeFromString<RealtimeEvent>(data)
                        }.getOrNull() ?: return@collect

                        callback(event)
                        if (!sawValidEvent) {
                            sawValidEvent = true
                            reconnectAttempt = 0
                        }
                    }

                    if (!kotlin.coroutines.coroutineContext.isActive) {
                        break
                    }
                    throw IllegalStateException("SSE stream ended")
                } catch (error: Throwable) {
                    if (error is CancellationException) {
                        throw error
                    }

                    val status = (error as? AYBException)?.status
                    if (status == 401 || status == 403) {
                        break
                    }

                    if (reconnectAttempt >= options.maxReconnectAttempts) {
                        break
                    }

                    val baseDelay = options.reconnectDelays[min(reconnectAttempt, options.reconnectDelays.lastIndex)]
                    val jitter = jitterProvider(options.jitterMax).coerceAtLeast(Duration.ZERO)
                    reconnectAttempt += 1
                    sleepFn(baseDelay + jitter)
                } finally {
                    connection?.cancel()
                }
            }
        }

        return {
            job.cancel()
        }
    }

    fun subscribeFlow(tables: List<String>, filter: String? = null): Flow<RealtimeEvent> = callbackFlow {
        val unsubscribe = subscribe(tables, filter) { event -> trySend(event).isSuccess }
        awaitClose { unsubscribe() }
    }

    suspend fun connectWebSocket() {
        wsLifecycleLock.withLock {
            if (wsState.isConnected()) {
                return
            }

            val url = buildWebSocketUrl(client.tokenStore.accessToken())
            val connection = wsTransport.connect(url = url, headers = emptyMap())

            val handshake = parseWebSocketMessage(connection.receive())
            if (handshake.type != WebSocketServerMessageType.CONNECTED) {
                connection.close()
                throw AYBException(status = 500, message = "websocket expected connected handshake")
            }

            wsState.setConnection(connection)

            wsReceiveJob?.cancel()
            wsReceiveJob = scope.launch {
                wsReceiveLoop(connection)
            }

            wsPingJob?.cancel()
            wsPingJob = scope.launch {
                wsPingLoop(connection)
            }
        }
    }

    suspend fun disconnectWebSocket() {
        wsLifecycleLock.withLock {
            wsReceiveJob?.cancel()
            wsReceiveJob = null

            wsPingJob?.cancel()
            wsPingJob = null

            val connection = wsState.connection()
            wsState.clearConnection()
            wsState.failAllPending(AYBException(status = 500, message = "websocket disconnected"))
            wsState.clearAllCallbacksAndSubscriptions()
            connection?.close()
        }
    }

    suspend fun sendAndAwaitReply(message: WebSocketClientMessage): WebSocketServerMessage {
        connectWebSocket()

        val ref = wsState.nextRef()
        val deferred = wsState.registerPendingReply(ref)

        val payload = message.copy(ref = ref).toJsonObject().toString()
        val connection = wsState.connection() ?: throw AYBException(status = 500, message = "websocket is not connected")
        connection.send(payload)

        val reply = deferred.await()
        if (reply.status == "error") {
            throw AYBException(status = 400, message = reply.message ?: "websocket reply error")
        }
        return reply
    }

    suspend fun subscribeWS(
        tables: List<String>,
        filter: String? = null,
        callback: (RealtimeEvent) -> Unit,
    ): () -> Unit {
        val callbackId = wsState.addTableCallback(tables.toSet(), callback)
        try {
            sendAndAwaitReply(
                WebSocketClientMessage(
                    type = WebSocketClientMessageType.SUBSCRIBE,
                    tables = tables,
                    filter = filter,
                ),
            )
        } catch (error: Throwable) {
            wsState.removeTableCallback(callbackId)
            throw error
        }

        return {
            wsState.removeTableCallback(callbackId)
            fireAndForgetSend(
                WebSocketClientMessage(
                    type = WebSocketClientMessageType.UNSUBSCRIBE,
                    tables = tables,
                    filter = filter,
                ),
            )
        }
    }

    suspend fun channelSubscribe(channel: String): () -> Unit {
        sendAndAwaitReply(
            WebSocketClientMessage(
                type = WebSocketClientMessageType.CHANNEL_SUBSCRIBE,
                channel = channel,
            ),
        )
        wsState.addChannelSubscription(channel)

        return {
            wsState.removeChannelSubscription(channel)
            fireAndForgetSend(
                WebSocketClientMessage(
                    type = WebSocketClientMessageType.CHANNEL_UNSUBSCRIBE,
                    channel = channel,
                ),
            )
        }
    }

    suspend fun broadcast(
        channel: String,
        event: String,
        payload: JsonObject,
        includeSelf: Boolean = false,
    ) {
        requireChannelSubscription(channel)
        sendAndAwaitReply(
            WebSocketClientMessage(
                type = WebSocketClientMessageType.BROADCAST,
                channel = channel,
                event = event,
                payload = payload,
                selfBroadcast = includeSelf,
            ),
        )
    }

    fun onBroadcast(channel: String, callback: (String, JsonObject) -> Unit): () -> Unit {
        val id = wsState.addBroadcastCallback(channel, callback)
        return {
            wsState.removeBroadcastCallback(channel, id)
        }
    }

    suspend fun presenceTrack(channel: String, state: JsonObject) {
        requireChannelSubscription(channel)
        sendAndAwaitReply(
            WebSocketClientMessage(
                type = WebSocketClientMessageType.PRESENCE_TRACK,
                channel = channel,
                state = state,
            ),
        )
    }

    suspend fun presenceUntrack(channel: String) {
        requireChannelSubscription(channel)
        sendAndAwaitReply(
            WebSocketClientMessage(
                type = WebSocketClientMessageType.PRESENCE_UNTRACK,
                channel = channel,
            ),
        )
    }

    suspend fun presenceSync(channel: String): Map<String, JsonObject> {
        requireChannelSubscription(channel)
        wsState.cachedPresenceSync(channel)?.let { cached ->
            return cached
        }

        val deferred = wsState.registerPresenceSync(channel)
        sendAndAwaitReply(
            WebSocketClientMessage(
                type = WebSocketClientMessageType.PRESENCE_SYNC,
                channel = channel,
            ),
        )

        return deferred.await()
    }

    fun subscribeWSFlow(tables: List<String>, filter: String? = null): Flow<RealtimeEvent> = callbackFlow {
        var unsubscribe: (() -> Unit)? = null
        val job = scope.launch {
            unsubscribe = subscribeWS(tables, filter) { event -> trySend(event).isSuccess }
        }
        awaitClose {
            unsubscribe?.invoke()
            job.cancel()
        }
    }

    fun broadcastFlow(channel: String): Flow<Pair<String, JsonObject>> = callbackFlow {
        val remove = onBroadcast(channel) { event, payload ->
            trySend(event to payload).isSuccess
        }
        awaitClose { remove() }
    }

    private suspend fun wsReceiveLoop(connection: WebSocketConnection) {
        try {
            while (kotlin.coroutines.coroutineContext.isActive) {
                val message = parseWebSocketMessage(connection.receive())
                when (message.type) {
                    WebSocketServerMessageType.REPLY -> wsState.resolveReply(message)
                    WebSocketServerMessageType.EVENT -> dispatchTableEvent(message)
                    WebSocketServerMessageType.BROADCAST -> dispatchBroadcastEvent(message)
                    WebSocketServerMessageType.PRESENCE -> {
                        if (message.presenceAction == "sync" && !message.channel.isNullOrEmpty()) {
                            wsState.resolvePresenceSync(message.channel, message.presences.orEmpty())
                        }
                    }

                    WebSocketServerMessageType.CONNECTED,
                    WebSocketServerMessageType.ERROR,
                    WebSocketServerMessageType.SYSTEM,
                    -> {
                        // no-op
                    }
                }
            }
        } catch (error: Throwable) {
            if (error !is CancellationException) {
                wsState.failAllPending(AYBException(status = 500, message = "websocket receive loop failed"))
            }
        }
    }

    private suspend fun wsPingLoop(connection: WebSocketConnection) {
        try {
            while (kotlin.coroutines.coroutineContext.isActive) {
                delay(30.seconds)
                connection.ping()
            }
        } catch (_: Throwable) {
            // no-op
        }
    }

    private fun dispatchTableEvent(message: WebSocketServerMessage) {
        val action = message.action ?: return
        val table = message.table ?: return
        val record = message.record ?: return

        val event = RealtimeEvent(
            action = action,
            table = table,
            record = record,
            oldRecord = message.oldRecord,
        )

        wsState.tableCallbacksSnapshot().forEach { (tables, callback) ->
            if (table in tables) {
                callback(event)
            }
        }
    }

    private fun dispatchBroadcastEvent(message: WebSocketServerMessage) {
        val channel = message.channel ?: return
        val event = message.event ?: return
        val payload = message.payload ?: buildJsonObject {}

        wsState.broadcastCallbacksSnapshot(channel).forEach { callback ->
            callback(event, payload)
        }
    }

    private fun requireChannelSubscription(channel: String) {
        if (!wsState.isChannelSubscribed(channel)) {
            throw AYBException(
                status = 400,
                message = "not subscribed to channel",
                code = "realtime/not-subscribed",
            )
        }
    }

    private fun parseWebSocketMessage(raw: String): WebSocketServerMessage {
        val payload = runCatching {
            json.parseToJsonElement(raw) as JsonObject
        }.getOrElse {
            throw AYBException(status = 500, message = "invalid websocket payload")
        }
        return WebSocketServerMessage.from(payload)
    }

    private fun fireAndForgetSend(message: WebSocketClientMessage) {
        scope.launch {
            runCatching {
                connectWebSocket()
                val connection = wsState.connection() ?: return@runCatching
                val ref = wsState.nextRef()
                connection.send(message.copy(ref = ref).toJsonObject().toString())
            }
        }
    }

    private fun buildSseUrl(tables: List<String>, filter: String?, token: String?): String {
        val base = client.configuration.baseURL.trimEnd('/')
        val queryItems = mutableListOf<Pair<String, String>>()
        queryItems += "tables" to tables.joinToString(",")
        if (!token.isNullOrEmpty()) {
            queryItems += "token" to token
        }
        if (!filter.isNullOrEmpty()) {
            queryItems += "filter" to filter
        }

        val query = queryItems.joinToString("&") { (name, value) ->
            "${name.urlEncode()}=${value.urlEncode()}"
        }

        return "$base/api/realtime?$query"
    }

    private fun buildWebSocketUrl(token: String?): String {
        val base = URI(client.configuration.baseURL)
        val scheme = when (base.scheme.lowercase()) {
            "https" -> "wss"
            else -> "ws"
        }
        val basePath = base.path.orEmpty().trimEnd('/')
        val wsPath = if (basePath.isEmpty()) "/api/realtime/ws" else "$basePath/api/realtime/ws"
        val query = if (token.isNullOrEmpty()) null else "token=${token.urlEncode()}"

        return URI(
            scheme,
            base.userInfo,
            base.host,
            base.port,
            wsPath,
            query,
            null,
        ).toString()
    }

    companion object {
        private fun defaultJitter(max: Duration): Duration {
            if (max <= Duration.ZERO) {
                return Duration.ZERO
            }
            val maxMillis = max.inWholeMilliseconds
            if (maxMillis <= 0) {
                return Duration.ZERO
            }
            return Random.nextLong(maxMillis + 1).milliseconds
        }
    }
}

private fun String.urlEncode(): String = URLEncoder.encode(this, StandardCharsets.UTF_8)

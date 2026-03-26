package dev.allyourbase

import kotlinx.coroutines.CompletableDeferred
import kotlinx.serialization.json.JsonObject
import java.util.UUID

class WebSocketState {
    private val lock = Any()

    private var connection: WebSocketConnection? = null
    private var connected: Boolean = false
    private var refCounter: Long = 0

    private val pendingReplies = linkedMapOf<String, CompletableDeferred<WebSocketServerMessage>>()
    private val bufferedReplies = linkedMapOf<String, WebSocketServerMessage>()

    private val tableCallbacks = linkedMapOf<UUID, Pair<Set<String>, (RealtimeEvent) -> Unit>>()
    private val broadcastCallbacks = linkedMapOf<String, LinkedHashMap<UUID, (String, JsonObject) -> Unit>>()
    private val channelSubscriptions = linkedSetOf<String>()

    private val pendingPresenceSync = linkedMapOf<String, CompletableDeferred<Map<String, JsonObject>>>()
    private val lastPresenceSync = linkedMapOf<String, Map<String, JsonObject>>()

    fun setConnection(value: WebSocketConnection?) {
        synchronized(lock) {
            connection = value
            connected = value != null
        }
    }

    fun connection(): WebSocketConnection? = synchronized(lock) { connection }

    fun isConnected(): Boolean = synchronized(lock) { connected }

    fun clearConnection() {
        synchronized(lock) {
            connection = null
            connected = false
        }
    }

    fun nextRef(): String = synchronized(lock) {
        refCounter += 1
        "r$refCounter"
    }

    fun registerPendingReply(ref: String): CompletableDeferred<WebSocketServerMessage> {
        val deferred = CompletableDeferred<WebSocketServerMessage>()
        synchronized(lock) {
            val buffered = bufferedReplies.remove(ref)
            if (buffered != null) {
                deferred.complete(buffered)
            } else {
                pendingReplies[ref] = deferred
            }
        }
        return deferred
    }

    fun bufferReply(message: WebSocketServerMessage) {
        val ref = message.ref ?: return
        synchronized(lock) {
            bufferedReplies[ref] = message
        }
    }

    fun resolveReply(message: WebSocketServerMessage) {
        val ref = message.ref ?: return
        synchronized(lock) {
            val pending = pendingReplies.remove(ref)
            if (pending != null) {
                pending.complete(message)
            } else {
                bufferedReplies[ref] = message
            }
        }
    }

    fun failAllPending(error: Throwable) {
        synchronized(lock) {
            pendingReplies.values.forEach { deferred -> deferred.completeExceptionally(error) }
            pendingReplies.clear()
            bufferedReplies.clear()
            pendingPresenceSync.values.forEach { deferred -> deferred.completeExceptionally(error) }
            pendingPresenceSync.clear()
        }
    }

    fun addTableCallback(tables: Set<String>, callback: (RealtimeEvent) -> Unit): UUID {
        val id = UUID.randomUUID()
        synchronized(lock) {
            tableCallbacks[id] = tables to callback
        }
        return id
    }

    fun removeTableCallback(id: UUID) {
        synchronized(lock) {
            tableCallbacks.remove(id)
        }
    }

    fun tableCallbacksSnapshot(): List<Pair<Set<String>, (RealtimeEvent) -> Unit>> = synchronized(lock) {
        tableCallbacks.values.toList()
    }

    fun addBroadcastCallback(channel: String, callback: (String, JsonObject) -> Unit): UUID {
        val id = UUID.randomUUID()
        synchronized(lock) {
            val channelMap = broadcastCallbacks.getOrPut(channel) { linkedMapOf() }
            channelMap[id] = callback
        }
        return id
    }

    fun removeBroadcastCallback(channel: String, id: UUID) {
        synchronized(lock) {
            val channelMap = broadcastCallbacks[channel] ?: return
            channelMap.remove(id)
            if (channelMap.isEmpty()) {
                broadcastCallbacks.remove(channel)
            }
        }
    }

    fun broadcastCallbacksSnapshot(channel: String): List<(String, JsonObject) -> Unit> = synchronized(lock) {
        broadcastCallbacks[channel]?.values?.toList().orEmpty()
    }

    fun addChannelSubscription(channel: String) {
        synchronized(lock) {
            channelSubscriptions.add(channel)
        }
    }

    fun removeChannelSubscription(channel: String) {
        synchronized(lock) {
            channelSubscriptions.remove(channel)
            lastPresenceSync.remove(channel)
            pendingPresenceSync.remove(channel)
        }
    }

    fun isChannelSubscribed(channel: String): Boolean = synchronized(lock) {
        channelSubscriptions.contains(channel)
    }

    fun registerPresenceSync(channel: String): CompletableDeferred<Map<String, JsonObject>> {
        val deferred = CompletableDeferred<Map<String, JsonObject>>()
        synchronized(lock) {
            pendingPresenceSync[channel] = deferred
        }
        return deferred
    }

    fun resolvePresenceSync(channel: String, presences: Map<String, JsonObject>) {
        synchronized(lock) {
            lastPresenceSync[channel] = presences
            pendingPresenceSync.remove(channel)?.complete(presences)
        }
    }

    fun cachedPresenceSync(channel: String): Map<String, JsonObject>? = synchronized(lock) {
        lastPresenceSync[channel]
    }

    fun clearAllCallbacksAndSubscriptions() {
        synchronized(lock) {
            tableCallbacks.clear()
            broadcastCallbacks.clear()
            channelSubscriptions.clear()
            pendingPresenceSync.clear()
            lastPresenceSync.clear()
        }
    }
}

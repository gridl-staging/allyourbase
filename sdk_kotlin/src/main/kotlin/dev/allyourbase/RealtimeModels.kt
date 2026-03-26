package dev.allyourbase

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonNames
import kotlinx.serialization.json.JsonObject
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

@Serializable
@OptIn(ExperimentalSerializationApi::class)
data class RealtimeEvent(
    val action: String,
    val table: String,
    val record: JsonObject,
    @JsonNames("old_record")
    val oldRecord: JsonObject? = null,
)

data class RealtimeOptions private constructor(
    val maxReconnectAttempts: Int,
    val reconnectDelays: List<Duration>,
    val jitterMax: Duration,
) {
    companion object {
        private val defaultReconnectDelays = listOf(250.milliseconds, 500.milliseconds, 1.seconds, 2.seconds, 4.seconds)

        operator fun invoke(
            maxReconnectAttempts: Int = 5,
            reconnectDelays: List<Duration> = defaultReconnectDelays,
            jitterMax: Duration = 100.milliseconds,
        ): RealtimeOptions {
            val normalizedAttempts = maxReconnectAttempts.coerceAtLeast(0)
            val normalizedDelays = reconnectDelays
                .map { if (it < Duration.ZERO) Duration.ZERO else it }
                .ifEmpty { defaultReconnectDelays }
            val normalizedJitter = if (jitterMax < Duration.ZERO) Duration.ZERO else jitterMax

            return RealtimeOptions(
                maxReconnectAttempts = normalizedAttempts,
                reconnectDelays = normalizedDelays,
                jitterMax = normalizedJitter,
            )
        }
    }
}

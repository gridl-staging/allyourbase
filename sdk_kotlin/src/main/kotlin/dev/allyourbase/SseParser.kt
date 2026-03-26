package dev.allyourbase

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.flow.flow

data class SseMessage(
    val event: String? = null,
    val data: String? = null,
    val id: String? = null,
    val retry: String? = null,
)

object SseParser {
    fun parse(lines: Flow<String>): Flow<SseMessage> = flow {
        var event: String? = null
        var id: String? = null
        var retry: String? = null
        val dataLines = mutableListOf<String>()

        suspend fun flush() {
            if (event == null && id == null && retry == null && dataLines.isEmpty()) {
                return
            }
            emit(
                SseMessage(
                    event = event,
                    data = if (dataLines.isEmpty()) null else dataLines.joinToString("\n"),
                    id = id,
                    retry = retry,
                ),
            )
            event = null
            id = null
            retry = null
            dataLines.clear()
        }

        lines.collect { line ->
            if (line.isEmpty()) {
                flush()
                return@collect
            }

            if (line.startsWith(":")) {
                return@collect
            }

            val separatorIndex = line.indexOf(':')
            if (separatorIndex == -1) {
                return@collect
            }

            val field = line.substring(0, separatorIndex)
            var value = line.substring(separatorIndex + 1)
            if (value.startsWith(" ")) {
                value = value.substring(1)
            }

            when (field) {
                "event" -> event = value
                "data" -> dataLines.add(value)
                "id" -> id = value
                "retry" -> retry = value
            }
        }

        flush()
    }
}

package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class AYBExceptionTest {
    private val json = Json

    @Test
    fun `from response parses numeric code and doc_url`() {
        val body = buildJsonObject {
            put("message", "forbidden")
            put("code", 403)
            put("doc_url", "https://docs.example/errors#forbidden")
            putJsonObject("data") {
                put("resource", "posts")
            }
        }

        val error = AYBException.from(
            HttpResponse(
                statusCode = 403,
                statusText = "Forbidden",
                headers = emptyMap(),
                body = jsonToBytes(body),
            ),
        )

        assertEquals(403, error.status)
        assertEquals("forbidden", error.message)
        assertEquals("403", error.code)
        assertEquals("https://docs.example/errors#forbidden", error.docUrl)
        assertEquals("posts", error.data?.get("resource")?.toString()?.trim('"'))
    }

    @Test
    fun `from response parses string code and docUrl`() {
        val body = buildJsonObject {
            put("message", "missing refresh")
            put("code", "auth/missing-refresh-token")
            put("docUrl", "https://docs.example/errors#missing-refresh")
        }

        val error = AYBException.from(
            HttpResponse(
                statusCode = 400,
                statusText = "Bad Request",
                headers = emptyMap(),
                body = jsonToBytes(body),
            ),
        )

        assertEquals("auth/missing-refresh-token", error.code)
        assertEquals("https://docs.example/errors#missing-refresh", error.docUrl)
    }

    @Test
    fun `from response falls back to status text when body is missing message`() {
        val body = buildJsonObject { put("code", "x") }

        val error = AYBException.from(
            HttpResponse(
                statusCode = 418,
                statusText = "I'm a teapot",
                headers = emptyMap(),
                body = jsonToBytes(body),
            ),
        )

        assertEquals("I'm a teapot", error.message)
    }

    @Test
    fun `from response falls back to status text for non-json body`() {
        val error = AYBException.from(
            HttpResponse(
                statusCode = 500,
                statusText = "Internal Server Error",
                headers = emptyMap(),
                body = "<html>oops</html>".encodeToByteArray(),
            ),
        )

        assertEquals("Internal Server Error", error.message)
        assertNull(error.code)
        assertNull(error.data)
        assertNull(error.docUrl)
    }
}

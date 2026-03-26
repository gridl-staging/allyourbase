package dev.allyourbase

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class RequestBuilderTest {
    @Test
    fun `build url sorts dictionary query keys`() {
        val builder = RequestBuilder("https://api.example.com")

        val url = builder.buildUrl(
            path = "/api/collections/posts",
            query = mapOf("z" to "9", "a" to "1", "m" to "5"),
            queryItems = emptyList(),
        )

        assertEquals("https://api.example.com/api/collections/posts?a=1&m=5&z=9", url)
    }

    @Test
    fun `build url uses query items when provided`() {
        val builder = RequestBuilder("https://api.example.com")

        val url = builder.buildUrl(
            path = "api/items",
            query = mapOf("ignored" to "1"),
            queryItems = listOf("second" to "2", "first" to "1"),
        )

        assertEquals("https://api.example.com/api/items?second=2&first=1", url)
    }

    @Test
    fun `build request injects defaults and authorization with json body`() {
        val builder = RequestBuilder("https://api.example.com")
        val body = buildJsonObject {
            put("email", "dev@allyourbase.io")
            put("password", "secret")
        }

        val request = builder.buildRequest(
            path = "/api/auth/login",
            method = HttpMethod.POST,
            query = emptyMap(),
            queryItems = emptyList(),
            headers = mapOf("X-Custom" to "yes"),
            body = body,
            bearerToken = "jwt_123",
        )

        assertEquals("https://api.example.com/api/auth/login", request.url)
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("application/json", request.headers["Accept"])
        assertEquals("application/json", request.headers["Content-Type"])
        assertEquals("Bearer jwt_123", request.headers["Authorization"])
        assertEquals("yes", request.headers["X-Custom"])
        assertEquals("{\"email\":\"dev@allyourbase.io\",\"password\":\"secret\"}", request.body!!.decodeToString())
    }

    @Test
    fun `build request omits content type when body is null and honors header override`() {
        val builder = RequestBuilder("https://api.example.com")

        val request = builder.buildRequest(
            path = "/api/collections/posts",
            method = HttpMethod.GET,
            query = emptyMap(),
            queryItems = emptyList(),
            headers = mapOf("Accept" to "text/plain"),
            body = null,
            bearerToken = null,
        )

        assertEquals("text/plain", request.headers["Accept"])
        assertNull(request.headers["Content-Type"])
        assertNull(request.body)
    }

    @Test
    fun `build request handles header names case-insensitively`() {
        val builder = RequestBuilder("https://api.example.com")
        val body = buildJsonObject {
            put("ok", true)
        }

        val request = builder.buildRequest(
            path = "/api/test",
            method = HttpMethod.POST,
            headers = mapOf(
                "accept" to "text/plain",
                "content-type" to "application/custom+json",
                "authorization" to "Bearer stale",
            ),
            body = body,
            bearerToken = "jwt_fresh",
        )

        val acceptKeys = request.headers.keys.filter { it.equals("accept", ignoreCase = true) }
        val contentTypeKeys = request.headers.keys.filter { it.equals("content-type", ignoreCase = true) }
        val authorizationKeys = request.headers.keys.filter { it.equals("authorization", ignoreCase = true) }

        assertEquals(1, acceptKeys.size)
        assertEquals(1, contentTypeKeys.size)
        assertEquals(1, authorizationKeys.size)
        assertEquals("text/plain", request.headers[acceptKeys.first()])
        assertEquals("application/custom+json", request.headers[contentTypeKeys.first()])
        assertEquals("Bearer jwt_fresh", request.headers[authorizationKeys.first()])
        assertTrue(request.body!!.isNotEmpty())
    }
}

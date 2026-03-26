package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonPrimitive
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class TransportModelsTest {
    @Test
    fun `http method enum matches expected verbs`() {
        assertEquals("GET", HttpMethod.GET.name)
        assertEquals("POST", HttpMethod.POST.name)
        assertEquals("PATCH", HttpMethod.PATCH.name)
        assertEquals("DELETE", HttpMethod.DELETE.name)
    }

    @Test
    fun `http request and response hold expected fields`() {
        val request = HttpRequest(
            url = "https://example.com/api/test",
            method = HttpMethod.POST,
            headers = mapOf("X-Test" to "yes"),
            body = "hello".encodeToByteArray(),
        )

        assertEquals("https://example.com/api/test", request.url)
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("yes", request.headers["X-Test"])
        assertArrayEquals("hello".encodeToByteArray(), request.body)

        val response = HttpResponse(
            statusCode = 201,
            statusText = "Created",
            headers = mapOf("content-type" to "application/json"),
            body = "{}".encodeToByteArray(),
        )

        assertEquals(201, response.statusCode)
        assertEquals("Created", response.statusText)
        assertEquals("application/json", response.headers["content-type"])
        assertArrayEquals("{}".encodeToByteArray(), response.body)
    }

    @Test
    fun `mock transport captures requests and dequeues responses`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = JsonPrimitive("ok"),
            ),
        )

        val request = HttpRequest(
            url = "https://example.com/api/health",
            method = HttpMethod.GET,
            headers = emptyMap(),
            body = null,
        )

        val response = transport.send(request)

        assertEquals(200, response.statusCode)
        assertEquals(1, transport.requests.size)
        assertEquals("https://example.com/api/health", transport.requests.first().url)
    }

    @Test
    fun `mock helpers support json bytes and case-insensitive header lookup`() {
        val bytes = jsonToBytes(JsonPrimitive("value"))
        assertArrayEquals("\"value\"".encodeToByteArray(), bytes)

        val headers = mapOf("Content-TYPE" to "application/json")
        assertEquals("application/json", lowercasedLookup(headers, "content-type"))
        assertNull(lowercasedLookup(headers, "authorization"))
    }
}

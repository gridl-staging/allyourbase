package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.net.URI
import java.net.URLDecoder
import java.nio.charset.StandardCharsets

class StorageClientTest {
    @Test
    fun `upload sends multipart body and content type`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = buildJsonObject {
                    put("id", "file_1")
                    put("bucket", "uploads")
                    put("name", "doc.txt")
                    put("size", 5)
                    put("contentType", "text/plain")
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)

        val uploaded = client.storage.upload(
            bucket = "uploads",
            data = "hello".encodeToByteArray(),
            name = "doc.txt",
            contentType = "text/plain",
        )

        assertEquals("file_1", uploaded.id)

        val request = transport.requests.first()
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("/api/storage/uploads", URI(request.url).path)

        val contentType = lowercasedLookup(request.headers, "content-type")
        assertTrue(contentType!!.startsWith("multipart/form-data; boundary="))

        val bodyText = request.body!!.decodeToString()
        assertTrue(bodyText.contains("Content-Disposition: form-data; name=\"file\"; filename=\"doc.txt\""))
        assertTrue(bodyText.contains("Content-Type: text/plain"))
        assertTrue(bodyText.contains("hello"))
    }

    @Test
    fun `download url returns deterministic path`() {
        val client = AYBClient("https://api.example.com")

        val url = client.storage.downloadUrl(bucket = "uploads", name = "nested/file.txt")

        assertEquals("https://api.example.com/api/storage/uploads/nested/file.txt", url)
    }

    @Test
    fun `delete uses expected method and path`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))

        val client = AYBClient("https://api.example.com", transport = transport)
        client.storage.delete("uploads", "doc.txt")

        val request = transport.requests.first()
        assertEquals(HttpMethod.DELETE, request.method)
        assertEquals("/api/storage/uploads/doc.txt", URI(request.url).path)
    }

    @Test
    fun `list sends query params and decodes envelope`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = buildJsonObject {
                    put("totalItems", 1)
                    put("items", kotlinx.serialization.json.buildJsonArray {
                        add(
                            buildJsonObject {
                                put("id", "file_1")
                                put("bucket", "uploads")
                                put("name", "doc.txt")
                                put("size", 5)
                                put("contentType", "text/plain")
                            },
                        )
                    })
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)
        val listed = client.storage.list("uploads", prefix = "dir/", limit = 10, offset = 20)

        assertEquals(1, listed.totalItems)
        assertEquals("file_1", listed.items.first().id)

        val query = URI(transport.requests.first().url).query
            .split("&")
            .associate { token ->
                val parts = token.split("=", limit = 2)
                URLDecoder.decode(parts[0], StandardCharsets.UTF_8) to
                    URLDecoder.decode(parts.getOrElse(1) { "" }, StandardCharsets.UTF_8)
            }

        assertEquals("dir/", query["prefix"])
        assertEquals("10", query["limit"])
        assertEquals("20", query["offset"])
    }

    @Test
    fun `signed url prepends base url for relative path`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = buildJsonObject { put("url", "/api/storage/uploads/doc.txt?token=signed") },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)
        val signed = client.storage.getSignedUrl("uploads", "doc.txt", expiresIn = 30)

        assertEquals("https://api.example.com/api/storage/uploads/doc.txt?token=signed", signed)
        val requestBody = kotlinx.serialization.json.Json.parseToJsonElement(transport.requests.first().body!!.decodeToString()).jsonObject
        assertEquals("30", requestBody["expiresIn"]?.jsonPrimitive?.content)
    }

    @Test
    fun `storage errors surface as ayb exception`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 404,
                json = buildJsonObject {
                    put("message", "not found")
                    put("code", "storage/not-found")
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)

        runCatching { client.storage.list("missing") }
            .onSuccess { throw AssertionError("expected failure") }
            .onFailure { error ->
                val ayb = error as AYBException
                assertEquals(404, ayb.status)
                assertEquals("storage/not-found", ayb.code)
            }
    }
}

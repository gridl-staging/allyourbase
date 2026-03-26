package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.putJsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class RecordsClientTest {
    private val json = Json

    @Test
    fun `list uses query params and decodes metadata plus items`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = buildJsonObject {
                    putJsonArray("items") {
                        add(buildJsonObject { put("id", "rec_1"); put("title", "A") })
                        add(buildJsonObject { put("id", "rec_2"); put("title", "B") })
                    }
                    put("page", 1)
                    put("perPage", 2)
                    put("totalItems", 2)
                    put("totalPages", 1)
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)
        val result = client.records.list(
            collection = "posts",
            params = ListParams(
                page = 1,
                perPage = 2,
                sort = "-created",
                filter = "status='pub'",
                search = "hello",
                fields = "id,title",
                expand = "author",
                skipTotal = true,
            ),
        )

        val request = transport.requests.first()
        assertEquals(HttpMethod.GET, request.method)
        assertEquals("/api/collections/posts", java.net.URI(request.url).path)
        assertEquals(2, result.items.size)
        assertEquals(2, result.metadata.totalItems)
        assertEquals("rec_1", result.items[0]["id"]!!.jsonPrimitive.content)
    }

    @Test
    fun `get works with and without auth token`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = buildJsonObject { put("id", "rec_1") }))
        transport.enqueue(StubResponse(status = 200, json = buildJsonObject { put("id", "rec_2") }))

        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt", "refresh")

        val withAuth = client.records.get("posts", "rec_1")
        assertEquals("rec_1", withAuth["id"]!!.jsonPrimitive.content)
        assertEquals("Bearer jwt", lowercasedLookup(transport.requests[0].headers, "authorization"))

        client.clearTokens()
        val noAuth = client.records.get("posts", "rec_2")
        assertEquals("rec_2", noAuth["id"]!!.jsonPrimitive.content)
        assertNull(lowercasedLookup(transport.requests[1].headers, "authorization"))
    }

    @Test
    fun `create update and delete use expected methods`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 201, json = buildJsonObject { put("id", "rec_new") }))
        transport.enqueue(StubResponse(status = 200, json = buildJsonObject { put("id", "rec_new"); put("title", "updated") }))
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))

        val client = AYBClient("https://api.example.com", transport = transport)

        val created = client.records.create("posts", buildJsonObject { put("title", "new") })
        val updated = client.records.update("posts", "rec_new", buildJsonObject { put("title", "updated") })
        client.records.delete("posts", "rec_new")

        assertEquals("rec_new", created["id"]!!.jsonPrimitive.content)
        assertEquals("updated", updated["title"]!!.jsonPrimitive.content)
        assertEquals(HttpMethod.POST, transport.requests[0].method)
        assertEquals(HttpMethod.PATCH, transport.requests[1].method)
        assertEquals(HttpMethod.DELETE, transport.requests[2].method)
    }

    @Test
    fun `batch posts operations and decodes results`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = buildJsonArray {
                    add(buildJsonObject {
                        put("index", 0)
                        put("status", 201)
                        putJsonObject("body") { put("id", "rec_1") }
                    })
                    add(buildJsonObject {
                        put("index", 1)
                        put("status", 204)
                    })
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)
        val result = client.records.batch(
            collection = "posts",
            operations = listOf(
                BatchOperation("POST", body = buildJsonObject { put("title", "one") }),
                BatchOperation("DELETE", id = "rec_1"),
            ),
        )

        val request = transport.requests.first()
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("/api/collections/posts/batch", java.net.URI(request.url).path)

        val reqBody = json.parseToJsonElement(request.body!!.decodeToString()).jsonObject
        val ops = reqBody["operations"] as JsonArray
        assertEquals(2, ops.size)
        assertEquals("POST", ops[0].jsonObject["method"]!!.jsonPrimitive.content)

        assertEquals(2, result.size)
        assertEquals(201, result[0].status)
        assertEquals("rec_1", result[0].body?.get("id")?.jsonPrimitive?.content)
        assertEquals(204, result[1].status)
        assertNull(result[1].body)
    }

    @Test
    fun `errors are surfaced as ayb exception`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 401,
                json = buildJsonObject {
                    put("message", "unauthorized")
                    put("code", "auth/unauthorized")
                },
            ),
        )
        val client = AYBClient("https://api.example.com", transport = transport)

        runCatching { client.records.list("posts") }
            .onSuccess { throw AssertionError("expected failure") }
            .onFailure { error ->
                val ayb = error as AYBException
                assertEquals(401, ayb.status)
                assertEquals("auth/unauthorized", ayb.code)
            }
    }
}

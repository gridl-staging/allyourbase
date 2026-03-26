package dev.allyourbase

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

class AYBClientCoreTest {
    @Test
    fun `configuration defaults and api key seeding are correct`() {
        val client = AYBClient("https://api.example.com", apiKey = "service_key")

        assertEquals("https://api.example.com", client.configuration.baseURL)
        assertEquals("service_key", client.configuration.apiKey)
        assertEquals(30.seconds, client.configuration.timeout)
        assertEquals(0, client.configuration.maxRetries)
        assertEquals(Duration.ZERO, client.configuration.retryDelay)
        assertEquals("service_key", client.token)
    }

    @Test
    fun `token management helpers update store`() {
        val client = AYBClient("https://api.example.com")

        client.setTokens("a1", "r1")
        assertEquals("a1", client.token)
        assertEquals("r1", client.refreshToken)

        client.clearTokens()
        assertNull(client.token)
        assertNull(client.refreshToken)

        client.setApiKey("service")
        assertEquals("service", client.token)
        assertNull(client.refreshToken)

        client.clearApiKey()
        assertNull(client.token)
    }

    @Test
    fun `request attaches bearer token and decodes payload`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = buildJsonObject { put("ok", true) }))

        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_1", "refresh_1")

        val value = client.request(
            path = "/api/health",
            method = HttpMethod.GET,
            decode = { json -> (json as JsonObject)["ok"]!!.jsonPrimitive.content },
        )

        assertEquals("true", value)
        assertEquals("Bearer jwt_1", lowercasedLookup(transport.requests.first().headers, "authorization"))
    }

    @Test
    fun `request skip auth omits authorization header`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = JsonPrimitive("ok")))

        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_1", "refresh_1")

        val value = client.request(
            path = "/api/public",
            method = HttpMethod.GET,
            skipAuth = true,
            decode = { it?.jsonPrimitive?.content },
        )

        assertEquals("ok", value)
        assertNull(lowercasedLookup(transport.requests.first().headers, "authorization"))
    }

    @Test
    fun `request throws ayb exception for non-2xx`() = runTest {
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

        runCatching {
            client.request(
                path = "/api/private",
                method = HttpMethod.GET,
                decode = { it },
            )
        }.onSuccess {
            throw AssertionError("expected AYBException")
        }.onFailure { error ->
            val ayb = error as AYBException
            assertEquals(401, ayb.status)
            assertEquals("auth/unauthorized", ayb.code)
        }
    }

    @Test
    fun `request returns null payload for 204`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)

        val value = client.request(
            path = "/api/empty",
            method = HttpMethod.DELETE,
            decode = { payload -> payload },
        )

        assertNull(value)
    }

    @Test
    fun `send with retries retries non-cancellation errors only`() = runTest {
        val retryingTransport = MockHttpTransport().apply {
            enqueue(IllegalStateException("transient"))
            enqueue(StubResponse(status = 200, json = JsonPrimitive("ok")))
        }
        val retryingClient = AYBClient(
            baseURL = "https://api.example.com",
            transport = retryingTransport,
            maxRetries = 1,
            retryDelay = Duration.ZERO,
        )

        val retried = retryingClient.request(
            path = "/api/retry",
            method = HttpMethod.GET,
            decode = { it?.jsonPrimitive?.content },
        )
        assertEquals("ok", retried)
        assertEquals(2, retryingTransport.requests.size)

        val cancellingTransport = MockHttpTransport().apply {
            enqueue(CancellationException("cancelled"))
        }
        val cancellingClient = AYBClient(
            baseURL = "https://api.example.com",
            transport = cancellingTransport,
            maxRetries = 3,
            retryDelay = Duration.ZERO,
        )

        runCatching {
            cancellingClient.request(
                path = "/api/retry",
                method = HttpMethod.GET,
                decode = { it },
            )
        }.onSuccess {
            throw AssertionError("expected cancellation")
        }.onFailure { error ->
            assertTrue(error is CancellationException)
            assertEquals(1, cancellingTransport.requests.size)
        }
    }

    @Test
    fun `auth state listeners can unsubscribe during emit`() {
        val client = AYBClient("https://api.example.com")
        val events = mutableListOf<Pair<AuthStateEvent, AuthSession?>>()

        lateinit var unsubscribeFirst: () -> Unit
        unsubscribeFirst = client.onAuthStateChange { event, session ->
            events.add(event to session)
            unsubscribeFirst()
        }
        client.onAuthStateChange { event, session ->
            events.add(event to session)
        }

        client.setTokens("token-1", "refresh-1")
        client.emitAuthState(AuthStateEvent.SIGNED_IN)
        client.emitAuthState(AuthStateEvent.TOKEN_REFRESHED)

        assertEquals(3, events.size)
        assertEquals(AuthStateEvent.SIGNED_IN, events[0].first)
        assertEquals("token-1", events[0].second?.token)
        assertEquals(AuthStateEvent.SIGNED_IN, events[1].first)
        assertEquals(AuthStateEvent.TOKEN_REFRESHED, events[2].first)
    }
}

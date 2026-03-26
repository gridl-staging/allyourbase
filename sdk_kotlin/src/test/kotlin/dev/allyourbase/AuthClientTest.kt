package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class AuthClientTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `register and login send expected request shape`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("r1", "rr1")))
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("r2", "rr2")))

        val client = AYBClient("https://api.example.com", transport = transport)

        val register = client.auth.register("dev@allyourbase.io", "secret")
        val login = client.auth.login("dev@allyourbase.io", "secret")

        assertEquals("r1", register.token)
        assertEquals("r2", login.token)

        val first = transport.requests[0]
        assertEquals(HttpMethod.POST, first.method)
        assertEquals("/api/auth/register", java.net.URI(first.url).path)
        val firstBody = json.parseToJsonElement(first.body!!.decodeToString()) as JsonObject
        assertEquals("dev@allyourbase.io", firstBody["email"]!!.jsonPrimitive.content)
        assertEquals("secret", firstBody["password"]!!.jsonPrimitive.content)

        val second = transport.requests[1]
        assertEquals(HttpMethod.POST, second.method)
        assertEquals("/api/auth/login", java.net.URI(second.url).path)
    }

    @Test
    fun `me uses bearer token`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = buildJsonObject {
                    put("id", "usr_1")
                    put("email", "dev@allyourbase.io")
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_1", "refresh_1")

        val me = client.auth.me()

        assertEquals("usr_1", me.id)
        assertEquals("Bearer jwt_1", lowercasedLookup(transport.requests.first().headers, "authorization"))
    }

    @Test
    fun `refresh posts refresh token stores new tokens and emits event`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_new", "refresh_new")))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_old", "refresh_old")

        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val refreshed = client.auth.refresh()

        assertEquals("jwt_new", refreshed.token)
        assertEquals("jwt_new", client.token)
        assertEquals("refresh_new", client.refreshToken)
        val body = json.parseToJsonElement(transport.requests.first().body!!.decodeToString()) as JsonObject
        assertEquals("refresh_old", body["refreshToken"]!!.jsonPrimitive.content)
        assertTrue(events.contains(AuthStateEvent.TOKEN_REFRESHED))
    }

    @Test
    fun `logout clears tokens and emits signed out`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_old", "refresh_old")

        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        client.auth.logout()

        assertNull(client.token)
        assertNull(client.refreshToken)
        assertTrue(events.contains(AuthStateEvent.SIGNED_OUT))
    }

    @Test
    fun `missing refresh token throws without network call`() = runTest {
        val transport = MockHttpTransport()
        val client = AYBClient("https://api.example.com", transport = transport)

        runCatching { client.auth.refresh() }
            .onSuccess { throw AssertionError("expected failure") }
            .onFailure { error ->
                val ayb = error as AYBException
                assertEquals("auth/missing-refresh-token", ayb.code)
            }

        assertEquals(0, transport.requests.size)
    }

    @Test
    fun `token lifecycle login to logout clears state`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_login", "refresh_login")))
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))

        val client = AYBClient("https://api.example.com", transport = transport)

        client.auth.login("dev@allyourbase.io", "secret")
        assertEquals("jwt_login", client.token)
        assertEquals("refresh_login", client.refreshToken)

        client.auth.logout()
        assertNull(client.token)
        assertNull(client.refreshToken)
    }

    @Test
    fun `auth state emits signed in and token refreshed and supports unsubscribe`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_login", "refresh_login")))
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_refresh", "refresh_refresh")))

        val client = AYBClient("https://api.example.com", transport = transport)
        val events = mutableListOf<AuthStateEvent>()

        val unsubscribe = client.onAuthStateChange { event, _ -> events.add(event) }
        client.auth.login("dev@allyourbase.io", "secret")
        client.auth.refresh()

        unsubscribe()
        client.emitAuthState(AuthStateEvent.SIGNED_OUT)

        assertEquals(listOf(AuthStateEvent.SIGNED_IN, AuthStateEvent.TOKEN_REFRESHED), events)
    }

    private fun authResponseJson(token: String, refresh: String) = buildJsonObject {
        put("token", token)
        put("refreshToken", refresh)
        putJsonObject("user") {
            put("id", "usr_1")
            put("email", "dev@allyourbase.io")
            put("email_verified", true)
            put("created_at", "2026-01-01T00:00:00Z")
        }
    }
}

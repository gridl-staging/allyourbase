package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.serializer

class AuthClient internal constructor(
    private val client: AYBClient,
) {
    private val json = Json { ignoreUnknownKeys = true }

    suspend fun register(email: String, password: String): AuthResponse =
        authenticate(path = "/api/auth/register", email = email, password = password)

    suspend fun login(email: String, password: String): AuthResponse =
        authenticate(path = "/api/auth/login", email = email, password = password)

    suspend fun me(): User =
        client.request(
            path = "/api/auth/me",
            method = HttpMethod.GET,
            decode = { payload -> decodePayload(payload) },
        )

    suspend fun logout() {
        val refreshToken = requireRefreshToken()
        client.request(
            path = "/api/auth/logout",
            method = HttpMethod.POST,
            body = buildJsonObject { put("refreshToken", refreshToken) },
            decode = { Unit },
        )
        client.clearTokens()
        client.emitAuthState(AuthStateEvent.SIGNED_OUT)
    }

    suspend fun refresh(): AuthResponse {
        val refreshToken = requireRefreshToken()
        val response: AuthResponse = client.request(
            path = "/api/auth/refresh",
            method = HttpMethod.POST,
            body = buildJsonObject { put("refreshToken", refreshToken) },
            decode = { payload -> decodePayload(payload) },
        )
        client.setTokens(response.token, response.refreshToken)
        client.emitAuthState(AuthStateEvent.TOKEN_REFRESHED)
        return response
    }

    private suspend fun authenticate(path: String, email: String, password: String): AuthResponse {
        val response: AuthResponse = client.request(
            path = path,
            method = HttpMethod.POST,
            body = buildJsonObject {
                put("email", email)
                put("password", password)
            },
            decode = { payload -> decodePayload(payload) },
        )
        client.setTokens(response.token, response.refreshToken)
        client.emitAuthState(AuthStateEvent.SIGNED_IN)
        return response
    }

    private fun requireRefreshToken(): String {
        val refreshToken = client.refreshToken
        if (refreshToken.isNullOrEmpty()) {
            throw AYBException(
                status = 400,
                message = "Missing refresh token",
                code = "auth/missing-refresh-token",
            )
        }
        return refreshToken
    }

    private inline fun <reified T> decodePayload(payload: JsonElement?): T {
        if (payload == null) {
            throw AYBException(status = 500, message = "Empty response payload")
        }
        return json.decodeFromJsonElement(serializer<T>(), payload)
    }
}

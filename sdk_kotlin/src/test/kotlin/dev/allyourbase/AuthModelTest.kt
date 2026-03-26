package dev.allyourbase

import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class AuthModelTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `user decodes aliases with json names`() {
        val payload = """
            {
              "user_id": "usr_1",
              "email_address": "dev@allyourbase.io",
              "email_verified": true,
              "created": "2026-01-01T00:00:00Z",
              "updated_at": null
            }
        """.trimIndent()

        val user = json.decodeFromString<User>(payload)
        assertEquals("usr_1", user.id)
        assertEquals("dev@allyourbase.io", user.email)
        assertEquals(true, user.emailVerified)
        assertEquals("2026-01-01T00:00:00Z", user.createdAt)
        assertNull(user.updatedAt)
    }

    @Test
    fun `auth response decodes canonical payload`() {
        val payload = """
            {
              "token": "jwt_1",
              "refreshToken": "refresh_1",
              "user": {
                "id": "usr_1",
                "email": "dev@allyourbase.io"
              }
            }
        """.trimIndent()

        val response = json.decodeFromString<AuthResponse>(payload)
        assertEquals("jwt_1", response.token)
        assertEquals("refresh_1", response.refreshToken)
        assertEquals("usr_1", response.user.id)
    }
}

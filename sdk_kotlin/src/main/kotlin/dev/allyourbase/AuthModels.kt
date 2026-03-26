package dev.allyourbase

import kotlinx.serialization.Serializable
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.JsonNames

@Serializable
@OptIn(ExperimentalSerializationApi::class)
data class User(
    @JsonNames("userId", "user_id")
    val id: String,
    @JsonNames("emailAddress", "email_address")
    val email: String,
    @JsonNames("email_verified")
    val emailVerified: Boolean? = null,
    @JsonNames("created_at", "created")
    val createdAt: String? = null,
    @JsonNames("updated_at", "updated")
    val updatedAt: String? = null,
)

@Serializable
data class AuthResponse(
    val token: String,
    val refreshToken: String,
    val user: User,
)

package dev.allyourbase

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonNames

@Serializable
@OptIn(ExperimentalSerializationApi::class)
data class StorageObject(
    val id: String,
    val bucket: String,
    val name: String,
    val size: Int,
    @JsonNames("content_type")
    val contentType: String,
    @JsonNames("user_id")
    val userId: String? = null,
    @JsonNames("created_at", "created")
    val createdAt: String? = null,
    @JsonNames("updated_at", "updated")
    val updatedAt: String? = null,
)

@Serializable
@OptIn(ExperimentalSerializationApi::class)
data class StorageListResponse(
    val items: List<StorageObject>,
    @JsonNames("total_items")
    val totalItems: Int,
)

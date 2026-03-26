package dev.allyourbase

import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class StorageModelTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `storage object decodes required and alias fields`() {
        val payload = """
            {
              "id": "file_1",
              "bucket": "uploads",
              "name": "doc.pdf",
              "size": 1024,
              "content_type": "application/pdf",
              "user_id": "usr_1",
              "created_at": "2026-01-01T00:00:00Z",
              "updated_at": null
            }
        """.trimIndent()

        val decoded = json.decodeFromString<StorageObject>(payload)

        assertEquals("file_1", decoded.id)
        assertEquals("uploads", decoded.bucket)
        assertEquals("doc.pdf", decoded.name)
        assertEquals(1024, decoded.size)
        assertEquals("application/pdf", decoded.contentType)
        assertEquals("usr_1", decoded.userId)
        assertEquals("2026-01-01T00:00:00Z", decoded.createdAt)
        assertNull(decoded.updatedAt)
    }

    @Test
    fun `storage list response decodes envelope`() {
        val payload = """
            {
              "items": [
                {
                  "id": "file_1",
                  "bucket": "uploads",
                  "name": "a.txt",
                  "size": 10,
                  "contentType": "text/plain"
                },
                {
                  "id": "file_2",
                  "bucket": "uploads",
                  "name": "b.txt",
                  "size": 20,
                  "content_type": "text/plain"
                }
              ],
              "totalItems": 2
            }
        """.trimIndent()

        val decoded = json.decodeFromString<StorageListResponse>(payload)

        assertEquals(2, decoded.totalItems)
        assertEquals(2, decoded.items.size)
        assertEquals("file_1", decoded.items[0].id)
        assertEquals("file_2", decoded.items[1].id)
    }
}

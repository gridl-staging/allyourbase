package dev.allyourbase

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MultipartBodyTest {
    @Test
    fun `build includes boundary content disposition and content type`() {
        val built = MultipartBody.build(
            fieldName = "file",
            data = "hello".encodeToByteArray(),
            filename = "greeting.txt",
            contentType = "text/plain",
            boundary = "test-boundary",
        )

        assertEquals("multipart/form-data; boundary=test-boundary", built.contentType)

        val text = built.body.decodeToString()
        assertTrue(text.contains("--test-boundary\\r\\n"))
        assertTrue(text.contains("Content-Disposition: form-data; name=\"file\"; filename=\"greeting.txt\"\\r\\n"))
        assertTrue(text.contains("Content-Type: text/plain\\r\\n"))
        assertTrue(text.contains("\\r\\nhello\\r\\n"))
        assertTrue(text.contains("--test-boundary--\\r\\n"))
    }

    @Test
    fun `build supports omitted filename and content type`() {
        val built = MultipartBody.build(
            fieldName = "file",
            data = byteArrayOf(1, 2, 3),
            boundary = "test-boundary-2",
        )

        val text = built.body.decodeToString()
        assertTrue(text.contains("Content-Disposition: form-data; name=\"file\"\\r\\n"))
        assertTrue(!text.contains("filename="))
        assertTrue(!text.contains("Content-Type:"))
        assertTrue(text.endsWith("--test-boundary-2--\\r\\n"))
    }
}

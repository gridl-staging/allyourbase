package dev.allyourbase

import java.io.ByteArrayOutputStream
import java.util.UUID

data class MultipartBody(
    val body: ByteArray,
    val contentType: String,
) {
    companion object {
        fun build(
            fieldName: String,
            data: ByteArray,
            filename: String? = null,
            contentType: String? = null,
            boundary: String = "Boundary-${UUID.randomUUID()}",
        ): MultipartBody {
            val out = ByteArrayOutputStream()

            out.write("--$boundary\\r\\n".encodeToByteArray())

            val disposition = buildString {
                append("Content-Disposition: form-data; name=\"")
                append(fieldName)
                append("\"")
                if (!filename.isNullOrEmpty()) {
                    append("; filename=\"")
                    append(filename)
                    append("\"")
                }
            }
            out.write("$disposition\\r\\n".encodeToByteArray())

            if (!contentType.isNullOrEmpty()) {
                out.write("Content-Type: $contentType\\r\\n".encodeToByteArray())
            }

            out.write("\\r\\n".encodeToByteArray())
            out.write(data)
            out.write("\\r\\n--$boundary--\\r\\n".encodeToByteArray())

            return MultipartBody(
                body = out.toByteArray(),
                contentType = "multipart/form-data; boundary=$boundary",
            )
        }
    }
}

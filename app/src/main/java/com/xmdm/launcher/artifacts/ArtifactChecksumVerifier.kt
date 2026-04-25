package com.xmdm.launcher.artifacts

import java.io.File
import java.io.FileInputStream
import java.io.InputStream
import java.security.MessageDigest
import java.util.Base64

class ArtifactChecksumVerifier {
    fun verify(content: ByteArray, expectedChecksum: String) {
        require(expectedChecksum.isNotBlank()) { "artifact checksum must not be blank" }
        val actual = sha256Base64Url(content)
        require(actual == expectedChecksum.trim()) {
            "invalid artifact checksum"
        }
    }

    fun verify(file: File, expectedChecksum: String) {
        require(expectedChecksum.isNotBlank()) { "artifact checksum must not be blank" }
        val actual = sha256Base64Url(file)
        require(actual == expectedChecksum.trim()) {
            "invalid artifact checksum"
        }
    }

    fun sha256Base64Url(content: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256")
        return Base64.getUrlEncoder().withoutPadding().encodeToString(digest.digest(content))
    }

    fun sha256Base64Url(file: File): String {
        FileInputStream(file).use { input ->
            return sha256Base64Url(input)
        }
    }

    fun sha256Base64Url(input: InputStream): String {
        val digest = MessageDigest.getInstance("SHA-256")
        val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
        while (true) {
            val read = input.read(buffer)
            if (read < 0) {
                break
            }
            digest.update(buffer, 0, read)
        }
        return Base64.getUrlEncoder().withoutPadding().encodeToString(digest.digest())
    }
}

package com.xmdm.launcher.artifacts

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

    fun sha256Base64Url(content: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256")
        return Base64.getUrlEncoder().withoutPadding().encodeToString(digest.digest(content))
    }
}

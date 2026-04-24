package com.xmdm.launcher.artifacts

import org.junit.Assert.assertEquals
import org.junit.Test

class ArtifactChecksumVerifierTest {
    private val verifier = ArtifactChecksumVerifier()

    @Test
    fun computesBase64UrlSha256() {
        val checksum = verifier.sha256Base64Url("hello".toByteArray())
        assertEquals("LPJNul-wow4m6DsqxbninhsWHlwfp0JecwQzYpOLmCQ", checksum)
    }

    @Test
    fun acceptsMatchingChecksum() {
        val content = "payload".toByteArray()
        verifier.verify(content, verifier.sha256Base64Url(content))
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsMismatchedChecksum() {
        verifier.verify("payload".toByteArray(), "bogus")
    }
}

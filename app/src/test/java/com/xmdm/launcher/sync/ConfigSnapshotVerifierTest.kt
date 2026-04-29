package com.xmdm.launcher.sync

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ConfigSnapshotVerifierTest {
    private val verifier = ConfigSnapshotVerifier()

    @Test
    fun verifiesSignedSnapshot() {
        val unsigned = """
            {
              "version":"1",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val signature = verifier.sign(unsigned, "secret-abc")
        val signed = """
            {
              "version":"1",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[],
              "signature":"$signature"
            }
        """.trimIndent()

        val parsed = verifier.verify(signed, "secret-abc")
        assertEquals("1", parsed.get("version").asString)
        assertTrue(parsed.get("signature").asString.isNotBlank())
    }

    @Test(expected = IllegalStateException::class)
    fun rejectsInvalidSignature() {
        verifier.verify(
            """
            {
              "version":"1",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{},
              "apps":[],
              "files":[],
              "certificates":[],
              "signature":"bogus"
            }
            """.trimIndent(),
            "secret-abc",
        )
    }
}

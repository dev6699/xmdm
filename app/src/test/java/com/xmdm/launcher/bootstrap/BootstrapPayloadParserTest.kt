package com.xmdm.launcher.bootstrap

import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test

class BootstrapPayloadParserTest {
    private val parser = BootstrapPayloadParser()

    @Test
    fun parsesCanonicalProvisioningPayload() {
        val parsed = parser.parse(
            """
            {
              "android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME": "com.xmdm.launcher/.AdminReceiver",
              "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION": "https://cdn.example/launcher.apk",
              "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM": "base64sha256",
              "android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED": true,
                "android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE": {
                "com.xmdm.BASE_URL": "https://mdm.example",
                "com.xmdm.SECONDARY_BASE_URL": "https://backup.example",
                "com.xmdm.ENROLLMENT_TOKEN": "token",
                "com.xmdm.DEVICE_ID": "serial-123"
              }
            }
            """.trimIndent(),
        )

        assertEquals("https://mdm.example", parsed.bootstrap.serverUrl)
        assertEquals("https://backup.example", parsed.bootstrap.secondaryServerUrl)
        assertEquals("token", parsed.bootstrap.enrollmentToken)
        assertEquals("serial-123", parsed.bootstrap.deviceId)
        assertEquals("{}", parsed.extrasJson)
    }

    @Test
    fun rejectsBareBootstrapKeys() {
        try {
            parser.parse(
                """
                {
                  "BASE_URL": "https://mdm.example",
                  "SECONDARY_BASE_URL": "https://backup.example",
                  "ENROLLMENT_TOKEN": "token",
                  "DEVICE_ID": "serial-123"
                }
                """.trimIndent(),
            )
            fail("expected bare bootstrap keys to be rejected")
        } catch (e: IllegalStateException) {
            assertEquals("bootstrap payload is missing a server URL", e.message)
        }
    }
}

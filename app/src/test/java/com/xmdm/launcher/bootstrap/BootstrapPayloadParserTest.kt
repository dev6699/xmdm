package com.xmdm.launcher.bootstrap

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
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
                "com.xmdm.SERVER_PROJECT": "rest",
                "com.xmdm.ENROLLMENT_TOKEN": "token",
                "com.xmdm.DEVICE_ID": "serial-123",
                "com.xmdm.DEVICE_ID_USE": "serial",
                "com.xmdm.CUSTOMER": "Acme",
                "com.xmdm.GROUP": "field"
              }
            }
            """.trimIndent(),
        )

        assertEquals("https://mdm.example", parsed.bootstrap.serverUrl)
        assertEquals("https://backup.example", parsed.bootstrap.secondaryServerUrl)
        assertEquals("rest", parsed.bootstrap.serverProject)
        assertEquals("token", parsed.bootstrap.enrollmentToken)
        assertEquals("serial-123", parsed.bootstrap.deviceId)
        assertEquals("serial", parsed.bootstrap.deviceIdUse)
        assertTrue(parsed.extrasJson.contains("\"com.xmdm.CUSTOMER\":\"Acme\""))
        assertTrue(parsed.extrasJson.contains("\"com.xmdm.GROUP\":\"field\""))
    }

    @Test
    fun parsesFallbackBootstrapPayload() {
        val parsed = parser.parse(
            """
            {
              "BASE_URL": "https://mdm.example",
              "SECONDARY_BASE_URL": "https://backup.example",
              "SERVER_PROJECT": "rest",
              "ENROLLMENT_TOKEN": "token",
              "DEVICE_ID": "serial-123",
              "DEVICE_ID_USE": "serial",
              "CUSTOMER": "Acme"
            }
            """.trimIndent(),
        )

        assertEquals("https://mdm.example", parsed.bootstrap.serverUrl)
        assertEquals("https://backup.example", parsed.bootstrap.secondaryServerUrl)
        assertEquals("rest", parsed.bootstrap.serverProject)
        assertEquals("token", parsed.bootstrap.enrollmentToken)
        assertEquals("serial-123", parsed.bootstrap.deviceId)
        assertEquals("serial", parsed.bootstrap.deviceIdUse)
        assertTrue(parsed.extrasJson.contains("\"CUSTOMER\":\"Acme\""))
    }
}

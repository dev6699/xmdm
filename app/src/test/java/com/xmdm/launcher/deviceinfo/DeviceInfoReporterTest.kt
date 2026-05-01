package com.xmdm.launcher.deviceinfo

import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertEquals
import org.junit.Test

class DeviceInfoReporterTest {
    @Test
    fun fingerprintChangesWhenRuntimeFieldsChange() {
        val base = linkedMapOf<String, Any?>(
            "deviceId" to "device-123",
            "configRevision" to 7L,
            "policyVersion" to 3L,
            "policyKioskMode" to false,
            "battery" to linkedMapOf(
                "level" to 50,
                "charging" to false,
                "plugged" to "",
            ),
        )
        val next = linkedMapOf<String, Any?>(
            "deviceId" to "device-123",
            "configRevision" to 7L,
            "policyVersion" to 3L,
            "policyKioskMode" to false,
            "battery" to linkedMapOf(
                "level" to 51,
                "charging" to false,
                "plugged" to "",
            ),
        )

        val baseFingerprint = deviceInfoFingerprint(base)
        val nextFingerprint = deviceInfoFingerprint(next)

        assertNotEquals(baseFingerprint, nextFingerprint)
        assertEquals(baseFingerprint, deviceInfoFingerprint(base))
    }
}

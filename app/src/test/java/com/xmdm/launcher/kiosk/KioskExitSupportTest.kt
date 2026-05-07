package com.xmdm.launcher.kiosk

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class KioskExitSupportTest {
    @Test
    fun gestureTrackerRequiresConsecutiveTaps() {
        val tracker = KioskExitGestureTracker(requiredTapCount = 3, resetTimeoutMs = 1_000L)

        assertFalse(tracker.registerTap(100L))
        assertFalse(tracker.registerTap(200L))
        assertTrue(tracker.registerTap(300L))
    }

    @Test
    fun gestureTrackerResetsAfterTimeout() {
        val tracker = KioskExitGestureTracker(requiredTapCount = 3, resetTimeoutMs = 1_000L)

        assertFalse(tracker.registerTap(100L))
        assertFalse(tracker.registerTap(200L))
        assertFalse(tracker.registerTap(1_500L))
        assertFalse(tracker.registerTap(1_600L))
        assertTrue(tracker.registerTap(1_700L))
    }

    @Test
    fun kioskExitPasscodeMatchesPolicyHash() {
        val hash = hashKioskExitPasscode("2468")
        val snapshotJson = """
            {
              "policy": {
                "restrictions": {
                  "kioskExitPasscodeHash": "$hash"
                }
              }
            }
        """.trimIndent()

        assertTrue(kioskExitPasscodeMatches(snapshotJson, "2468"))
        assertFalse(kioskExitPasscodeMatches(snapshotJson, "1357"))
    }
}

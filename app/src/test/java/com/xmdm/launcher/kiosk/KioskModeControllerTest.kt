package com.xmdm.launcher.kiosk

import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.PolicyCacheState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class KioskModeControllerTest {
    @Test
    fun enablesAndDisablesLockTaskForKioskPolicy() {
        val host = FakeHost(deviceOwner = true, inLockTaskMode = false)
        val controller = KioskModeController(host) { }

        controller.apply(agentState("""{"policy":{"kioskMode":true}}"""))

        assertTrue(host.setPackagesCalls.contains(listOf("com.xmdm.launcher")))
        assertTrue(host.startLockTaskCalls == 1)
        assertTrue(host.inLockTaskMode)

        controller.apply(agentState("""{"policy":{"kioskMode":false}}"""))

        assertTrue(host.stopLockTaskCalls == 1)
        assertTrue(host.setPackagesCalls.contains(emptyList()))
        assertFalse(host.inLockTaskMode)
    }

    @Test
    fun doesNotEnableKioskUntilContentForThatPolicyVersionIsReady() {
        val host = FakeHost(deviceOwner = true, inLockTaskMode = false)
        val controller = KioskModeController(host) { }

        controller.apply(
            agentState(
                snapshotJson = """{"policy":{"kioskMode":true}}""",
                policyVersion = 2,
                managedAppsVersion = 1,
            ),
        )

        assertEquals(0, host.startLockTaskCalls)
        assertFalse(host.inLockTaskMode)
    }

    private fun agentState(
        snapshotJson: String,
        policyVersion: Long = 1,
        managedAppsVersion: Long? = null,
        managedFilesVersion: Long? = null,
    ): AgentState {
        return AgentState(
            policyCache = PolicyCacheState(
                snapshotJson = snapshotJson,
                version = policyVersion,
                lastSyncAtEpochMillis = 1L,
            ),
            managedApps = managedAppsVersion?.let {
                com.xmdm.launcher.state.ManagedAppsState(
                    snapshotJson = snapshotJson,
                    version = it,
                    lastAppliedAtEpochMillis = 1L,
                )
            },
            managedFiles = managedFilesVersion?.let {
                com.xmdm.launcher.state.ManagedFilesState(
                    snapshotJson = snapshotJson,
                    version = it,
                    lastAppliedAtEpochMillis = 1L,
                )
            },
        )
    }

    private class FakeHost(
        override val packageName: String = "com.xmdm.launcher",
        var deviceOwner: Boolean = true,
        var inLockTaskMode: Boolean = false,
    ) : KioskModeHost {
        var startLockTaskCalls = 0
        var stopLockTaskCalls = 0
        val setPackagesCalls = mutableListOf<List<String>>()

        override fun isDeviceOwnerApp(): Boolean = deviceOwner

        override fun isInLockTaskMode(): Boolean = inLockTaskMode

        override fun setLockTaskPackages(packages: Array<String>) {
            setPackagesCalls += packages.toList()
        }

        override fun startLockTask() {
            startLockTaskCalls += 1
            inLockTaskMode = true
        }

        override fun stopLockTask() {
            stopLockTaskCalls += 1
            inLockTaskMode = false
        }
    }
}

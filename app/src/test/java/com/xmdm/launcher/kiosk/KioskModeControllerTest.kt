package com.xmdm.launcher.kiosk

import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.KioskControlState
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
        assertFalse(host.keepScreenOnCalls.last())
        assertFalse(host.stayAwakeWhilePluggedInCalls.last())

        controller.apply(agentState("""{"policy":{"kioskMode":false}}"""))

        assertTrue(host.stopLockTaskCalls == 1)
        assertTrue(host.setPackagesCalls.contains(emptyList()))
        assertFalse(host.inLockTaskMode)
        assertFalse(host.keepScreenOnCalls.last())
        assertFalse(host.stayAwakeWhilePluggedInCalls.last())
    }

    @Test
    fun launchesConfiguredKioskAppWhenPolicyNamesOne() {
        val host = FakeHost(deviceOwner = true, inLockTaskMode = true)
        val controller = KioskModeController(host) { }

        controller.apply(
            agentState(
                snapshotJson = """{"policy":{"kioskMode":true,"kioskAppPackage":"com.example.kiosk"}}""",
            ),
        )

        assertTrue(host.setPackagesCalls.contains(listOf("com.xmdm.launcher", "com.example.kiosk")))
        assertTrue(host.launchCalls.contains(LaunchCall(packageName = "com.example.kiosk", lockTaskEnabled = true)))
        assertTrue(host.stopLockTaskCalls == 1)
        assertTrue(host.finishHostActivityCalls == 1)
        assertFalse(host.inLockTaskMode)
    }

    @Test
    fun keepsScreenOnWhenKioskPolicyRequestsIt() {
        val host = FakeHost(deviceOwner = true, inLockTaskMode = false)
        val controller = KioskModeController(host) { }

        controller.apply(
            agentState(
                snapshotJson = """{"policy":{"kioskMode":true,"restrictions":{"kioskKeepScreenOn":true}}}""",
            ),
        )

        assertTrue(host.keepScreenOnCalls.last())

        controller.apply(
            agentState(
                snapshotJson = """{"policy":{"kioskMode":false,"restrictions":{"kioskKeepScreenOn":false}}}""",
            ),
        )

        assertFalse(host.keepScreenOnCalls.last())
    }

    @Test
    fun staysAwakeWhilePluggedInWhenKioskPolicyRequestsIt() {
        val host = FakeHost(deviceOwner = true, inLockTaskMode = false)
        val controller = KioskModeController(host) { }

        controller.apply(
            agentState(
                snapshotJson = """{"policy":{"kioskMode":true,"restrictions":{"kioskStayAwakeWhilePluggedIn":true}}}""",
            ),
        )

        assertTrue(host.stayAwakeWhilePluggedInCalls.last())

        controller.apply(
            agentState(
                snapshotJson = """{"policy":{"kioskMode":false,"restrictions":{"kioskStayAwakeWhilePluggedIn":false}}}""",
            ),
        )

        assertFalse(host.stayAwakeWhilePluggedInCalls.last())
    }

    @Test
    fun keepsKioskExitedUntilPolicyVersionChanges() {
        val host = FakeHost(deviceOwner = true, inLockTaskMode = true)
        val controller = KioskModeController(host) { }

        controller.apply(
            agentState(
                snapshotJson = """{"policy":{"kioskMode":true}}""",
                policyVersion = 7,
                kioskExitSuppressedUntilPolicyVersion = 7,
            ),
        )

        assertTrue(host.stopLockTaskCalls == 1)
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
        kioskExitSuppressedUntilPolicyVersion: Long? = null,
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
            kioskControl = kioskExitSuppressedUntilPolicyVersion?.let {
                KioskControlState(exitSuppressedUntilPolicyVersion = it)
            },
        )
    }

    private data class LaunchCall(
        val packageName: String,
        val lockTaskEnabled: Boolean,
    )

    private class FakeHost(
        override val packageName: String = "com.xmdm.launcher",
        var deviceOwner: Boolean = true,
        var inLockTaskMode: Boolean = false,
        var launchablePackages: MutableSet<String> = mutableSetOf(),
    ) : KioskModeHost {
        var startLockTaskCalls = 0
        var stopLockTaskCalls = 0
        var finishHostActivityCalls = 0
        val keepScreenOnCalls = mutableListOf<Boolean>()
        val stayAwakeWhilePluggedInCalls = mutableListOf<Boolean>()
        val setPackagesCalls = mutableListOf<List<String>>()
        val launchCalls = mutableListOf<LaunchCall>()

        override fun isDeviceOwnerApp(): Boolean = deviceOwner

        override fun isInLockTaskMode(): Boolean = inLockTaskMode

        override fun setKeepScreenOn(keepScreenOn: Boolean) {
            keepScreenOnCalls += keepScreenOn
        }

        override fun setStayAwakeWhilePluggedIn(stayAwakeWhilePluggedIn: Boolean) {
            stayAwakeWhilePluggedInCalls += stayAwakeWhilePluggedIn
        }

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

        override fun finishHostActivity() {
            finishHostActivityCalls += 1
        }

        override fun canLaunchPackage(packageName: String): Boolean {
            return packageName in launchablePackages
        }

        override fun launchPackage(packageName: String, lockTaskEnabled: Boolean): Boolean {
            launchCalls += LaunchCall(packageName = packageName, lockTaskEnabled = lockTaskEnabled)
            if (lockTaskEnabled) {
                inLockTaskMode = true
            }
            return true
        }
    }
}

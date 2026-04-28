package com.xmdm.launcher.packages

import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.PolicyCacheState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PackageRulesControllerTest {
    @Test
    fun suspendsAndUnsuspendsBlockedPackages() {
        val host = FakeHost(deviceOwner = true)
        val controller = PackageRulesController(host) { }

        controller.apply(
            agentState("""{"policy":{"restrictions":{"blockPackages":["com.example.bad","com.xmdm.launcher"],"suspendPackages":["com.example.worse"],"allowPackages":["com.example.good"]}}}"""),
        )

        assertTrue(host.calls.contains(SuspendCall(packages = listOf("com.example.bad", "com.example.worse"), suspended = true)))
        assertFalse(host.calls.any { it.packages.contains("com.xmdm.launcher") })

        controller.apply(
            agentState("""{"policy":{"restrictions":{"allowPackages":["com.example.bad"]}}}"""),
        )

        assertTrue(host.calls.contains(SuspendCall(packages = listOf("com.example.bad", "com.example.worse"), suspended = false)))
        assertEquals(2, host.calls.size)
    }

    @Test
    fun doesNotSuspendUntilDeviceOwnerIsAvailable() {
        val host = FakeHost(deviceOwner = false)
        val controller = PackageRulesController(host) { }

        controller.apply(agentState("""{"policy":{"restrictions":{"blockPackages":["com.example.bad"]}}}"""))

        assertEquals(0, host.calls.size)
    }

    private fun agentState(snapshotJson: String): AgentState {
        return AgentState(
            policyCache = PolicyCacheState(
                snapshotJson = snapshotJson,
                version = 1,
                lastSyncAtEpochMillis = 1L,
            ),
        )
    }

    private data class SuspendCall(
        val packages: List<String>,
        val suspended: Boolean,
    )

    private class FakeHost(
        override val packageName: String = "com.xmdm.launcher",
        var deviceOwner: Boolean = true,
    ) : PackageRulesHost {
        val calls = mutableListOf<SuspendCall>()
        private val suspendedPackages = mutableSetOf<String>()

        override fun isDeviceOwnerApp(): Boolean = deviceOwner

        override fun isPackageSuspended(packageName: String): Boolean {
            return packageName in suspendedPackages
        }

        override fun setPackagesSuspended(packages: Array<String>, suspended: Boolean) {
            calls += SuspendCall(packages = packages.toList(), suspended = suspended)
            if (suspended) {
                suspendedPackages.addAll(packages)
            } else {
                suspendedPackages.removeAll(packages.toSet())
            }
        }
    }
}

package com.xmdm.launcher.state

import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Test

class LauncherEnrollmentStateMachineTest {
    @Test
    fun startsEnrollmentOncePerBootstrapUntilRetry() {
        val machine = LauncherEnrollmentStateMachine()
        val bootstrap = bootstrapState("""{"run":"one"}""")

        machine.onBootstrapReceived("""{"run":"one"}""")
        assertSame(bootstrap, machine.nextEnrollmentBootstrap(agentState(bootstrap)))
        assertNull(machine.nextEnrollmentBootstrap(agentState(bootstrap)))

        machine.onEnrollmentFailed("enrollment", "boom", bootstrap.rawJson)
        machine.onBootstrapReceived("""{"run":"one"}""")
        assertNotNull(machine.nextEnrollmentBootstrap(agentState(bootstrap)))
    }

    @Test
    fun staysEnrolledAfterSuccess() {
        val machine = LauncherEnrollmentStateMachine()
        val bootstrap = bootstrapState("""{"run":"two"}""")

        machine.onBootstrapReceived("""{"run":"two"}""")
        assertNotNull(machine.nextEnrollmentBootstrap(agentState(bootstrap)))
        machine.onEnrollmentSucceeded()

        val enrolledState = agentState(
            bootstrap = bootstrap,
            identity = DeviceIdentityState(
                deviceId = "device-1",
                deviceIdUse = "serial",
                deviceSecret = "secret",
            ),
        )
        assertNull(machine.nextEnrollmentBootstrap(enrolledState))
    }

    private fun agentState(
        bootstrap: BootstrapState,
        identity: DeviceIdentityState? = null,
    ): AgentState {
        return AgentState(
            bootstrap = bootstrap,
            identity = identity,
        )
    }

    private fun bootstrapState(rawJson: String): BootstrapState {
        return BootstrapState(
            serverUrl = "https://mdm.example",
            secondaryServerUrl = null,
            serverProject = "rest",
            enrollmentToken = "token",
            deviceId = "device-1",
            deviceIdUse = "serial",
            bootstrapExtrasJson = "{}",
            rawJson = rawJson,
        )
    }
}

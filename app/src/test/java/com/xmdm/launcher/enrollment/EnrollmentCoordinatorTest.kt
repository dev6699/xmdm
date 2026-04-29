package com.xmdm.launcher.enrollment

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.BootstrapState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files

@OptIn(ExperimentalCoroutinesApi::class)
class EnrollmentCoordinatorTest {
    @Test
    fun enrollsAndPersistsIdentity() = runTest {
        val file = createTempFile("enrollment", ".preferences_pb")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val store = AgentStateStore(
            PreferenceDataStoreFactory.create(
                scope = scope,
                produceFile = { file },
            ),
        )

        val coordinator = EnrollmentCoordinator(
            stateStore = store,
            gateway = object : EnrollmentGateway {
                var request: EnrollmentRequest? = null

                override suspend fun enroll(serverUrl: String, request: EnrollmentRequest): EnrollmentResponse {
                    assertEquals("https://mdm.example", serverUrl)
                    this.request = request
                    return EnrollmentResponse(
                        deviceId = "device-123",
                        deviceSecret = "secret-abc",
                        status = "enrolled",
                    )
                }
            },
        )

        val result = coordinator.enroll(
            BootstrapState(
                serverUrl = "https://mdm.example",
                secondaryServerUrl = null,
                serverProject = "rest",
                enrollmentToken = "enroll-token",
                deviceId = "device-123",
                deviceIdUse = "serial",
                bootstrapExtrasJson = """{"customer":"Acme"}""",
            ),
        )

        assertEquals("device-123", result.identity.deviceId)
        assertEquals("serial", result.identity.deviceIdUse)
        assertEquals("secret-abc", result.identity.deviceSecret)
        assertTrue(store.state.first().isEnrolled)
        scope.cancel()
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsResponseWithMismatchedDeviceId() = runTest {
        val file = createTempFile("enrollment-mismatch", ".preferences_pb")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val store = AgentStateStore(
            PreferenceDataStoreFactory.create(
                scope = scope,
                produceFile = { file },
            ),
        )
        try {
            EnrollmentCoordinator(
                stateStore = store,
                gateway = object : EnrollmentGateway {
                    override suspend fun enroll(serverUrl: String, request: EnrollmentRequest): EnrollmentResponse {
                        return EnrollmentResponse(
                            deviceId = "device-xyz",
                            deviceSecret = "secret-abc",
                            status = "enrolled",
                        )
                    }
                },
            ).enroll(
                BootstrapState(
                    serverUrl = "https://mdm.example",
                    secondaryServerUrl = null,
                    serverProject = "rest",
                    enrollmentToken = "enroll-token",
                    deviceId = "device-123",
                    deviceIdUse = "serial",
                    bootstrapExtrasJson = "{}",
                ),
            )
        } finally {
            scope.cancel()
        }
    }

    private fun createTempFile(prefix: String, suffix: String): File {
        val path = Files.createTempFile(prefix, suffix)
        return path.toFile()
    }
}

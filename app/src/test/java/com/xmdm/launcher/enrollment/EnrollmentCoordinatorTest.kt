package com.xmdm.launcher.enrollment

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.google.gson.Gson
import com.google.gson.JsonObject
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
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset

@OptIn(ExperimentalCoroutinesApi::class)
class EnrollmentCoordinatorTest {
    @Test
    fun enrollsAndPersistsIdentityAndConfigSnapshot() = runTest {
        val file = createTempFile("enrollment", ".preferences_pb")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val store = AgentStateStore(
            PreferenceDataStoreFactory.create(
                scope = scope,
                produceFile = { file },
            ),
        )
        val verifier = com.xmdm.launcher.sync.ConfigSnapshotVerifier()
        val unsigned = """
            {
              "version":"1",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[],
              "commands":[]
            }
        """.trimIndent()
        val config = Gson().fromJson(
            """
            {
              "version":"1",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[],
              "commands":[],
              "signature":"${verifier.sign(unsigned, "secret-abc")}"
            }
            """.trimIndent(),
            JsonObject::class.java,
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
                        config = config,
                    )
                }
            },
            verifier = verifier,
            clock = Clock.fixed(Instant.ofEpochMilli(123456789L), ZoneOffset.UTC),
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
        assertEquals(1L, result.policyCache.version)
        assertEquals(123456789L, result.policyCache.lastSyncAtEpochMillis)
        assertTrue(store.state.first().isEnrolled)
        assertTrue(store.state.first().hasPolicyCache)
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
        val verifier = com.xmdm.launcher.sync.ConfigSnapshotVerifier()
        val unsigned = """
            {
              "version":"1",
              "device":{"deviceId":"device-xyz","deviceIdUse":"serial"},
              "policy":{},
              "apps":[],
              "files":[],
              "certificates":[],
              "commands":[]
            }
        """.trimIndent()
        val config = Gson().fromJson(
            """
            {
              "version":"1",
              "device":{"deviceId":"device-xyz","deviceIdUse":"serial"},
              "policy":{},
              "apps":[],
              "files":[],
              "certificates":[],
              "commands":[],
              "signature":"${verifier.sign(unsigned, "secret-abc")}"
            }
            """.trimIndent(),
            JsonObject::class.java,
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
                            config = config,
                        )
                    }
                },
                verifier = verifier,
                clock = Clock.fixed(Instant.ofEpochMilli(1L), ZoneOffset.UTC),
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

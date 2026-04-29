package com.xmdm.launcher.sync

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.xmdm.launcher.retry.RetryPolicy
import com.xmdm.launcher.retry.Sleeper
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
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
class ConfigSyncEngineTest {
    @Test
    fun retriesTransientFetchFailuresAndPersistsVerifiedSnapshot() = runTest {
        val file = createTempFile("config-sync", ".preferences_pb")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val store = AgentStateStore(
            PreferenceDataStoreFactory.create(
                scope = scope,
                produceFile = { file },
            ),
        )
        val sleeps = mutableListOf<Long>()
        val verifier = ConfigSnapshotVerifier()
        val unsigned = """
            {
              "version":"7",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val signed = """
            {
              "version":"7",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[],
              "signature":"${verifier.sign(unsigned, "secret-abc")}"
            }
        """.trimIndent()

        val engine = ConfigSyncEngine(
            stateStore = store,
            fetcher = object : ConfigSnapshotFetcher {
                var count = 0

                override suspend fun fetch(request: ConfigFetchRequest): String {
                    count += 1
                    if (count < 3) {
                        error("temporary network failure")
                    }
                    assertEquals("https://mdm.example", request.serverUrl)
                    assertEquals("rest", request.serverProject)
                    assertEquals("device-123", request.deviceId)
                    assertEquals("secret-abc", request.deviceSecret)
                    return signed
                }
            },
            verifier = verifier,
            clock = Clock.fixed(Instant.ofEpochMilli(123456789L), ZoneOffset.UTC),
            retryPolicy = RetryPolicy(maxAttempts = 4, initialDelayMs = 10, maxDelayMs = 100),
            sleeper = object : Sleeper {
                override suspend fun sleep(durationMs: Long) {
                    sleeps += durationMs
                }
            },
        )

        val result = engine.sync(
            BootstrapState(
                serverUrl = "https://mdm.example",
                secondaryServerUrl = null,
                serverProject = "rest",
                enrollmentToken = "enroll-token",
                deviceId = null,
                deviceIdUse = null,
                bootstrapExtrasJson = """{"customer":"Acme"}""",
            ),
            DeviceIdentityState(
                deviceId = "device-123",
                deviceIdUse = "serial",
                deviceSecret = "secret-abc",
            ),
        )

        assertEquals(7L, result.version)
        assertEquals(123456789L, result.lastSyncAtEpochMillis)
        assertTrue(store.state.first().hasPolicyCache)
        assertEquals(listOf(10L, 20L), sleeps)
        scope.cancel()
    }

    @Test
    fun fallsBackToSecondaryServerUrlWhenPrimaryFetchPathIsUnavailable() = runTest {
        val file = createTempFile("config-sync-fallback", ".preferences_pb")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val store = AgentStateStore(
            PreferenceDataStoreFactory.create(
                scope = scope,
                produceFile = { file },
            ),
        )
        val sleeps = mutableListOf<Long>()
        val verifier = ConfigSnapshotVerifier()
        val unsigned = """
            {
              "version":"7",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val signed = """
            {
              "version":"7",
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"bootstrapExtras":{"customer":"Acme"}},
              "apps":[],
              "files":[],
              "certificates":[],
              "signature":"${verifier.sign(unsigned, "secret-abc")}"
            }
        """.trimIndent()
        val requests = mutableListOf<String>()

        val engine = ConfigSyncEngine(
            stateStore = store,
            fetcher = object : ConfigSnapshotFetcher {
                override suspend fun fetch(request: ConfigFetchRequest): String {
                    requests += request.serverUrl
                    return when (request.serverUrl) {
                        "https://mdm-primary.example" -> error("primary polling path unavailable")
                        "https://mdm-secondary.example" -> signed
                        else -> error("unexpected server url: ${request.serverUrl}")
                    }
                }
            },
            verifier = verifier,
            clock = Clock.fixed(Instant.ofEpochMilli(123456789L), ZoneOffset.UTC),
            retryPolicy = RetryPolicy(maxAttempts = 2, initialDelayMs = 10, maxDelayMs = 100),
            sleeper = object : Sleeper {
                override suspend fun sleep(durationMs: Long) {
                    sleeps += durationMs
                }
            },
        )

        val result = engine.sync(
            BootstrapState(
                serverUrl = "https://mdm-primary.example",
                secondaryServerUrl = "https://mdm-secondary.example",
                serverProject = "rest",
                enrollmentToken = "enroll-token",
                deviceId = null,
                deviceIdUse = null,
                bootstrapExtrasJson = """{"customer":"Acme"}""",
            ),
            DeviceIdentityState(
                deviceId = "device-123",
                deviceIdUse = "serial",
                deviceSecret = "secret-abc",
            ),
        )

        assertEquals(listOf("https://mdm-primary.example", "https://mdm-secondary.example"), requests)
        assertEquals(7L, result.version)
        assertEquals(123456789L, result.lastSyncAtEpochMillis)
        assertTrue(store.state.first().hasPolicyCache)
        assertEquals(emptyList<Long>(), sleeps)
        scope.cancel()
    }

    @Test(expected = IllegalStateException::class)
    fun doesNotPersistInvalidSnapshot() = runTest {
        val file = createTempFile("config-sync-invalid", ".preferences_pb")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val store = AgentStateStore(
            PreferenceDataStoreFactory.create(
                scope = scope,
                produceFile = { file },
            ),
        )
        val engine = ConfigSyncEngine(
            stateStore = store,
            fetcher = object : ConfigSnapshotFetcher {
                override suspend fun fetch(request: ConfigFetchRequest): String {
                    return """
                        {
                          "version":"7",
                          "device":{"deviceId":"device-123","deviceIdUse":"serial"},
                          "policy":{},
                          "apps":[],
                          "files":[],
                          "certificates":[],
                          "signature":"bogus"
                        }
                    """.trimIndent()
                }
            },
            verifier = ConfigSnapshotVerifier(),
            clock = Clock.fixed(Instant.ofEpochMilli(1L), ZoneOffset.UTC),
            retryPolicy = RetryPolicy(maxAttempts = 2, initialDelayMs = 1, maxDelayMs = 1),
        )

        try {
            engine.sync(
                BootstrapState(
                    serverUrl = "https://mdm.example",
                    secondaryServerUrl = null,
                    serverProject = "rest",
                    enrollmentToken = "enroll-token",
                    deviceId = null,
                    deviceIdUse = null,
                    bootstrapExtrasJson = "{}",
                ),
                DeviceIdentityState(
                    deviceId = "device-123",
                    deviceIdUse = "serial",
                    deviceSecret = "secret-abc",
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

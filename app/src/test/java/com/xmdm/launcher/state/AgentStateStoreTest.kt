package com.xmdm.launcher.state

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.CoroutineScope
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
class AgentStateStoreTest {
    @Test
    fun saveAndReloadState() = runTest {
        val storeFile = createTempFile("agent-state", ".preferences_pb")
        val first = newStore(storeFile)
        val store = first.store

        store.saveBootstrap(
            BootstrapState(
                serverUrl = "https://mdm.example",
                secondaryServerUrl = null,
                enrollmentToken = "enroll-token",
                deviceId = null,
                bootstrapExtrasJson = """{"config":"prod"}""",
                rawJson = """{"com.xmdm.BASE_URL":"https://mdm.example"}""",
            ),
        )
        store.saveDeviceIdentity(
            DeviceIdentityState(
                deviceId = "device-123",
                deviceSecret = "secret-abc",
            ),
        )
        store.savePolicyCache(
            PolicyCacheState(
                snapshotJson = """{"version":"1"}""",
                version = 7,
                lastSyncAtEpochMillis = 123456789L,
            ),
        )
        store.saveManagedApps(
            ManagedAppsState(
                snapshotJson = """{"version":"1","apps":[]}""",
                version = 7,
                lastAppliedAtEpochMillis = 123456999L,
            ),
        )
        store.saveManagedFiles(
            ManagedFilesState(
                snapshotJson = """{"version":"1","files":[]}""",
                version = 7,
                lastAppliedAtEpochMillis = 123456990L,
            ),
        )
        store.saveCertificates(
            CertificatesState(
                snapshotJson = """{"version":"1","certificates":[]}""",
                version = 7,
                lastAppliedAtEpochMillis = 123456980L,
            ),
        )
        store.saveKioskControl(
            KioskControlState(exitSuppressedUntilPolicyVersion = 7L),
        )
        first.scope.cancel()

        val reloaded = newStore(storeFile)
        val state = reloaded.store.state.first()

        assertTrue(state.isBootstrapped)
        assertTrue(state.isEnrolled)
        assertTrue(state.hasPolicyCache)
        assertTrue(state.hasManagedApps)
        assertTrue(state.hasManagedFiles)
        assertTrue(state.hasCertificates)
        assertEquals("https://mdm.example", state.bootstrap?.serverUrl)
        assertEquals("enroll-token", state.bootstrap?.enrollmentToken)
        assertEquals("""{"config":"prod"}""", state.bootstrap?.bootstrapExtrasJson)
        assertEquals("""{"com.xmdm.BASE_URL":"https://mdm.example"}""", state.bootstrap?.rawJson)
        assertEquals("device-123", state.identity?.deviceId)
        assertEquals("secret-abc", state.identity?.deviceSecret)
        assertEquals("""{"version":"1"}""", state.policyCache?.snapshotJson)
        assertEquals(7L, state.policyCache?.version)
        assertEquals(123456789L, state.policyCache?.lastSyncAtEpochMillis)
        assertEquals("""{"version":"1","apps":[]}""", state.managedApps?.snapshotJson)
        assertEquals(7L, state.managedApps?.version)
        assertEquals(123456999L, state.managedApps?.lastAppliedAtEpochMillis)
        assertEquals("""{"version":"1","files":[]}""", state.managedFiles?.snapshotJson)
        assertEquals(7L, state.managedFiles?.version)
        assertEquals(123456990L, state.managedFiles?.lastAppliedAtEpochMillis)
        assertEquals("""{"version":"1","certificates":[]}""", state.certificates?.snapshotJson)
        assertEquals(7L, state.certificates?.version)
        assertEquals(123456980L, state.certificates?.lastAppliedAtEpochMillis)
        assertEquals(7L, state.kioskControl?.exitSuppressedUntilPolicyVersion)
        reloaded.scope.cancel()
    }

    private fun newStore(file: File): TestStore {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val dataStore = PreferenceDataStoreFactory.create(
            scope = scope,
            produceFile = { file },
        )
        return TestStore(scope, AgentStateStore(dataStore))
    }

    private fun createTempFile(prefix: String, suffix: String): File {
        val path = Files.createTempFile(prefix, suffix)
        return path.toFile()
    }

    private data class TestStore(
        val scope: CoroutineScope,
        val store: AgentStateStore,
    )
}

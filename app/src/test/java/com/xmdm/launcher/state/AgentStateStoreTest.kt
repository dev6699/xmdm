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
import org.junit.Assert.assertFalse
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
                serverProject = "rest",
                enrollmentToken = "enroll-token",
                deviceId = null,
                deviceIdUse = null,
                bootstrapExtrasJson = """{"customer":"Acme"}""",
                rawJson = """{"BASE_URL":"https://mdm.example"}""",
            ),
        )
        store.saveDeviceIdentity(
            DeviceIdentityState(
                deviceId = "device-123",
                deviceIdUse = "serial",
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
        first.scope.cancel()

        val reloaded = newStore(storeFile)
        val state = reloaded.store.state.first()

        assertTrue(state.isBootstrapped)
        assertTrue(state.isEnrolled)
        assertTrue(state.hasPolicyCache)
        assertTrue(state.hasManagedApps)
        assertTrue(state.hasManagedFiles)
        assertEquals("https://mdm.example", state.bootstrap?.serverUrl)
        assertEquals("rest", state.bootstrap?.serverProject)
        assertEquals("enroll-token", state.bootstrap?.enrollmentToken)
        assertEquals("""{"customer":"Acme"}""", state.bootstrap?.bootstrapExtrasJson)
        assertEquals("""{"BASE_URL":"https://mdm.example"}""", state.bootstrap?.rawJson)
        assertEquals("device-123", state.identity?.deviceId)
        assertEquals("serial", state.identity?.deviceIdUse)
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

        reloaded.scope.cancel()
    }

    @Test
    fun clearEnrollmentStatePreservesBootstrap() = runTest {
        val storeFile = createTempFile("agent-state", ".preferences_pb")
        val first = newStore(storeFile)
        val store = first.store

        store.saveBootstrap(
            BootstrapState(
                serverUrl = "https://mdm.example",
                secondaryServerUrl = null,
                serverProject = "rest",
                enrollmentToken = "enroll-token",
                deviceId = null,
                deviceIdUse = null,
                bootstrapExtrasJson = "{}",
                rawJson = """{"BASE_URL":"https://mdm.example"}""",
            ),
        )
        store.saveDeviceIdentity(
            DeviceIdentityState(
                deviceId = "device-123",
                deviceIdUse = "serial",
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
        store.clearEnrollmentState()
        first.scope.cancel()

        val second = newStore(storeFile)
        val state = second.store.state.first()
        assertTrue(state.isBootstrapped)
        assertFalse(state.isEnrolled)
        assertFalse(state.hasPolicyCache)
        assertFalse(state.hasManagedApps)
        assertFalse(state.hasManagedFiles)
        assertEquals("https://mdm.example", state.bootstrap?.serverUrl)
        assertEquals("rest", state.bootstrap?.serverProject)
        assertEquals("enroll-token", state.bootstrap?.enrollmentToken)
        assertEquals("""{"BASE_URL":"https://mdm.example"}""", state.bootstrap?.rawJson)
        second.scope.cancel()
    }

    @Test
    fun clearProvisioningStatePreservesManagedApps() = runTest {
        val storeFile = createTempFile("agent-state", ".preferences_pb")
        val first = newStore(storeFile)
        val store = first.store

        store.saveBootstrap(
            BootstrapState(
                serverUrl = "https://mdm.example",
                secondaryServerUrl = null,
                serverProject = "rest",
                enrollmentToken = "enroll-token",
                deviceId = null,
                deviceIdUse = null,
                bootstrapExtrasJson = "{}",
                rawJson = """{"BASE_URL":"https://mdm.example"}""",
            ),
        )
        store.saveDeviceIdentity(
            DeviceIdentityState(
                deviceId = "device-123",
                deviceIdUse = "serial",
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
                snapshotJson = """{"version":"1","apps":[{"packageName":"com.example.old"}]}""",
                version = 7,
                lastAppliedAtEpochMillis = 123456999L,
            ),
        )
        store.saveManagedFiles(
            ManagedFilesState(
                snapshotJson = """{"version":"1","files":[{"path":"device-config.txt"}]}""",
                version = 7,
                lastAppliedAtEpochMillis = 123456990L,
            ),
        )
        store.clearProvisioningState()
        first.scope.cancel()

        val second = newStore(storeFile)
        val state = second.store.state.first()
        assertFalse(state.isBootstrapped)
        assertFalse(state.isEnrolled)
        assertFalse(state.hasPolicyCache)
        assertTrue(state.hasManagedApps)
        assertTrue(state.hasManagedFiles)
        assertEquals("""{"version":"1","apps":[{"packageName":"com.example.old"}]}""", state.managedApps?.snapshotJson)
        assertEquals(7L, state.managedApps?.version)
        assertEquals("""{"version":"1","files":[{"path":"device-config.txt"}]}""", state.managedFiles?.snapshotJson)
        assertEquals(7L, state.managedFiles?.version)
        second.scope.cancel()
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

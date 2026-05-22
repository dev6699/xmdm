package com.xmdm.launcher.bootstrap

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.xmdm.launcher.state.AgentStateStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files

class BootstrapProvisionerTest {
    @Test
    fun persistsParsedBootstrapState() = runTest {
        val file = createTempFile("bootstrap-state", ".preferences_pb")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        val store = AgentStateStore(
            PreferenceDataStoreFactory.create(
                scope = scope,
                produceFile = { file },
            ),
        )
        val provisioner = BootstrapProvisioner(store)

        provisioner.persist(
            """
            {
              "android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE": {
                "com.xmdm.BASE_URL": "https://mdm.example",
                "com.xmdm.ENROLLMENT_TOKEN": "token",
                "com.xmdm.DEVICE_ID": "serial-123"
              }
            }
            """.trimIndent(),
        )

        val state = store.state.first()
        assertTrue(state.isBootstrapped)
        assertEquals("https://mdm.example", state.bootstrap?.serverUrl)
        assertEquals("token", state.bootstrap?.enrollmentToken)
        assertEquals("serial-123", state.bootstrap?.deviceId)
        assertEquals("{}", state.bootstrap?.bootstrapExtrasJson)

        scope.cancel()
    }

    private fun createTempFile(prefix: String, suffix: String): File {
        val path = Files.createTempFile(prefix, suffix)
        return path.toFile()
    }
}

package com.xmdm.launcher.logs

import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlinx.coroutines.CancellationException
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset

class DeviceLogCoordinatorTest {
    @Test
    fun uploadsQueuedLogsAndClearsBuffer() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway()
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = Clock.fixed(Instant.parse("2026-04-30T12:00:00Z"), ZoneOffset.UTC),
        )

        coordinator.record("launcher", "info", "started")
        coordinator.record("sync", "warn", "failed", mapOf("error" to "boom"))

        val uploaded = coordinator.upload(
            bootstrap = bootstrap(),
            identity = identity(),
        )

        assertEquals(2, uploaded)
        assertEquals(1, gateway.uploads.size)
        assertEquals("https://mdm.example", gateway.uploads[0].serverUrl)
        assertEquals("device-123", gateway.uploads[0].deviceId)
        assertEquals("secret-abc", gateway.uploads[0].deviceSecret)
        assertEquals(2, gateway.uploads[0].request.entries.size)
        assertTrue(queue.entries.isEmpty())
    }

    @Test
    fun restoresQueuedLogsWhenUploadFails() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway(failUpload = true)
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = Clock.fixed(Instant.parse("2026-04-30T12:00:00Z"), ZoneOffset.UTC),
        )

        coordinator.record("launcher", "info", "started")

        try {
            coordinator.upload(
                bootstrap = bootstrap(),
                identity = identity(),
            )
        } catch (_: Throwable) {
        }

        assertEquals(1, queue.entries.size)
        assertEquals("started", queue.entries[0].message)
    }

    @Test
    fun restoresQueuedLogsWhenUploadIsCancelled() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway(cancelUpload = true)
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = Clock.fixed(Instant.parse("2026-04-30T12:00:00Z"), ZoneOffset.UTC),
        )

        coordinator.record("launcher", "info", "started")

        try {
            coordinator.upload(
                bootstrap = bootstrap(),
                identity = identity(),
            )
        } catch (failure: Throwable) {
            assertTrue(failure is CancellationException)
        }

        assertEquals(1, queue.entries.size)
        assertEquals("started", queue.entries[0].message)
    }

    private fun bootstrap(): BootstrapState {
        return BootstrapState(
            serverUrl = "https://mdm.example",
            secondaryServerUrl = null,
            serverProject = "rest",
            enrollmentToken = "enroll-token",
            deviceId = null,
            deviceIdUse = null,
            bootstrapExtrasJson = "{}",
            rawJson = """{"serverUrl":"https://mdm.example"}""",
        )
    }

    private fun identity(): DeviceIdentityState {
        return DeviceIdentityState(
            deviceId = "device-123",
            deviceIdUse = "serial",
            deviceSecret = "secret-abc",
        )
    }

    private class RecordingQueue : DeviceLogQueue {
        val entries = mutableListOf<DeviceLogEntry>()

        override suspend fun append(entry: DeviceLogEntry) {
            entries += entry
        }

        override suspend fun drain(): List<DeviceLogEntry> {
            val drained = entries.toList()
            entries.clear()
            return drained
        }

        override suspend fun prepend(entries: List<DeviceLogEntry>) {
            this.entries.addAll(0, entries)
        }
    }

    private class RecordingGateway(
        private val failUpload: Boolean = false,
        private val cancelUpload: Boolean = false,
    ) : DeviceLogGateway {
        val uploads = mutableListOf<UploadCall>()

        override suspend fun upload(
            serverUrl: String,
            deviceId: String,
            deviceSecret: String,
            request: DeviceLogUploadRequest,
        ) {
            uploads += UploadCall(serverUrl, deviceId, deviceSecret, request)
            if (cancelUpload) {
                throw CancellationException("cancelled")
            }
            if (failUpload) {
                error("boom")
            }
        }
    }

    private data class UploadCall(
        val serverUrl: String,
        val deviceId: String,
        val deviceSecret: String,
        val request: DeviceLogUploadRequest,
    )
}

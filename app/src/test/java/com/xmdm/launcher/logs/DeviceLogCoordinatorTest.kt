package com.xmdm.launcher.logs

import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset

class DeviceLogCoordinatorTest {
    @Test
    fun uploadsQueuedLogsAndRemovesUploadedEntries() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway()
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = fixedClock(),
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
        assertEquals(1, gateway.uploads[0].request.schemaVersion)
        assertEquals("2026-04-30T12:00:00Z", gateway.uploads[0].request.observedAt)
        assertEquals(2, gateway.uploads[0].request.entries.size)
        assertEquals("2026-04-30T12:00:00Z", gateway.uploads[0].request.entries[0].observedAt)
        assertEquals("launcher", gateway.uploads[0].request.entries[0].source)
        assertEquals("started", gateway.uploads[0].request.entries[0].message)
        assertTrue(gateway.uploads[0].request.entries.all { it.id.isNotBlank() })
        assertNotEquals(gateway.uploads[0].request.entries[0].id, gateway.uploads[0].request.entries[1].id)
        assertTrue(queue.entries.isEmpty())
    }

    @Test
    fun keepsQueuedLogsWhenUploadFails() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway(failUpload = true)
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = fixedClock(),
        )

        coordinator.record("launcher", "info", "started")
        val queuedId = queue.entries.single().id

        try {
            coordinator.upload(
                bootstrap = bootstrap(),
                identity = identity(),
            )
        } catch (_: Throwable) {
        }

        assertEquals(1, gateway.uploads.size)
        assertEquals(1, queue.entries.size)
        assertEquals(queuedId, queue.entries[0].id)
        assertEquals("started", queue.entries[0].message)
    }

    @Test
    fun keepsQueuedLogsWhenUploadIsCancelled() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway(cancelUpload = true)
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = fixedClock(),
        )

        coordinator.record("launcher", "info", "started")
        val queuedId = queue.entries.single().id

        try {
            coordinator.upload(
                bootstrap = bootstrap(),
                identity = identity(),
            )
        } catch (failure: Throwable) {
            assertTrue(failure is CancellationException)
        }

        assertEquals(1, gateway.uploads.size)
        assertEquals(1, queue.entries.size)
        assertEquals(queuedId, queue.entries[0].id)
        assertEquals("started", queue.entries[0].message)
    }

    @Test
    fun uploadsAtMostOneBatchAndLeavesRemainingEntries() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway()
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = fixedClock(),
        )

        repeat(55) { index ->
            coordinator.record("test", "info", "entry-$index")
        }

        val uploaded = coordinator.upload(
            bootstrap = bootstrap(),
            identity = identity(),
        )

        assertEquals(50, uploaded)
        assertEquals(1, gateway.uploads.size)
        assertEquals(50, gateway.uploads[0].request.entries.size)
        assertEquals(5, queue.entries.size)
        assertEquals("entry-50", queue.entries.first().message)
        assertEquals("entry-54", queue.entries.last().message)
    }

    @Test
    fun fallsBackToSecondaryServerWithoutRequeueingMarker() = runTest {
        val queue = RecordingQueue()
        val gateway = RecordingGateway(failPrimaryOnly = true)
        val coordinator = DeviceLogCoordinator(
            queue = queue,
            gateway = gateway,
            clock = fixedClock(),
        )

        coordinator.record("launcher", "info", "started")

        val uploaded = coordinator.upload(
            bootstrap = bootstrap(secondaryServerUrl = "https://backup.example"),
            identity = identity(),
        )

        assertEquals(1, uploaded)
        assertEquals(2, gateway.uploads.size)
        assertEquals("https://mdm.example", gateway.uploads[0].serverUrl)
        assertEquals("https://backup.example", gateway.uploads[1].serverUrl)
        assertTrue(queue.entries.isEmpty())
    }

    private fun fixedClock(): Clock {
        return Clock.fixed(Instant.parse("2026-04-30T12:00:00Z"), ZoneOffset.UTC)
    }

    private fun bootstrap(secondaryServerUrl: String? = null): BootstrapState {
        return BootstrapState(
            serverUrl = "https://mdm.example",
            secondaryServerUrl = secondaryServerUrl,
            enrollmentToken = "enroll-token",
            deviceId = null,
            bootstrapExtrasJson = "{}",
            rawJson = """{"serverUrl":"https://mdm.example"}""",
        )
    }

    private fun identity(): DeviceIdentityState {
        return DeviceIdentityState(
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )
    }

    private class RecordingQueue : DeviceLogQueue {
        val entries = mutableListOf<DeviceLogEntry>()

        override suspend fun append(entry: DeviceLogEntry) {
            entries += entry
        }

        override suspend fun count(): Int {
            return entries.size
        }

        override suspend fun peekBatch(limit: Int): List<DeviceLogEntry> {
            return if (limit <= 0) emptyList() else entries.take(limit)
        }

        override suspend fun remove(ids: Set<String>) {
            entries.removeAll { it.id in ids }
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
        private val failPrimaryOnly: Boolean = false,
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
            if (failUpload || (failPrimaryOnly && serverUrl == "https://mdm.example")) {
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

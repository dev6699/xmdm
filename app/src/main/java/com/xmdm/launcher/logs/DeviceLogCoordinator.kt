package com.xmdm.launcher.logs

import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.time.Clock
import kotlin.math.min
import kotlin.random.Random

class DeviceLogCoordinator(
    private val queue: DeviceLogQueue,
    private val gateway: DeviceLogGateway,
    private val sessionProvider: suspend () -> Pair<BootstrapState, DeviceIdentityState>? = { null },
    private val clock: Clock = Clock.systemUTC(),
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val uploadMutex = Mutex()
    private val stateMutex = Mutex()

    private var flushJob: Job? = null
    private var retryDelayMs = INITIAL_RETRY_DELAY_MS
    private var consecutiveUploadFailures = 0
    private var firstUploadFailureAtEpochMillis: Long? = null
    private var lastUploadFailureType: String? = null
    private var lastUploadFailureMessage: String? = null

    suspend fun record(
        source: String,
        level: String,
        message: String,
        payload: Map<String, Any?>? = null,
    ) {
        queue.append(
            DeviceLogEntry(
                observedAt = clock.instant().toString(),
                source = source,
                level = level,
                message = message,
                payload = payload?.takeIf { it.isNotEmpty() },
            ),
        )

        val activeSession = sessionProvider() ?: return
        val pendingCount = queue.count()
        if (pendingCount >= FLUSH_ENTRY_THRESHOLD) {
            triggerImmediateFlush(activeSession)
            return
        }

        scheduleTimedFlushIfNeeded()
    }

    suspend fun flushPendingLogsIfSessionAvailable() {
        val activeSession = sessionProvider() ?: return
        if (queue.count() > 0) {
            triggerImmediateFlush(activeSession)
        }
    }

    suspend fun upload(bootstrap: BootstrapState, identity: DeviceIdentityState): Int {
        val batch = queue.peekBatch(MAX_BATCH_ENTRIES)
        if (batch.isEmpty()) {
            return 0
        }
        postBatch(bootstrap, identity, batch)
        queue.remove(batch.map { it.id }.toSet())
        return batch.size
    }

    private fun triggerImmediateFlush(session: Pair<BootstrapState, DeviceIdentityState>) {
        scope.launch {
            flushCurrentSession(session)
        }
    }

    private suspend fun scheduleTimedFlushIfNeeded() {
        scheduleFlush(FLUSH_DELAY_MS)
    }

    private suspend fun scheduleRetryFlush() {
        val jitterMs = Random.nextLong(0L, RETRY_JITTER_MS + 1L)
        val delayMs = retryDelayMs + jitterMs
        retryDelayMs = min(retryDelayMs * 2, MAX_RETRY_DELAY_MS)
        scheduleFlush(delayMs)
    }

    private suspend fun scheduleFlush(delayMs: Long) {
        stateMutex.withLock {
            if (flushJob?.isActive == true) {
                return
            }
            val job = scope.launch {
                delay(delayMs)
                flushCurrentSession()
            }
            flushJob = job
            job.invokeOnCompletion {
                scope.launch {
                    stateMutex.withLock {
                        if (flushJob === job) {
                            flushJob = null
                        }
                    }
                }
            }
        }
    }

    private suspend fun flushCurrentSession(
        sessionOverride: Pair<BootstrapState, DeviceIdentityState>? = null,
    ) {
        val activeSession = sessionOverride ?: sessionProvider() ?: return
        uploadMutex.withLock {
            stateMutex.withLock {
                flushJob = null
            }
            var uploadedTotal = 0
            try {
                var batchIndex = 0
                while (batchIndex < MAX_BATCHES_PER_FLUSH) {
                    val uploaded = upload(activeSession.first, activeSession.second)
                    if (uploaded <= 0) {
                        break
                    }
                    uploadedTotal += uploaded
                    batchIndex += 1
                    if (batchIndex < MAX_BATCHES_PER_FLUSH && queue.count() > 0) {
                        delay(RECOVERY_BATCH_DELAY_MS)
                    }
                }
                if (uploadedTotal > 0) {
                    recordUploadRecoveredIfNeeded(uploadedTotal)
                    retryDelayMs = INITIAL_RETRY_DELAY_MS
                    consecutiveUploadFailures = 0
                    firstUploadFailureAtEpochMillis = null
                    lastUploadFailureType = null
                    lastUploadFailureMessage = null
                }
                if (queue.count() > 0) {
                    scheduleTimedFlushIfNeeded()
                }
            } catch (failure: Throwable) {
                if (failure is CancellationException) {
                    throw failure
                }
                rememberUploadFailure(failure)
                scheduleRetryFlush()
            }
        }
    }

    private suspend fun recordUploadRecoveredIfNeeded(uploadedCount: Int) {
        val failureCount = consecutiveUploadFailures
        if (failureCount <= 0) {
            return
        }
        val firstFailureAt = firstUploadFailureAtEpochMillis
        queue.append(
            DeviceLogEntry(
                observedAt = clock.instant().toString(),
                source = "logs",
                level = "info",
                message = "device log upload recovered",
                payload = mapOf(
                    "consecutiveFailures" to failureCount,
                    "firstFailureAtEpochMillis" to firstFailureAt,
                    "recoveredAtEpochMillis" to clock.millis(),
                    "offlineDurationMs" to firstFailureAt?.let { clock.millis() - it },
                    "uploadedCount" to uploadedCount,
                    "lastErrorType" to lastUploadFailureType,
                    "lastErrorMessage" to lastUploadFailureMessage,
                ),
            ),
        )
    }

    private fun rememberUploadFailure(failure: Throwable) {
        consecutiveUploadFailures += 1
        if (firstUploadFailureAtEpochMillis == null) {
            firstUploadFailureAtEpochMillis = clock.millis()
        }
        lastUploadFailureType = failure.javaClass.simpleName
        lastUploadFailureMessage = failure.message?.take(MAX_ERROR_MESSAGE_CHARS)
    }

    private suspend fun postBatch(
        bootstrap: BootstrapState,
        identity: DeviceIdentityState,
        batch: List<DeviceLogEntry>,
    ) {
        val serverUrls = buildList {
            add(bootstrap.serverUrl)
            bootstrap.secondaryServerUrl
                ?.trim()
                ?.takeIf { it.isNotEmpty() && it != bootstrap.serverUrl }
                ?.let { add(it) }
        }
        var lastFailure: Throwable? = null
        for ((index, serverUrl) in serverUrls.withIndex()) {
            try {
                gateway.upload(
                    serverUrl = serverUrl,
                    deviceId = identity.deviceId,
                    deviceSecret = identity.deviceSecret,
                    request = DeviceLogUploadRequest(
                        observedAt = clock.instant().toString(),
                        entries = batch,
                    ),
                )
                return
            } catch (failure: Throwable) {
                if (failure is CancellationException) {
                    throw failure
                }
                lastFailure = failure
            }
        }
        throw lastFailure ?: error("device log upload failed without a recorded error")
    }

    private companion object {
        const val FLUSH_ENTRY_THRESHOLD = 5
        const val FLUSH_DELAY_MS = 30_000L
        const val MAX_BATCH_ENTRIES = 50
        const val MAX_BATCHES_PER_FLUSH = 3
        const val RECOVERY_BATCH_DELAY_MS = 5_000L
        const val INITIAL_RETRY_DELAY_MS = 30_000L
        const val MAX_RETRY_DELAY_MS = 15 * 60_000L
        const val RETRY_JITTER_MS = 5_000L
        const val MAX_ERROR_MESSAGE_CHARS = 200
    }
}

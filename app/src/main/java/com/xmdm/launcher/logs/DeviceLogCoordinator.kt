package com.xmdm.launcher.logs

import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.time.Clock

class DeviceLogCoordinator(
    private val queue: DeviceLogQueue,
    private val gateway: DeviceLogGateway,
    private val clock: Clock = Clock.systemUTC(),
) {
    private val mutex = Mutex()

    suspend fun record(
        source: String,
        level: String,
        message: String,
        payload: Map<String, Any?>? = null,
    ) {
        mutex.withLock {
            queue.append(
                DeviceLogEntry(
                    observedAt = clock.instant().toString(),
                    source = source,
                    level = level,
                    message = message,
                    payload = payload?.takeIf { it.isNotEmpty() },
                ),
            )
        }
    }

    suspend fun upload(bootstrap: BootstrapState, identity: DeviceIdentityState): Int {
        val batch = mutex.withLock { queue.drain() }
        if (batch.isEmpty()) {
            return 0
        }
        try {
            postBatch(bootstrap, identity, batch)
            return batch.size
        } catch (failure: Throwable) {
            mutex.withLock {
                queue.prepend(batch)
            }
            if (failure is CancellationException) {
                throw failure
            }
            throw failure
        }
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
        for (serverUrl in serverUrls) {
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
}

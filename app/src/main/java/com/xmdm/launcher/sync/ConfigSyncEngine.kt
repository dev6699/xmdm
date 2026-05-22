package com.xmdm.launcher.sync

import com.xmdm.launcher.retry.RetryPolicy
import com.xmdm.launcher.retry.CoroutineSleeper
import com.xmdm.launcher.retry.Sleeper
import com.xmdm.launcher.retry.retrying
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import com.xmdm.launcher.state.PolicyCacheState
import kotlinx.coroutines.CancellationException
import java.time.Clock

data class ConfigFetchRequest(
    val serverUrl: String,
    val deviceId: String,
    val deviceSecret: String,
)

interface ConfigSnapshotFetcher {
    suspend fun fetch(request: ConfigFetchRequest): String
}

class ConfigSyncEngine(
    private val stateStore: AgentStateStore,
    private val fetcher: ConfigSnapshotFetcher,
    private val verifier: ConfigSnapshotVerifier = ConfigSnapshotVerifier(),
    private val clock: Clock = Clock.systemUTC(),
    private val retryPolicy: RetryPolicy = RetryPolicy(),
    private val sleeper: Sleeper? = null,
) {
    suspend fun sync(bootstrap: BootstrapState, identity: DeviceIdentityState): PolicyCacheState {
        val rawSnapshot = retrying(policy = retryPolicy, sleeper = sleeper ?: CoroutineSleeper) { _ ->
            fetchSnapshot(bootstrap, identity)
        }
        val verified = verifier.verify(rawSnapshot, identity.deviceSecret)
        val version = verified.get("version")?.takeIf { !it.isJsonNull }?.asString?.toLongOrNull()
            ?: error("config snapshot version must be a numeric string")
        val cached = PolicyCacheState(
            snapshotJson = rawSnapshot,
            version = version,
            lastSyncAtEpochMillis = clock.millis(),
        )
        stateStore.savePolicyCache(cached)
        return cached
    }

    private suspend fun fetchSnapshot(bootstrap: BootstrapState, identity: DeviceIdentityState): String {
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
                return fetcher.fetch(
                    ConfigFetchRequest(
                        serverUrl = serverUrl,
                        deviceId = identity.deviceId,
                        deviceSecret = identity.deviceSecret,
                    ),
                )
            } catch (failure: Throwable) {
                if (failure is CancellationException) {
                    throw failure
                }
                lastFailure = failure
            }
        }

        throw lastFailure ?: error("config snapshot fetch failed without a recorded error")
    }
}

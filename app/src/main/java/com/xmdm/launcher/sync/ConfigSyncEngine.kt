package com.xmdm.launcher.sync

import com.xmdm.launcher.retry.RetryPolicy
import com.xmdm.launcher.retry.CoroutineSleeper
import com.xmdm.launcher.retry.Sleeper
import com.xmdm.launcher.retry.retrying
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import com.xmdm.launcher.state.PolicyCacheState
import java.time.Clock

data class ConfigFetchRequest(
    val serverUrl: String,
    val serverProject: String,
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
            fetcher.fetch(
                ConfigFetchRequest(
                    serverUrl = bootstrap.serverUrl,
                    serverProject = bootstrap.serverProject,
                    deviceId = identity.deviceId,
                    deviceSecret = identity.deviceSecret,
                ),
            )
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
}

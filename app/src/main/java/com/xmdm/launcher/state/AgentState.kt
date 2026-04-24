package com.xmdm.launcher.state

data class BootstrapState(
    val serverUrl: String,
    val serverProject: String,
    val enrollmentToken: String,
    val bootstrapExtrasJson: String,
)

data class DeviceIdentityState(
    val deviceId: String,
    val deviceIdUse: String,
    val deviceSecret: String,
)

data class PolicyCacheState(
    val snapshotJson: String,
    val version: Long,
    val lastSyncAtEpochMillis: Long,
)

data class AgentState(
    val bootstrap: BootstrapState? = null,
    val identity: DeviceIdentityState? = null,
    val policyCache: PolicyCacheState? = null,
) {
    val isBootstrapped: Boolean
        get() = bootstrap != null

    val isEnrolled: Boolean
        get() = identity != null

    val hasPolicyCache: Boolean
        get() = policyCache != null

    companion object {
        fun empty(): AgentState = AgentState()
    }
}

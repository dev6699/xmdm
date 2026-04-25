package com.xmdm.launcher.state

data class BootstrapState(
    val serverUrl: String,
    val secondaryServerUrl: String?,
    val serverProject: String,
    val enrollmentToken: String,
    val deviceId: String?,
    val deviceIdUse: String?,
    val bootstrapExtrasJson: String,
    val rawJson: String? = null,
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

data class ManagedAppsState(
    val snapshotJson: String,
    val version: Long,
    val lastAppliedAtEpochMillis: Long,
)

data class ManagedFilesState(
    val snapshotJson: String,
    val version: Long,
    val lastAppliedAtEpochMillis: Long,
)

data class AgentState(
    val bootstrap: BootstrapState? = null,
    val identity: DeviceIdentityState? = null,
    val policyCache: PolicyCacheState? = null,
    val managedApps: ManagedAppsState? = null,
    val managedFiles: ManagedFilesState? = null,
) {
    val isBootstrapped: Boolean
        get() = bootstrap != null

    val isEnrolled: Boolean
        get() = identity != null

    val hasPolicyCache: Boolean
        get() = policyCache != null

    val hasManagedApps: Boolean
        get() = managedApps != null

    val hasManagedFiles: Boolean
        get() = managedFiles != null

    companion object {
        fun empty(): AgentState = AgentState()
    }
}

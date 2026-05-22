package com.xmdm.launcher.state

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.agentStateDataStore by preferencesDataStore(name = "agent_state")

class AgentStateStore(
    private val dataStore: DataStore<Preferences>,
) {
    val state: Flow<AgentState> = dataStore.data.map(::readState)

    suspend fun saveBootstrap(state: BootstrapState) {
        dataStore.edit { prefs ->
            prefs[Keys.BOOTSTRAP_SERVER_URL] = state.serverUrl
            if (state.secondaryServerUrl == null) {
                prefs.remove(Keys.BOOTSTRAP_SECONDARY_SERVER_URL)
            } else {
                prefs[Keys.BOOTSTRAP_SECONDARY_SERVER_URL] = state.secondaryServerUrl
            }
            prefs[Keys.BOOTSTRAP_ENROLLMENT_TOKEN] = state.enrollmentToken
            if (state.deviceId == null) {
                prefs.remove(Keys.BOOTSTRAP_DEVICE_ID)
            } else {
                prefs[Keys.BOOTSTRAP_DEVICE_ID] = state.deviceId
            }
            prefs[Keys.BOOTSTRAP_EXTRAS_JSON] = state.bootstrapExtrasJson
            if (state.rawJson == null) {
                prefs.remove(Keys.BOOTSTRAP_RAW_JSON)
            } else {
                prefs[Keys.BOOTSTRAP_RAW_JSON] = state.rawJson
            }
        }
    }

    suspend fun saveDeviceIdentity(state: DeviceIdentityState) {
        dataStore.edit { prefs ->
            prefs[Keys.DEVICE_ID] = state.deviceId
            prefs[Keys.DEVICE_SECRET] = state.deviceSecret
        }
    }

    suspend fun savePolicyCache(state: PolicyCacheState) {
        dataStore.edit { prefs ->
            prefs[Keys.POLICY_SNAPSHOT_JSON] = state.snapshotJson
            prefs[Keys.POLICY_VERSION] = state.version
            prefs[Keys.POLICY_LAST_SYNC_AT_EPOCH_MILLIS] = state.lastSyncAtEpochMillis
        }
    }

    suspend fun saveManagedApps(state: ManagedAppsState) {
        dataStore.edit { prefs ->
            prefs[Keys.MANAGED_APPS_SNAPSHOT_JSON] = state.snapshotJson
            prefs[Keys.MANAGED_APPS_VERSION] = state.version
            prefs[Keys.MANAGED_APPS_LAST_APPLIED_AT_EPOCH_MILLIS] = state.lastAppliedAtEpochMillis
        }
    }

    suspend fun saveManagedFiles(state: ManagedFilesState) {
        dataStore.edit { prefs ->
            prefs[Keys.MANAGED_FILES_SNAPSHOT_JSON] = state.snapshotJson
            prefs[Keys.MANAGED_FILES_VERSION] = state.version
            prefs[Keys.MANAGED_FILES_LAST_APPLIED_AT_EPOCH_MILLIS] = state.lastAppliedAtEpochMillis
        }
    }

    suspend fun saveCertificates(state: CertificatesState) {
        dataStore.edit { prefs ->
            prefs[Keys.CERTIFICATES_SNAPSHOT_JSON] = state.snapshotJson
            prefs[Keys.CERTIFICATES_VERSION] = state.version
            prefs[Keys.CERTIFICATES_LAST_APPLIED_AT_EPOCH_MILLIS] = state.lastAppliedAtEpochMillis
        }
    }

    suspend fun saveKioskControl(state: KioskControlState) {
        dataStore.edit { prefs ->
            if (state.exitSuppressedUntilPolicyVersion == null) {
                prefs.remove(Keys.KIOSK_EXIT_SUPPRESSED_UNTIL_POLICY_VERSION)
            } else {
                prefs[Keys.KIOSK_EXIT_SUPPRESSED_UNTIL_POLICY_VERSION] = state.exitSuppressedUntilPolicyVersion
            }
        }
    }

    suspend fun clearCertificates() {
        dataStore.edit { prefs ->
            prefs.remove(Keys.CERTIFICATES_SNAPSHOT_JSON)
            prefs.remove(Keys.CERTIFICATES_VERSION)
            prefs.remove(Keys.CERTIFICATES_LAST_APPLIED_AT_EPOCH_MILLIS)
        }
    }

    suspend fun clearAllState() {
        dataStore.edit { prefs ->
            prefs.clear()
        }
    }

    private fun readState(prefs: Preferences): AgentState {
        val bootstrap = bootstrapFromPrefs(prefs)
        val identity = identityFromPrefs(prefs)
        val policyCache = policyCacheFromPrefs(prefs)
        val managedApps = managedAppsFromPrefs(prefs)
        val managedFiles = managedFilesFromPrefs(prefs)
        val certificates = certificatesFromPrefs(prefs)
        val kioskControl = kioskControlFromPrefs(prefs)
        return AgentState(
            bootstrap = bootstrap,
            identity = identity,
            policyCache = policyCache,
            managedApps = managedApps,
            managedFiles = managedFiles,
            certificates = certificates,
            kioskControl = kioskControl,
        )
    }

    private fun bootstrapFromPrefs(prefs: Preferences): BootstrapState? {
        val serverUrl = prefs[Keys.BOOTSTRAP_SERVER_URL] ?: return null
        val secondaryServerUrl = prefs[Keys.BOOTSTRAP_SECONDARY_SERVER_URL]
        val enrollmentToken = prefs[Keys.BOOTSTRAP_ENROLLMENT_TOKEN] ?: return null
        val deviceId = prefs[Keys.BOOTSTRAP_DEVICE_ID]
        val bootstrapExtrasJson = prefs[Keys.BOOTSTRAP_EXTRAS_JSON] ?: "{}"
        val rawJson = prefs[Keys.BOOTSTRAP_RAW_JSON]
        return BootstrapState(
            serverUrl = serverUrl,
            secondaryServerUrl = secondaryServerUrl,
            enrollmentToken = enrollmentToken,
            deviceId = deviceId,
            bootstrapExtrasJson = bootstrapExtrasJson,
            rawJson = rawJson,
        )
    }

    private fun identityFromPrefs(prefs: Preferences): DeviceIdentityState? {
        val deviceId = prefs[Keys.DEVICE_ID] ?: return null
        val deviceSecret = prefs[Keys.DEVICE_SECRET] ?: return null
        return DeviceIdentityState(
            deviceId = deviceId,
            deviceSecret = deviceSecret,
        )
    }

    private fun policyCacheFromPrefs(prefs: Preferences): PolicyCacheState? {
        val snapshotJson = prefs[Keys.POLICY_SNAPSHOT_JSON] ?: return null
        val version = prefs[Keys.POLICY_VERSION] ?: return null
        val lastSyncAtEpochMillis = prefs[Keys.POLICY_LAST_SYNC_AT_EPOCH_MILLIS] ?: return null
        return PolicyCacheState(
            snapshotJson = snapshotJson,
            version = version,
            lastSyncAtEpochMillis = lastSyncAtEpochMillis,
        )
    }

    private fun managedAppsFromPrefs(prefs: Preferences): ManagedAppsState? {
        val snapshotJson = prefs[Keys.MANAGED_APPS_SNAPSHOT_JSON] ?: return null
        val version = prefs[Keys.MANAGED_APPS_VERSION] ?: return null
        val lastAppliedAtEpochMillis = prefs[Keys.MANAGED_APPS_LAST_APPLIED_AT_EPOCH_MILLIS] ?: return null
        return ManagedAppsState(
            snapshotJson = snapshotJson,
            version = version,
            lastAppliedAtEpochMillis = lastAppliedAtEpochMillis,
        )
    }

    private fun managedFilesFromPrefs(prefs: Preferences): ManagedFilesState? {
        val snapshotJson = prefs[Keys.MANAGED_FILES_SNAPSHOT_JSON] ?: return null
        val version = prefs[Keys.MANAGED_FILES_VERSION] ?: return null
        val lastAppliedAtEpochMillis = prefs[Keys.MANAGED_FILES_LAST_APPLIED_AT_EPOCH_MILLIS] ?: return null
        return ManagedFilesState(
            snapshotJson = snapshotJson,
            version = version,
            lastAppliedAtEpochMillis = lastAppliedAtEpochMillis,
        )
    }

    private fun certificatesFromPrefs(prefs: Preferences): CertificatesState? {
        val snapshotJson = prefs[Keys.CERTIFICATES_SNAPSHOT_JSON] ?: return null
        val version = prefs[Keys.CERTIFICATES_VERSION] ?: return null
        val lastAppliedAtEpochMillis = prefs[Keys.CERTIFICATES_LAST_APPLIED_AT_EPOCH_MILLIS] ?: return null
        return CertificatesState(
            snapshotJson = snapshotJson,
            version = version,
            lastAppliedAtEpochMillis = lastAppliedAtEpochMillis,
        )
    }

    private fun kioskControlFromPrefs(prefs: Preferences): KioskControlState? {
        val exitSuppressedUntilPolicyVersion = prefs[Keys.KIOSK_EXIT_SUPPRESSED_UNTIL_POLICY_VERSION]
        return if (exitSuppressedUntilPolicyVersion == null) {
            null
        } else {
            KioskControlState(exitSuppressedUntilPolicyVersion = exitSuppressedUntilPolicyVersion)
        }
    }

    private object Keys {
        val BOOTSTRAP_SERVER_URL = stringPreferencesKey("bootstrap_server_url")
        val BOOTSTRAP_SECONDARY_SERVER_URL = stringPreferencesKey("bootstrap_secondary_server_url")
        val BOOTSTRAP_ENROLLMENT_TOKEN = stringPreferencesKey("bootstrap_enrollment_token")
        val BOOTSTRAP_DEVICE_ID = stringPreferencesKey("bootstrap_device_id")
        val BOOTSTRAP_EXTRAS_JSON = stringPreferencesKey("bootstrap_extras_json")
        val BOOTSTRAP_RAW_JSON = stringPreferencesKey("bootstrap_raw_json")

        val DEVICE_ID = stringPreferencesKey("device_id")
        val DEVICE_SECRET = stringPreferencesKey("device_secret")

        val POLICY_SNAPSHOT_JSON = stringPreferencesKey("policy_snapshot_json")
        val POLICY_VERSION = longPreferencesKey("policy_version")
        val POLICY_LAST_SYNC_AT_EPOCH_MILLIS = longPreferencesKey("policy_last_sync_at_epoch_millis")
        val MANAGED_APPS_SNAPSHOT_JSON = stringPreferencesKey("managed_apps_snapshot_json")
        val MANAGED_APPS_VERSION = longPreferencesKey("managed_apps_version")
        val MANAGED_APPS_LAST_APPLIED_AT_EPOCH_MILLIS = longPreferencesKey("managed_apps_last_applied_at_epoch_millis")
        val MANAGED_FILES_SNAPSHOT_JSON = stringPreferencesKey("managed_files_snapshot_json")
        val MANAGED_FILES_VERSION = longPreferencesKey("managed_files_version")
        val MANAGED_FILES_LAST_APPLIED_AT_EPOCH_MILLIS = longPreferencesKey("managed_files_last_applied_at_epoch_millis")
        val CERTIFICATES_SNAPSHOT_JSON = stringPreferencesKey("certificates_snapshot_json")
        val CERTIFICATES_VERSION = longPreferencesKey("certificates_version")
        val CERTIFICATES_LAST_APPLIED_AT_EPOCH_MILLIS = longPreferencesKey("certificates_last_applied_at_epoch_millis")
        val KIOSK_EXIT_SUPPRESSED_UNTIL_POLICY_VERSION = longPreferencesKey("kiosk_exit_suppressed_until_policy_version")
    }

    companion object {
        fun from(context: Context): AgentStateStore {
            return AgentStateStore(context.agentStateDataStore)
        }
    }
}

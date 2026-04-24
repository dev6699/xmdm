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
            prefs[Keys.BOOTSTRAP_SERVER_PROJECT] = state.serverProject
            prefs[Keys.BOOTSTRAP_ENROLLMENT_TOKEN] = state.enrollmentToken
            if (state.deviceId == null) {
                prefs.remove(Keys.BOOTSTRAP_DEVICE_ID)
            } else {
                prefs[Keys.BOOTSTRAP_DEVICE_ID] = state.deviceId
            }
            if (state.deviceIdUse == null) {
                prefs.remove(Keys.BOOTSTRAP_DEVICE_ID_USE)
            } else {
                prefs[Keys.BOOTSTRAP_DEVICE_ID_USE] = state.deviceIdUse
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
            prefs[Keys.DEVICE_ID_USE] = state.deviceIdUse
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

    suspend fun clearEnrollmentState() {
        dataStore.edit { prefs ->
            prefs.remove(Keys.DEVICE_ID)
            prefs.remove(Keys.DEVICE_ID_USE)
            prefs.remove(Keys.DEVICE_SECRET)
            prefs.remove(Keys.POLICY_SNAPSHOT_JSON)
            prefs.remove(Keys.POLICY_VERSION)
            prefs.remove(Keys.POLICY_LAST_SYNC_AT_EPOCH_MILLIS)
        }
    }

    private fun readState(prefs: Preferences): AgentState {
        val bootstrap = bootstrapFromPrefs(prefs)
        val identity = identityFromPrefs(prefs)
        val policyCache = policyCacheFromPrefs(prefs)
        return AgentState(
            bootstrap = bootstrap,
            identity = identity,
            policyCache = policyCache,
        )
    }

    private fun bootstrapFromPrefs(prefs: Preferences): BootstrapState? {
        val serverUrl = prefs[Keys.BOOTSTRAP_SERVER_URL] ?: return null
        val secondaryServerUrl = prefs[Keys.BOOTSTRAP_SECONDARY_SERVER_URL]
        val serverProject = prefs[Keys.BOOTSTRAP_SERVER_PROJECT] ?: return null
        val enrollmentToken = prefs[Keys.BOOTSTRAP_ENROLLMENT_TOKEN] ?: return null
        val deviceId = prefs[Keys.BOOTSTRAP_DEVICE_ID]
        val deviceIdUse = prefs[Keys.BOOTSTRAP_DEVICE_ID_USE]
        val bootstrapExtrasJson = prefs[Keys.BOOTSTRAP_EXTRAS_JSON] ?: "{}"
        val rawJson = prefs[Keys.BOOTSTRAP_RAW_JSON]
        return BootstrapState(
            serverUrl = serverUrl,
            secondaryServerUrl = secondaryServerUrl,
            serverProject = serverProject,
            enrollmentToken = enrollmentToken,
            deviceId = deviceId,
            deviceIdUse = deviceIdUse,
            bootstrapExtrasJson = bootstrapExtrasJson,
            rawJson = rawJson,
        )
    }

    private fun identityFromPrefs(prefs: Preferences): DeviceIdentityState? {
        val deviceId = prefs[Keys.DEVICE_ID] ?: return null
        val deviceIdUse = prefs[Keys.DEVICE_ID_USE] ?: return null
        val deviceSecret = prefs[Keys.DEVICE_SECRET] ?: return null
        return DeviceIdentityState(
            deviceId = deviceId,
            deviceIdUse = deviceIdUse,
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

    private object Keys {
        val BOOTSTRAP_SERVER_URL = stringPreferencesKey("bootstrap_server_url")
        val BOOTSTRAP_SECONDARY_SERVER_URL = stringPreferencesKey("bootstrap_secondary_server_url")
        val BOOTSTRAP_SERVER_PROJECT = stringPreferencesKey("bootstrap_server_project")
        val BOOTSTRAP_ENROLLMENT_TOKEN = stringPreferencesKey("bootstrap_enrollment_token")
        val BOOTSTRAP_DEVICE_ID = stringPreferencesKey("bootstrap_device_id")
        val BOOTSTRAP_DEVICE_ID_USE = stringPreferencesKey("bootstrap_device_id_use")
        val BOOTSTRAP_EXTRAS_JSON = stringPreferencesKey("bootstrap_extras_json")
        val BOOTSTRAP_RAW_JSON = stringPreferencesKey("bootstrap_raw_json")

        val DEVICE_ID = stringPreferencesKey("device_id")
        val DEVICE_ID_USE = stringPreferencesKey("device_id_use")
        val DEVICE_SECRET = stringPreferencesKey("device_secret")

        val POLICY_SNAPSHOT_JSON = stringPreferencesKey("policy_snapshot_json")
        val POLICY_VERSION = longPreferencesKey("policy_version")
        val POLICY_LAST_SYNC_AT_EPOCH_MILLIS = longPreferencesKey("policy_last_sync_at_epoch_millis")
    }

    companion object {
        fun from(context: Context): AgentStateStore {
            return AgentStateStore(context.agentStateDataStore)
        }
    }
}

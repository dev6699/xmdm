package com.xmdm.launcher.commands

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import kotlinx.coroutines.flow.first

private val Context.deviceCommandResultDataStore by preferencesDataStore(name = "device_command_results")

interface DeviceCommandResultJournal {
    suspend fun lookup(commandId: String): DeviceCommandExecutionResult?
    suspend fun record(commandId: String, result: DeviceCommandExecutionResult)
}

object NoOpDeviceCommandResultJournal : DeviceCommandResultJournal {
    override suspend fun lookup(commandId: String): DeviceCommandExecutionResult? = null

    override suspend fun record(commandId: String, result: DeviceCommandExecutionResult) {
        Unit
    }
}

class DeviceCommandResultStore(
    private val dataStore: DataStore<Preferences>,
    private val gson: Gson = Gson(),
    private val maxEntries: Int = 256,
) : DeviceCommandResultJournal {
    private val entriesType = object : TypeToken<List<StoredCommandResult>>() {}.type

    override suspend fun lookup(commandId: String): DeviceCommandExecutionResult? {
        if (commandId.isBlank()) {
            return null
        }
        return readEntries(dataStore.data.first()).lastOrNull { it.commandId == commandId }?.result
    }

    override suspend fun record(commandId: String, result: DeviceCommandExecutionResult) {
        if (commandId.isBlank()) {
            return
        }
        dataStore.edit { prefs ->
            val next = readEntries(prefs).filterNot { it.commandId == commandId }.toMutableList()
            next += StoredCommandResult(commandId = commandId, result = result, recordedAtEpochMillis = System.currentTimeMillis())
            writeEntries(prefs, next)
        }
    }

    private fun readEntries(prefs: Preferences): List<StoredCommandResult> {
        val raw = prefs[Keys.RECENT_COMMAND_RESULTS_JSON] ?: return emptyList()
        return runCatching {
            @Suppress("UNCHECKED_CAST")
            gson.fromJson<List<StoredCommandResult>>(raw, entriesType)
        }.getOrDefault(emptyList())
    }

    private fun writeEntries(prefs: androidx.datastore.preferences.core.MutablePreferences, entries: List<StoredCommandResult>) {
        val trimmed = entries.takeLast(maxEntries)
        if (trimmed.isEmpty()) {
            prefs.remove(Keys.RECENT_COMMAND_RESULTS_JSON)
            return
        }
        prefs[Keys.RECENT_COMMAND_RESULTS_JSON] = gson.toJson(trimmed, entriesType)
    }

    private object Keys {
        val RECENT_COMMAND_RESULTS_JSON = stringPreferencesKey("recent_command_results_json")
    }

    companion object {
        fun from(context: Context): DeviceCommandResultStore {
            return DeviceCommandResultStore(context.deviceCommandResultDataStore)
        }
    }
}

private data class StoredCommandResult(
    val commandId: String,
    val result: DeviceCommandExecutionResult,
    val recordedAtEpochMillis: Long,
)

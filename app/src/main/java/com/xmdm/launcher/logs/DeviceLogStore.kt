package com.xmdm.launcher.logs

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken

private val Context.deviceLogDataStore by preferencesDataStore(name = "device_logs")

interface DeviceLogQueue {
    suspend fun append(entry: DeviceLogEntry)
    suspend fun drain(): List<DeviceLogEntry>
    suspend fun prepend(entries: List<DeviceLogEntry>)
}

class DeviceLogStore(
    private val dataStore: DataStore<Preferences>,
    private val gson: Gson = Gson(),
    private val maxEntries: Int = 500,
) : DeviceLogQueue {
    private val entriesType = object : TypeToken<List<DeviceLogEntry>>() {}.type

    override suspend fun append(entry: DeviceLogEntry) {
        dataStore.edit { prefs ->
            val next = readEntries(prefs).toMutableList()
            next += entry
            writeEntries(prefs, next)
        }
    }

    override suspend fun drain(): List<DeviceLogEntry> {
        var drained = emptyList<DeviceLogEntry>()
        dataStore.edit { prefs ->
            drained = readEntries(prefs)
            prefs.remove(Keys.PENDING_ENTRIES_JSON)
        }
        return drained
    }

    override suspend fun prepend(entries: List<DeviceLogEntry>) {
        if (entries.isEmpty()) {
            return
        }
        dataStore.edit { prefs ->
            val current = readEntries(prefs)
            writeEntries(prefs, entries + current)
        }
    }

    private fun readEntries(prefs: Preferences): List<DeviceLogEntry> {
        val raw = prefs[Keys.PENDING_ENTRIES_JSON] ?: return emptyList()
        return runCatching {
            @Suppress("UNCHECKED_CAST")
            gson.fromJson<List<DeviceLogEntry>>(raw, entriesType)
        }.getOrDefault(emptyList())
    }

    private fun writeEntries(prefs: androidx.datastore.preferences.core.MutablePreferences, entries: List<DeviceLogEntry>) {
        val trimmed = entries.takeLast(maxEntries)
        if (trimmed.isEmpty()) {
            prefs.remove(Keys.PENDING_ENTRIES_JSON)
            return
        }
        prefs[Keys.PENDING_ENTRIES_JSON] = gson.toJson(trimmed, entriesType)
    }

    private object Keys {
        val PENDING_ENTRIES_JSON = stringPreferencesKey("pending_entries_json")
    }

    companion object {
        fun from(context: Context): DeviceLogStore {
            return DeviceLogStore(context.deviceLogDataStore)
        }
    }
}


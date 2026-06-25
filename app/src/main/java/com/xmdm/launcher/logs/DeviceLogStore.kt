package com.xmdm.launcher.logs

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import kotlinx.coroutines.flow.first
import java.time.Instant
import java.util.UUID

private val Context.deviceLogDataStore by preferencesDataStore(name = "device_logs")

interface DeviceLogQueue {
    suspend fun append(entry: DeviceLogEntry)
    suspend fun count(): Int
    suspend fun peekBatch(limit: Int): List<DeviceLogEntry>
    suspend fun remove(ids: Set<String>)
    suspend fun drain(): List<DeviceLogEntry>
    suspend fun prepend(entries: List<DeviceLogEntry>)
}

class DeviceLogStore(
    private val dataStore: DataStore<Preferences>,
    private val gson: Gson = Gson(),
    private val maxEntries: Int = 500,
    private val maxSerializedChars: Int = 512 * 1024,
) : DeviceLogQueue {
    private val entriesType = object : TypeToken<List<DeviceLogEntry>>() {}.type

    override suspend fun append(entry: DeviceLogEntry) {
        dataStore.edit { prefs ->
            val next = readEntries(prefs).toMutableList()
            next += entry
            writeEntries(prefs, next)
        }
    }

    override suspend fun count(): Int {
        return dataStore.data.first()[Keys.PENDING_ENTRIES_JSON]
            ?.let { raw -> parseEntries(raw).size }
            ?: 0
    }

    override suspend fun peekBatch(limit: Int): List<DeviceLogEntry> {
        if (limit <= 0) {
            return emptyList()
        }
        return dataStore.data.first()[Keys.PENDING_ENTRIES_JSON]
            ?.let { raw -> parseEntries(raw).take(limit) }
            ?: emptyList()
    }

    override suspend fun remove(ids: Set<String>) {
        if (ids.isEmpty()) {
            return
        }
        dataStore.edit { prefs ->
            val remaining = readEntries(prefs).filterNot { it.id in ids }
            writeEntries(prefs, remaining)
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
        return parseEntries(raw)
    }

    private fun parseEntries(raw: String): List<DeviceLogEntry> {
        return runCatching {
            @Suppress("UNCHECKED_CAST")
            gson.fromJson<List<DeviceLogEntry>>(raw, entriesType)
        }.getOrDefault(emptyList())
            .map { entry ->
                val id = runCatching { entry.id }.getOrNull()
                if (id.isNullOrBlank()) {
                    entry.copy(id = UUID.randomUUID().toString())
                } else {
                    entry
                }
            }
    }

    private fun writeEntries(
        prefs: androidx.datastore.preferences.core.MutablePreferences,
        entries: List<DeviceLogEntry>,
    ) {
        val trimmed = trimForLimits(entries)
        if (trimmed.isEmpty()) {
            prefs.remove(Keys.PENDING_ENTRIES_JSON)
            return
        }
        prefs[Keys.PENDING_ENTRIES_JSON] = gson.toJson(trimmed, entriesType)
    }

    private fun trimForLimits(entries: List<DeviceLogEntry>): List<DeviceLogEntry> {
        if (maxEntries <= 0 || maxSerializedChars <= 0) {
            return emptyList()
        }

        val working = entries.toMutableList()
        var droppedCount = 0

        while (working.size > maxEntries) {
            if (dropLowestPriorityEntry(working)) {
                droppedCount += 1
            } else {
                break
            }
        }

        while (gson.toJson(working, entriesType).length > maxSerializedChars && working.isNotEmpty()) {
            if (dropLowestPriorityEntry(working)) {
                droppedCount += 1
            } else {
                break
            }
        }

        if (droppedCount > 0) {
            working += DeviceLogEntry(
                observedAt = Instant.now().toString(),
                source = "logs",
                level = "warn",
                message = "device logs dropped",
                payload = mapOf(
                    "droppedCount" to droppedCount,
                    "queueEntryLimit" to maxEntries,
                    "queueSerializedCharLimit" to maxSerializedChars,
                ),
            )
            while (working.size > maxEntries) {
                dropLowestPriorityEntry(working, keepLatestDropSummary = true)
            }
            while (gson.toJson(working, entriesType).length > maxSerializedChars && working.isNotEmpty()) {
                dropLowestPriorityEntry(working, keepLatestDropSummary = true)
            }
        }

        return working
    }

    private fun dropLowestPriorityEntry(
        entries: MutableList<DeviceLogEntry>,
        keepLatestDropSummary: Boolean = false,
    ): Boolean {
        val candidates = entries.withIndex()
            .filterNot { keepLatestDropSummary && it.value.source == "logs" && it.value.message == "device logs dropped" }
        if (candidates.isEmpty()) {
            return false
        }
        val indexToDrop = candidates.minWithOrNull(
            compareBy<IndexedValue<DeviceLogEntry>> { priorityScore(it.value) }
                .thenBy { it.index },
        )?.index ?: return false
        entries.removeAt(indexToDrop)
        return true
    }

    private fun priorityScore(entry: DeviceLogEntry): Int {
        val securityOrCritical = when (entry.message) {
            "kiosk exit requested locally",
            "kiosk exit requested by command",
            "kiosk exit passcode accepted",
            "kiosk app launch failed",
            "managed app install failed",
            "managed files apply failed",
            "certificates apply failed",
            "enrollment failed",
            "config invalid",
            "config changed",
            "launcher upgraded",
            "managed apps apply resumed",
            "managed files cleared",
            "certificates cleared",
            "kiosk app launch skipped",
            "device logs dropped" -> true
            else -> false
        }
        if (securityOrCritical) {
            return 100
        }
        return when (entry.level.lowercase()) {
            "error" -> 80
            "warn" -> 60
            "info" -> 40
            "debug" -> 20
            else -> 30
        }
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

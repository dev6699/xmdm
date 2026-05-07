package com.xmdm.launcher.kiosk

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.state.AgentState
import java.security.MessageDigest
import java.util.Base64

class KioskExitGestureTracker(
    private val requiredTapCount: Int = DEFAULT_REQUIRED_TAP_COUNT,
    private val resetTimeoutMs: Long = DEFAULT_RESET_TIMEOUT_MS,
) {
    private var tapCount: Int = 0
    private var lastTapAtMs: Long = 0L

    fun registerTap(nowMs: Long): Boolean {
        if (tapCount > 0 && nowMs - lastTapAtMs > resetTimeoutMs) {
            reset()
        }
        lastTapAtMs = nowMs
        tapCount += 1
        if (tapCount < requiredTapCount) {
            return false
        }
        reset()
        return true
    }

    fun reset() {
        tapCount = 0
        lastTapAtMs = 0L
    }

    companion object {
        const val DEFAULT_REQUIRED_TAP_COUNT = 5
        const val DEFAULT_RESET_TIMEOUT_MS = 1_500L
    }
}

internal fun kioskPolicyActive(state: AgentState): Boolean {
    val policyCache = state.policyCache ?: return false
    return kioskModeEnabled(policyCache.snapshotJson) &&
        kioskExitPasscodeConfigured(policyCache.snapshotJson) &&
        isPolicyContentReady(state, policyCache.version) &&
        !isKioskExitSuppressed(state, policyCache.version)
}

internal fun isPolicyContentReady(state: AgentState, version: Long): Boolean {
    val managedAppsReady = state.managedApps?.version?.let { it == version } ?: true
    val managedFilesReady = state.managedFiles?.version?.let { it == version } ?: true
    return managedAppsReady && managedFilesReady
}

internal fun isKioskExitSuppressed(state: AgentState, version: Long): Boolean {
    val suppressedUntil = state.kioskControl?.exitSuppressedUntilPolicyVersion ?: return false
    return version <= suppressedUntil
}

internal fun kioskModeEnabled(snapshotJson: String): Boolean {
    val root = rootJsonObject(snapshotJson) ?: return false
    val policy = root.getAsJsonObject("policy") ?: return false
    return booleanValue(
        policy,
        "kioskMode",
        "kiosk_mode",
    )
}

internal fun kioskExitPasscodeHash(snapshotJson: String): String? {
    val root = rootJsonObject(snapshotJson) ?: return null
    val policy = root.getAsJsonObject("policy") ?: return null
    val restrictions = policy.getAsJsonObject("restrictions") ?: return null
    return stringValue(
        restrictions,
        "kioskExitPasscodeHash",
        "kiosk_exit_passcode_hash",
    )
}

internal fun kioskExitPasscodeConfigured(snapshotJson: String): Boolean {
    return !kioskExitPasscodeHash(snapshotJson).isNullOrBlank()
}

internal fun kioskExitPasscodeMatches(snapshotJson: String, candidate: String): Boolean {
    val expectedHash = kioskExitPasscodeHash(snapshotJson) ?: return false
    return MessageDigest.isEqual(
        expectedHash.toByteArray(Charsets.UTF_8),
        hashKioskExitPasscode(candidate).toByteArray(Charsets.UTF_8),
    )
}

internal fun hashKioskExitPasscode(passcode: String): String {
    val digest = MessageDigest.getInstance("SHA-256").digest(passcode.trim().toByteArray(Charsets.UTF_8))
    return Base64.getUrlEncoder().withoutPadding().encodeToString(digest)
}

private fun rootJsonObject(snapshotJson: String): JsonObject? {
    return runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
}

internal fun booleanValue(source: JsonObject, vararg names: String): Boolean {
    for (name in names) {
        val value = source.get(name) ?: continue
        if (value.isJsonNull) continue
        when {
            value.isJsonPrimitive && value.asJsonPrimitive.isBoolean -> return value.asBoolean
            value.isJsonPrimitive && value.asJsonPrimitive.isString -> {
                val raw = value.asString.trim()
                if (raw.equals("true", ignoreCase = true)) return true
                if (raw.equals("false", ignoreCase = true)) return false
            }
        }
    }
    return false
}

internal fun stringValue(source: JsonObject, vararg names: String): String? {
    for (name in names) {
        val value = source.get(name) ?: continue
        if (value.isJsonNull) continue
        if (value.isJsonPrimitive && value.asJsonPrimitive.isString) {
            val raw = value.asString.trim()
            if (raw.isNotEmpty()) {
                return raw
            }
        }
    }
    return null
}

package com.xmdm.launcher.kiosk

import android.app.Activity
import android.app.ActivityManager
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.os.Build
import android.util.Log
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.AdminReceiver
import com.xmdm.launcher.state.AgentState

interface KioskModeHost {
    val packageName: String
    fun isDeviceOwnerApp(): Boolean
    fun isInLockTaskMode(): Boolean
    fun setLockTaskPackages(packages: Array<String>)
    fun startLockTask()
    fun stopLockTask()
}

fun interface KioskModeLogger {
    fun warn(message: String)
}

private object AndroidKioskModeLogger : KioskModeLogger {
    override fun warn(message: String) {
        Log.w("XmdmLauncher", message)
    }
}

class AndroidKioskModeHost(
    private val activity: Activity,
) : KioskModeHost {
    private val devicePolicyManager: DevicePolicyManager? by lazy {
        activity.getSystemService(DevicePolicyManager::class.java)
    }
    private val adminComponent by lazy {
        ComponentName(activity, AdminReceiver::class.java)
    }

    override val packageName: String
        get() = activity.packageName

    override fun isDeviceOwnerApp(): Boolean {
        return devicePolicyManager?.isDeviceOwnerApp(activity.packageName) == true
    }

    override fun isInLockTaskMode(): Boolean {
        val activityManager = activity.getSystemService(ActivityManager::class.java) ?: return false
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            activityManager.lockTaskModeState == ActivityManager.LOCK_TASK_MODE_LOCKED
        } else {
            activityManager.isInLockTaskMode()
        }
    }

    override fun setLockTaskPackages(packages: Array<String>) {
        val dpm = devicePolicyManager ?: return
        if (!isDeviceOwnerApp()) {
            return
        }
        dpm.setLockTaskPackages(adminComponent, packages)
    }

    override fun startLockTask() {
        activity.startLockTask()
    }

    override fun stopLockTask() {
        activity.stopLockTask()
    }
}

class KioskModeController(
    private val host: KioskModeHost,
    private val logger: KioskModeLogger = AndroidKioskModeLogger,
) {
    private var appliedKioskMode: Boolean? = null

    fun apply(state: AgentState) {
        val policyCache = state.policyCache
        val desiredKioskMode = policyCache?.let { cache ->
            isKioskModeEnabled(cache.snapshotJson) && isPolicyContentReady(state, cache.version)
        } == true

        if (appliedKioskMode == desiredKioskMode) {
            return
        }

        if (desiredKioskMode) {
            if (!host.isDeviceOwnerApp()) {
                logger.warn("kiosk mode requested but device owner is unavailable")
                return
            }
            host.setLockTaskPackages(arrayOf(host.packageName))
            if (!host.isInLockTaskMode()) {
                host.startLockTask()
            }
            logger.warn("kiosk mode enabled")
            appliedKioskMode = true
            return
        }

        host.setLockTaskPackages(emptyArray())
        if (host.isInLockTaskMode()) {
            host.stopLockTask()
        }
        logger.warn("kiosk mode disabled")
        appliedKioskMode = false
    }

    private fun isPolicyContentReady(state: AgentState, version: Long): Boolean {
        val managedAppsReady = state.managedApps?.version?.let { it == version } ?: true
        val managedFilesReady = state.managedFiles?.version?.let { it == version } ?: true
        return managedAppsReady && managedFilesReady
    }

    private fun isKioskModeEnabled(snapshotJson: String): Boolean {
        val root = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
            ?: return false
        val policy = root.getAsJsonObject("policy") ?: return false
        return booleanValue(
            policy,
            "kioskMode",
            "kiosk_mode",
        )
    }

    private fun booleanValue(source: JsonObject, vararg names: String): Boolean {
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
}

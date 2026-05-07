package com.xmdm.launcher.kiosk

import android.app.Activity
import android.app.ActivityManager
import android.app.ActivityOptions
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Intent
import android.os.BatteryManager
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.view.WindowManager
import android.provider.Settings
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.AdminReceiver
import com.xmdm.launcher.state.AgentState
import kotlinx.coroutines.delay

interface KioskModeHost {
    val packageName: String
    fun isDeviceOwnerApp(): Boolean
    fun isInLockTaskMode(): Boolean
    fun setKeepScreenOn(keepScreenOn: Boolean)
    fun setStayAwakeWhilePluggedIn(stayAwakeWhilePluggedIn: Boolean)
    fun setLockTaskPackages(packages: Array<String>)
    fun startLockTask()
    fun stopLockTask()
    fun finishHostActivity()
    fun canLaunchPackage(packageName: String): Boolean
    fun launchPackage(packageName: String, lockTaskEnabled: Boolean): Boolean
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

    override fun setKeepScreenOn(keepScreenOn: Boolean) {
        if (keepScreenOn) {
            activity.window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        } else {
            activity.window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        }
    }

    override fun setStayAwakeWhilePluggedIn(stayAwakeWhilePluggedIn: Boolean) {
        val dpm = devicePolicyManager ?: return
        if (!isDeviceOwnerApp()) {
            return
        }
        val value = if (stayAwakeWhilePluggedIn) {
            BatteryManager.BATTERY_PLUGGED_AC or
                BatteryManager.BATTERY_PLUGGED_USB or
                BatteryManager.BATTERY_PLUGGED_WIRELESS
        } else {
            0
        }
        runCatching {
            dpm.setGlobalSetting(
                adminComponent,
                Settings.Global.STAY_ON_WHILE_PLUGGED_IN,
                value.toString(),
            )
        }.onFailure {
            Log.w("XmdmLauncher", "failed to set stay-on-while-plugged-in=$stayAwakeWhilePluggedIn", it)
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

    override fun finishHostActivity() {
        if (!activity.moveTaskToBack(true)) {
            activity.finishAndRemoveTask()
        }
    }

    override fun canLaunchPackage(packageName: String): Boolean {
        return kioskLaunchIntent(packageName) != null
    }

    override fun launchPackage(packageName: String, lockTaskEnabled: Boolean): Boolean {
        val launchIntent = kioskLaunchIntent(packageName) ?: return false
        launchIntent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED)
        return runCatching {
            val options = if (lockTaskEnabled) createLaunchOptions() else null
            if (options == null) {
                activity.startActivity(launchIntent)
            } else {
                activity.startActivity(launchIntent, options)
            }
        }.fold(
            onSuccess = { true },
            onFailure = {
                Log.w("XmdmLauncher", "failed to launch package=$packageName", it)
                false
            },
        )
    }

    private fun kioskLaunchIntent(packageName: String): Intent? {
        return activity.packageManager.getLaunchIntentForPackage(packageName)
    }

    private fun createLaunchOptions(): Bundle? {
        val options = ActivityOptions.makeBasic()
        runCatching {
            val method = options.javaClass.methods.firstOrNull { candidate ->
                candidate.name == "setLockTaskEnabled" && candidate.parameterTypes.size == 1 &&
                    candidate.parameterTypes[0] == Boolean::class.javaPrimitiveType
            } ?: return@runCatching
            method.invoke(options, true)
        }
        return options.toBundle()
    }
}

class KioskModeController(
    private val host: KioskModeHost,
    private val logger: KioskModeLogger = AndroidKioskModeLogger,
) {
    private var appliedKioskMode: Boolean? = null
    private var appliedKioskPackage: String? = null

    fun apply(state: AgentState, forceLaunch: Boolean = false) {
        val policyCache = state.policyCache
        val desiredKioskMode = policyCache?.let { cache ->
            kioskModeEnabled(cache.snapshotJson) &&
                kioskExitPasscodeConfigured(cache.snapshotJson) &&
                isPolicyContentReady(state, cache.version) &&
                !isKioskExitSuppressed(state, cache.version)
        } == true
        val desiredKeepScreenOn = policyCache?.takeIf { desiredKioskMode }?.let { cache ->
            kioskKeepScreenOn(cache.snapshotJson)
        } == true
        host.setKeepScreenOn(desiredKeepScreenOn)
        val desiredStayAwakeWhilePluggedIn = policyCache?.takeIf { desiredKioskMode }?.let { cache ->
            kioskStayAwakeWhilePluggedIn(cache.snapshotJson)
        } == true
        host.setStayAwakeWhilePluggedIn(desiredStayAwakeWhilePluggedIn)
        val desiredKioskPackage = policyCache?.takeIf { desiredKioskMode }?.let { cache ->
            kioskPackageName(cache.snapshotJson)?.takeIf { it.isNotBlank() } ?: host.packageName
        }

        if (desiredKioskMode &&
            appliedKioskMode == true &&
            appliedKioskPackage == desiredKioskPackage &&
            !forceLaunch
        ) {
            if (desiredKioskPackage != host.packageName || host.isInLockTaskMode()) {
                return
            }
        }

        if (!desiredKioskMode && appliedKioskMode == false && !host.isInLockTaskMode()) {
            return
        }

        if (desiredKioskMode) {
            if (!host.isDeviceOwnerApp()) {
                logger.warn("kiosk mode requested but device owner is unavailable")
                return
            }
            val launched = enforceKiosk(desiredKioskPackage ?: host.packageName, forceLaunch)
            if (launched || host.isInLockTaskMode()) {
                logger.warn("kiosk mode enabled")
                appliedKioskMode = true
                appliedKioskPackage = desiredKioskPackage ?: host.packageName
            }
            return
        }

        if (!desiredKioskMode && host.isInLockTaskMode()) {
            host.stopLockTask()
        }
        host.setLockTaskPackages(emptyArray())
        logger.warn("kiosk mode disabled")
        appliedKioskMode = false
        appliedKioskPackage = null
    }

    suspend fun launchConfiguredKioskApp(state: AgentState): Boolean {
        val policyCache = state.policyCache ?: return false
        if (!kioskModeEnabled(policyCache.snapshotJson) ||
            !kioskExitPasscodeConfigured(policyCache.snapshotJson) ||
            !isPolicyContentReady(state, policyCache.version) ||
            isKioskExitSuppressed(state, policyCache.version)
        ) {
            return false
        }
        if (!host.isDeviceOwnerApp()) {
            logger.warn("kiosk mode requested but device owner is unavailable")
            return false
        }
        val kioskPackage = kioskPackageName(policyCache.snapshotJson)?.takeIf { it.isNotBlank() } ?: host.packageName
        if (kioskPackage != host.packageName) {
            var remainingWaitMs = 60_000L
            while (remainingWaitMs > 0 && !host.canLaunchPackage(kioskPackage)) {
                logger.warn("waiting for kiosk app package $kioskPackage to become launchable")
                delay(500)
                remainingWaitMs -= 500
            }
            if (!host.canLaunchPackage(kioskPackage)) {
                logger.warn("kiosk app package $kioskPackage is still not launchable")
                return false
            }
        }
        return enforceKiosk(kioskPackage, forceLaunch = true)
    }

    private fun enforceKiosk(kioskPackage: String, forceLaunch: Boolean): Boolean {
        val lockTaskPackages = linkedSetOf(host.packageName, kioskPackage).toTypedArray()
        host.setLockTaskPackages(lockTaskPackages)
        if (kioskPackage == host.packageName) {
            if (!host.isInLockTaskMode()) {
                host.startLockTask()
            }
            return true
        }
        logger.warn("launching kiosk app package $kioskPackage force=$forceLaunch")
        val launched = host.launchPackage(kioskPackage, lockTaskEnabled = true)
        if (!launched) {
            logger.warn("kiosk app package $kioskPackage could not be launched")
            if (!host.isInLockTaskMode()) {
                host.startLockTask()
            }
            appliedKioskMode = true
            appliedKioskPackage = host.packageName
            return false
        }
        appliedKioskMode = true
        appliedKioskPackage = kioskPackage
        if (kioskPackage != host.packageName) {
            if (host.isInLockTaskMode()) {
                host.stopLockTask()
            }
            host.finishHostActivity()
        }
        return true
    }

    private fun kioskPackageName(snapshotJson: String): String? {
        val root = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
            ?: return null
        val policy = root.getAsJsonObject("policy") ?: return null
        return stringValue(policy, "kioskAppPackage", "kiosk_app_package")
    }

    private fun kioskKeepScreenOn(snapshotJson: String): Boolean {
        val root = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
            ?: return false
        val policy = root.getAsJsonObject("policy") ?: return false
        val restrictions = policy.getAsJsonObject("restrictions") ?: return false
        return booleanValue(
            restrictions,
            "kioskKeepScreenOn",
            "kiosk_keep_screen_on",
        )
    }

    private fun kioskStayAwakeWhilePluggedIn(snapshotJson: String): Boolean {
        val root = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
            ?: return false
        val policy = root.getAsJsonObject("policy") ?: return false
        val restrictions = policy.getAsJsonObject("restrictions") ?: return false
        return booleanValue(
            restrictions,
            "kioskStayAwakeWhilePluggedIn",
            "kiosk_stay_awake_while_plugged_in",
        )
    }

}

internal fun kioskKeepScreenOn(snapshotJson: String): Boolean {
    return kioskRestrictionBoolean(
        snapshotJson,
        "kioskKeepScreenOn",
        "kiosk_keep_screen_on",
    )
}

internal fun kioskUnlockOnBoot(snapshotJson: String): Boolean {
    return kioskRestrictionBoolean(
        snapshotJson,
        "kioskUnlockOnBoot",
        "kiosk_unlock_on_boot",
    )
}

private fun kioskRestrictionBoolean(snapshotJson: String, vararg names: String): Boolean {
    val root = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
        ?: return false
    val policy = root.getAsJsonObject("policy") ?: return false
    val restrictions = policy.getAsJsonObject("restrictions") ?: return false
    for (name in names) {
        val value = restrictions.get(name) ?: continue
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

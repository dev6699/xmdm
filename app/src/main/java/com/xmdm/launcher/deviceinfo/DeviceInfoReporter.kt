package com.xmdm.launcher.deviceinfo

import com.google.gson.GsonBuilder
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.BatteryManager
import android.os.Build
import com.google.gson.JsonParser
import com.xmdm.launcher.BuildConfig
import com.xmdm.launcher.AdminReceiver
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.time.Clock

class DeviceInfoReporter(
    private val context: Context,
    private val gateway: HttpDeviceInfoGateway = HttpDeviceInfoGateway(),
    private val clock: Clock = Clock.systemUTC(),
) {
    private val mutex = Mutex()
    private var lastReportFingerprint: String? = null

    suspend fun uploadIfNeeded(
        bootstrap: BootstrapState,
        identity: DeviceIdentityState,
        state: AgentState,
    ) {
        val payload = collectPayload(bootstrap, identity, state)
        val fingerprint = deviceInfoFingerprint(payload)
        mutex.withLock {
            if (lastReportFingerprint == fingerprint) {
                return
            }
        }
        gateway.upload(
            serverUrl = bootstrap.serverUrl,
            deviceId = identity.deviceId,
            deviceSecret = identity.deviceSecret,
            request = DeviceInfoUploadRequest(
                observedAt = clock.instant().toString(),
                payload = payload,
            ),
        )
        mutex.withLock {
            lastReportFingerprint = fingerprint
        }
    }

    private fun collectPayload(
        bootstrap: BootstrapState,
        identity: DeviceIdentityState,
        state: AgentState,
    ): Map<String, Any?> {
        val payload = linkedMapOf<String, Any?>()
        payload["deviceId"] = identity.deviceId
        payload["deviceIdUse"] = identity.deviceIdUse
        payload["serverUrl"] = bootstrap.serverUrl
        payload["serverProject"] = bootstrap.serverProject
        payload["appPackage"] = context.packageName
        payload["appVersionName"] = BuildConfig.VERSION_NAME
        payload["appVersionCode"] = BuildConfig.VERSION_CODE
        payload["model"] = Build.MODEL
        payload["manufacturer"] = Build.MANUFACTURER
        payload["brand"] = Build.BRAND
        payload["device"] = Build.DEVICE
        payload["product"] = Build.PRODUCT
        payload["hardware"] = Build.HARDWARE
        payload["board"] = Build.BOARD
        payload["abi"] = Build.SUPPORTED_ABIS.joinToString(",")
        payload["androidVersion"] = Build.VERSION.RELEASE
        payload["sdkInt"] = Build.VERSION.SDK_INT
        payload["securityPatch"] = Build.VERSION.SECURITY_PATCH
        payload["serial"] = deviceSerial()
        payload["battery"] = batterySnapshot()
        payload["deviceOwner"] = isDeviceOwnerApp()
        state.policyCache?.let { cache ->
            payload["configRevision"] = cache.version
            payload["policyVersion"] = policyVersion(cache.snapshotJson)
            payload["policyKioskMode"] = policyKioskMode(cache.snapshotJson)
        }
        state.certificates?.let { payload["certificatesVersion"] = it.version }
        installedCaCertsCount()?.let { payload["installedCaCertsCount"] = it }
        state.managedApps?.let { payload["managedAppsVersion"] = it.version }
        state.managedFiles?.let { payload["managedFilesVersion"] = it.version }
        return payload
    }

    private fun installedCaCertsCount(): Int? {
        return runCatching {
            val dpm = context.getSystemService(DevicePolicyManager::class.java) ?: return null
            if (!dpm.isDeviceOwnerApp(context.packageName)) {
                return null
            }
            val adminComponent = ComponentName(context, AdminReceiver::class.java)
            dpm.getInstalledCaCerts(adminComponent)?.size
        }.getOrNull()
    }

    private fun policyVersion(snapshotJson: String): Long? {
        return runCatching {
            val root = JsonParser.parseString(snapshotJson).asJsonObject
            root.getAsJsonObject("policy")?.get("version")?.takeIf { !it.isJsonNull }?.asLong
        }.getOrNull()
    }

    private fun policyKioskMode(snapshotJson: String): Boolean? {
        return runCatching {
            val root = JsonParser.parseString(snapshotJson).asJsonObject
            root.getAsJsonObject("policy")?.get("kioskMode")?.takeIf { !it.isJsonNull }?.asBoolean
        }.getOrNull()
    }

    private fun isDeviceOwnerApp(): Boolean {
        val dpm = context.getSystemService(DevicePolicyManager::class.java) ?: return false
        return dpm.isDeviceOwnerApp(context.packageName)
    }

    private fun deviceSerial(): String? {
        return runCatching {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                Build.getSerial()
            } else {
                @Suppress("DEPRECATION")
                Build.SERIAL
            }
        }.getOrNull()
    }

    private fun batterySnapshot(): Map<String, Any?> {
        val filter = IntentFilter(Intent.ACTION_BATTERY_CHANGED)
        @Suppress("DEPRECATION")
        val batteryStatus = context.registerReceiver(null, filter) ?: return emptyMap()
        val status = batteryStatus.getIntExtra(BatteryManager.EXTRA_STATUS, -1)
        val charging = status == BatteryManager.BATTERY_STATUS_CHARGING || status == BatteryManager.BATTERY_STATUS_FULL
        val plugged = when (batteryStatus.getIntExtra(BatteryManager.EXTRA_PLUGGED, -1)) {
            BatteryManager.BATTERY_PLUGGED_USB -> "usb"
            BatteryManager.BATTERY_PLUGGED_AC -> "ac"
            BatteryManager.BATTERY_PLUGGED_WIRELESS -> "wireless"
            else -> ""
        }
        val level = batteryStatus.getIntExtra(BatteryManager.EXTRA_LEVEL, -1)
        val scale = batteryStatus.getIntExtra(BatteryManager.EXTRA_SCALE, -1)
        val percent = if (level >= 0 && scale > 0) (level * 100 / scale) else null
        return linkedMapOf<String, Any?>(
            "level" to percent,
            "charging" to charging,
            "plugged" to plugged,
        )
    }
}

internal fun deviceInfoFingerprint(payload: Map<String, Any?>): String {
    return GsonBuilder().serializeNulls().create().toJson(payload)
}

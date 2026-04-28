package com.xmdm.launcher.packages

import android.app.Activity
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.util.Log
import com.google.gson.JsonParser
import com.xmdm.launcher.AdminReceiver
import com.xmdm.launcher.state.AgentState

interface PackageRulesHost {
    val packageName: String
    fun isDeviceOwnerApp(): Boolean
    fun isPackageSuspended(packageName: String): Boolean
    fun setPackagesSuspended(packages: Array<String>, suspended: Boolean)
}

fun interface PackageRulesLogger {
    fun warn(message: String)
}

private object AndroidPackageRulesLogger : PackageRulesLogger {
    override fun warn(message: String) {
        Log.w("XmdmLauncher", message)
    }
}

class AndroidPackageRulesHost(
    private val activity: Activity,
) : PackageRulesHost {
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

    override fun isPackageSuspended(packageName: String): Boolean {
        val dpm = devicePolicyManager ?: return false
        if (!isDeviceOwnerApp()) {
            return false
        }
        return dpm.isPackageSuspended(adminComponent, packageName)
    }

    override fun setPackagesSuspended(packages: Array<String>, suspended: Boolean) {
        val dpm = devicePolicyManager ?: return
        if (!isDeviceOwnerApp()) {
            return
        }
        if (packages.isEmpty()) {
            return
        }
        dpm.setPackagesSuspended(adminComponent, packages, suspended)
    }
}

class PackageRulesController(
    private val host: PackageRulesHost,
    private val logger: PackageRulesLogger = AndroidPackageRulesLogger,
) {
    private var appliedSuspendedPackages: Set<String>? = null

    fun apply(state: AgentState) {
        val desiredSuspendedPackages = state.policyCache?.let { cache ->
            parseDesiredSuspendedPackages(cache.snapshotJson)
        } ?: emptySet()

        val currentlySuspended = desiredSuspendedPackages.filterTo(linkedSetOf()) { host.isPackageSuspended(it) }
        if (desiredSuspendedPackages == appliedSuspendedPackages && currentlySuspended == desiredSuspendedPackages) {
            return
        }
        if (!host.isDeviceOwnerApp()) {
            logger.warn("package rules requested but device owner is unavailable")
            return
        }

        val filteredDesired = desiredSuspendedPackages - host.packageName
        val previous = appliedSuspendedPackages ?: emptySet()
        val toUnsuspend = previous - filteredDesired
        val toSuspend = filteredDesired - previous

        if (toUnsuspend.isNotEmpty()) {
            host.setPackagesSuspended(toUnsuspend.sorted().toTypedArray(), false)
        }
        if (toSuspend.isNotEmpty()) {
            host.setPackagesSuspended(toSuspend.sorted().toTypedArray(), true)
        }

        appliedSuspendedPackages = filteredDesired.filterTo(linkedSetOf()) { host.isPackageSuspended(it) }
        logger.warn("package rules applied suspended=${filteredDesired.size}")
    }

    private fun parseDesiredSuspendedPackages(snapshotJson: String): Set<String> {
        val root = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
            ?: return emptySet()
        val policy = root.getAsJsonObject("policy") ?: return emptySet()
        val restrictions = policy.getAsJsonObject("restrictions") ?: return emptySet()

        val allowPackages = readStringSet(restrictions, "allowPackages")
        val blockPackages = readStringSet(restrictions, "blockPackages")
        val suspendPackages = readStringSet(restrictions, "suspendPackages")

        return (blockPackages + suspendPackages) - allowPackages
    }

    private fun readStringSet(source: com.google.gson.JsonObject, name: String): Set<String> {
        val value = source.get(name) ?: return emptySet()
        if (!value.isJsonArray) {
            return emptySet()
        }
        return value.asJsonArray.mapNotNullTo(linkedSetOf()) { element ->
            if (!element.isJsonPrimitive) return@mapNotNullTo null
            val raw = element.asString.trim()
            raw.takeIf { it.isNotEmpty() }
        }
    }
}

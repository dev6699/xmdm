package com.xmdm.launcher.commands

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import com.google.gson.JsonObject
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.security.MessageDigest
import java.util.Locale

interface CompanionAppLaunchHost {
    fun isPackageInstalled(packageName: String): Boolean
    fun packageSignatureDigests(packageName: String): Set<String>?
    fun canLaunchPackage(packageName: String): Boolean
    fun canLaunchActivity(packageName: String, activityName: String): Boolean
    fun launchPackage(packageName: String, bootstrapPayload: String? = null): Boolean
    fun launchActivity(packageName: String, activityName: String, bootstrapPayload: String? = null): Boolean
}

fun interface CompanionAppLaunchLogger {
    fun warn(message: String)
}

private object AndroidCompanionAppLaunchLogger : CompanionAppLaunchLogger {
    override fun warn(message: String) {
        Log.w("XmdmLauncher", message)
    }
}

class AndroidCompanionAppLaunchHost(
    private val context: Context,
) : CompanionAppLaunchHost {
    private val packageManager: PackageManager
        get() = context.packageManager

    override fun isPackageInstalled(packageName: String): Boolean {
        return runCatching { packageInfo(packageName) }.isSuccess
    }

    override fun packageSignatureDigests(packageName: String): Set<String>? {
        val packageInfo = runCatching { packageInfo(packageName) }.getOrNull() ?: return null
        val signatures = packageSignatures(packageInfo)
        if (signatures.isEmpty()) {
            return null
        }
        return signatures.mapTo(linkedSetOf()) { sha256Hex(it) }
    }

    override fun canLaunchPackage(packageName: String): Boolean {
        return packageManager.getLaunchIntentForPackage(packageName) != null
    }

    override fun canLaunchActivity(packageName: String, activityName: String): Boolean {
        val intent = explicitLaunchIntent(packageName, activityName)
        return intent.resolveActivity(packageManager) != null
    }

    override fun launchPackage(packageName: String, bootstrapPayload: String?): Boolean {
        val launchIntent = packageManager.getLaunchIntentForPackage(packageName) ?: return false
        launchIntent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED)
        launchIntent.withBootstrapPayload(bootstrapPayload)
        return runCatching {
            context.startActivity(launchIntent)
        }.fold(
            onSuccess = { true },
            onFailure = {
                Log.w("XmdmLauncher", "failed to launch companion package=$packageName", it)
                false
            },
        )
    }

    override fun launchActivity(packageName: String, activityName: String, bootstrapPayload: String?): Boolean {
        val launchIntent = explicitLaunchIntent(packageName, activityName)
        launchIntent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED)
        launchIntent.withBootstrapPayload(bootstrapPayload)
        return runCatching {
            context.startActivity(launchIntent)
        }.fold(
            onSuccess = { true },
            onFailure = {
                Log.w("XmdmLauncher", "failed to launch companion activity=$packageName/$activityName", it)
                false
            },
        )
    }

    private fun explicitLaunchIntent(packageName: String, activityName: String): Intent {
        val componentName = ComponentName(packageName, normalizeActivityName(packageName, activityName))
        return Intent(Intent.ACTION_MAIN).apply {
            addCategory(Intent.CATEGORY_LAUNCHER)
            component = componentName
        }
    }

    private fun Intent.withBootstrapPayload(bootstrapPayload: String?): Intent {
        val payload = bootstrapPayload?.trim().orEmpty()
        if (payload.isNotEmpty()) {
            putExtra(Intent.EXTRA_TEXT, payload)
        }
        return this
    }

    private fun packageInfo(packageName: String): PackageInfo {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            packageManager.getPackageInfo(
                packageName,
                PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES.toLong()),
            )
        } else {
            @Suppress("DEPRECATION")
            packageManager.getPackageInfo(packageName, PackageManager.GET_SIGNING_CERTIFICATES)
        }
    }

    private fun packageSignatures(packageInfo: PackageInfo): List<ByteArray> {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            val signingInfo = packageInfo.signingInfo ?: return emptyList()
            val signatures = if (signingInfo.hasMultipleSigners()) {
                signingInfo.apkContentsSigners
            } else {
                signingInfo.signingCertificateHistory
            }
            signatures?.mapNotNull { it.toByteArray().takeIf { bytes -> bytes.isNotEmpty() } }.orEmpty()
        } else {
            @Suppress("DEPRECATION")
            packageInfo.signatures?.mapNotNull { it.toByteArray().takeIf { bytes -> bytes.isNotEmpty() } }.orEmpty()
        }
    }

    private fun sha256Hex(bytes: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(bytes)
        return buildString(digest.size * 2) {
            for (byte in digest) {
                append(((byte.toInt() ushr 4) and 0x0f).toString(16))
                append((byte.toInt() and 0x0f).toString(16))
            }
        }
    }

    private fun normalizeActivityName(packageName: String, activityName: String): String {
        val trimmed = activityName.trim()
        if (trimmed.isEmpty()) {
            return trimmed
        }
        if (trimmed.startsWith(".")) {
            return packageName + trimmed
        }
        if (trimmed.contains('/')) {
            val component = ComponentName.unflattenFromString(trimmed) ?: return trimmed
            return component.className
        }
        return trimmed
    }
}

class CompanionAppLaunchCoordinator(
    private val host: CompanionAppLaunchHost,
    private val snapshotVerifier: ConfigSnapshotVerifier = ConfigSnapshotVerifier(),
    private val logger: CompanionAppLaunchLogger = AndroidCompanionAppLaunchLogger,
    private val launchDispatcher: CoroutineDispatcher = Dispatchers.Main,
) {
    suspend fun execute(state: AgentState, command: DeviceCommandRecord): DeviceCommandExecutionResult {
        val identity = state.identity ?: return failure(command, "device identity unavailable")
        val managedApps = state.managedApps ?: return failure(command, "managed apps snapshot unavailable")
        val request = parseRequest(command)
            ?: return failure(command, "companion app launch payload is invalid")
        val snapshot = runCatching {
            snapshotVerifier.verify(managedApps.snapshotJson, identity.deviceSecret)
        }.getOrElse {
            return failure(command, "managed apps snapshot signature invalid", it)
        }
        val declaredPackages = declaredPackageNames(snapshot)
        if (request.packageName !in declaredPackages) {
            return failure(command, "companion package declaration missing")
        }
        if (!host.isPackageInstalled(request.packageName)) {
            return failure(command, "companion app is not installed")
        }
        val expectedSignature = normalizeSignatureDigest(request.signatureSha256)
            ?: return failure(command, "companion app signature is required")
        val actualSignatures = host.packageSignatureDigests(request.packageName)
            ?: return failure(command, "companion app signature unavailable")
        if (!signaturesMatch(expectedSignature, actualSignatures)) {
            return failure(command, "companion app signature mismatch")
        }

        val launched = withContext(launchDispatcher) {
            if (request.activityName.isNullOrBlank()) {
                if (!host.canLaunchPackage(request.packageName)) {
                    return@withContext false
                }
                host.launchPackage(request.packageName, request.bootstrapPayload)
            } else {
                if (!host.canLaunchActivity(request.packageName, request.activityName)) {
                    return@withContext false
                }
                host.launchActivity(request.packageName, request.activityName, request.bootstrapPayload)
            }
        }
        if (!launched) {
            return failure(command, "companion app could not be launched")
        }
        logger.warn("companion app launch requested package=${request.packageName} activity=${request.activityName ?: ""}")
        return DeviceCommandExecutionResult(
            status = "acked",
            message = "companion app launch requested",
            details = command.commandDetails(
                "command" to command.type,
                "packageName" to request.packageName,
                "activityName" to request.activityName,
                "signatureSha256" to request.signatureSha256,
                "launchTarget" to if (request.activityName.isNullOrBlank()) "package" else "activity",
            ),
        )
    }

    private fun parseRequest(command: DeviceCommandRecord): CompanionAppLaunchRequest? {
        val payload = command.payload ?: return null
        val packageName = payload.string("packageName") ?: return null
        val activityName = payload.string("activityName")
        val signatureSha256 = payload.string("signatureSha256")
        val bootstrapPayload = payload.string("bootstrapPayload")
        return CompanionAppLaunchRequest(
            packageName = packageName,
            activityName = activityName,
            signatureSha256 = signatureSha256,
            bootstrapPayload = bootstrapPayload,
        )
    }

    private fun declaredPackageNames(snapshot: JsonObject): Set<String> {
        val apps = snapshot.getAsJsonArray("apps") ?: return emptySet()
        return apps.mapNotNullTo(linkedSetOf()) { element ->
            if (!element.isJsonObject) {
                return@mapNotNullTo null
            }
            element.asJsonObject.string("packageName")
        }
    }

    private fun failure(
        command: DeviceCommandRecord,
        message: String,
        error: Throwable? = null,
    ): DeviceCommandExecutionResult {
        if (error != null) {
            logger.warn("$message: ${error.message ?: error.javaClass.simpleName}")
        } else {
            logger.warn(message)
        }
        return DeviceCommandExecutionResult(
            status = "failed",
            message = message,
            details = command.commandDetails(
                "command" to command.type,
            ),
        )
    }

    private fun normalizeSignatureDigest(raw: String?): String? {
        val value = raw?.trim()?.lowercase(Locale.US)?.replace(":", "")?.replace("-", "") ?: return null
        return value.takeIf { it.isNotEmpty() }
    }

    private fun signaturesMatch(expected: String, actual: Set<String>): Boolean {
        if (actual.isEmpty()) {
            return false
        }
        val normalizedExpected = normalizeSignatureDigest(expected) ?: return false
        return actual.any { normalizeSignatureDigest(it) == normalizedExpected }
    }
}

private fun JsonObject.string(name: String): String? {
    val value = get(name) ?: return null
    if (value.isJsonNull) {
        return null
    }
    return value.asString.takeIf { it.isNotBlank() }
}

data class CompanionAppLaunchRequest(
    val packageName: String,
    val activityName: String?,
    val signatureSha256: String?,
    val bootstrapPayload: String?,
)

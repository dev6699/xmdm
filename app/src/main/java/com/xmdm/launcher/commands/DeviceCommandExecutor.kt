package com.xmdm.launcher.commands

import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.os.Build
import com.xmdm.launcher.AdminReceiver

interface DeviceCommandActionExecutor {
    suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult
}

class DeviceCommandExecutor(
    private val rebootAction: DeviceRebooter,
) : DeviceCommandActionExecutor {
    override suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult {
        return when (command.type.lowercase()) {
            "ping" -> DeviceCommandExecutionResult(
                status = "acked",
                message = "pong",
                details = command.details("command" to "ping"),
            )
            "reboot" -> {
                rebootAction.reboot()
                DeviceCommandExecutionResult(
                    status = "acked",
                    message = "device reboot requested",
                    details = command.details("command" to "reboot"),
                )
            }
            else -> DeviceCommandExecutionResult(
                status = "failed",
                message = "unsupported command type: ${command.type}",
                details = command.details("command" to command.type),
            )
        }
    }
}

interface DeviceRebooter {
    fun reboot()
}

class AndroidDeviceRebooter(
    private val context: Context,
) : DeviceRebooter {
    override fun reboot() {
        val devicePolicyManager = context.getSystemService(DevicePolicyManager::class.java)
            ?: error("device policy manager unavailable")
        if (!devicePolicyManager.isDeviceOwnerApp(context.packageName)) {
            error("device owner is required for reboot commands")
        }
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.N) {
            error("reboot commands require Android N or later")
        }
        val admin = ComponentName(context, AdminReceiver::class.java)
        runCatching {
            val method = devicePolicyManager.javaClass.methods.firstOrNull { it.name == "reboot" }
                ?: error("device policy manager reboot API unavailable")
            when (method.parameterTypes.size) {
                0 -> method.invoke(devicePolicyManager)
                1 -> method.invoke(devicePolicyManager, admin)
                else -> error("unexpected reboot method signature")
            }
        }.getOrElse { throw IllegalStateException("failed to request reboot", it) }
    }
}


private fun DeviceCommandRecord.details(vararg values: Pair<String, Any?>): Map<String, Any> {
    val result = linkedMapOf<String, Any>()
    result["commandId"] = id
    result["commandType"] = type
    for ((key, value) in values) {
        if (value != null) {
            result[key] = value
        }
    }
    payload?.let { payloadJson ->
        result["payload"] = payloadJson
    }
    expiresAt?.let { result["expiresAt"] = it }
    return result
}

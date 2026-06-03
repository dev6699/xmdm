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
    private val configSyncAction: (suspend () -> DeviceCommandExecutionResult)? = null,
    private val kioskExitAction: (suspend () -> DeviceCommandExecutionResult)? = null,
    private val companionAppLaunchAction: (suspend (DeviceCommandRecord) -> DeviceCommandExecutionResult)? = null,
) : DeviceCommandActionExecutor {
    override suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult {
        return when (command.type.lowercase()) {
            "ping" -> DeviceCommandExecutionResult(
                status = "acked",
                message = "pong",
                details = command.commandDetails("command" to "ping"),
            )
            "reboot" -> {
                rebootAction.reboot()
                DeviceCommandExecutionResult(
                    status = "acked",
                    message = "device reboot requested",
                    details = command.commandDetails("command" to "reboot"),
                )
            }
            "sync_config" -> configSyncAction?.invoke() ?: DeviceCommandExecutionResult(
                status = "failed",
                message = "config sync unavailable",
                details = command.commandDetails("command" to command.type),
            )
            "exit_kiosk" -> kioskExitAction?.invoke() ?: DeviceCommandExecutionResult(
                status = "failed",
                message = "kiosk exit unavailable",
                details = command.commandDetails("command" to command.type),
            )
            "launch_companion_app" -> companionAppLaunchAction?.invoke(command) ?: DeviceCommandExecutionResult(
                status = "failed",
                message = "companion app launch unavailable",
                details = command.commandDetails("command" to command.type),
            )
            else -> DeviceCommandExecutionResult(
                status = "failed",
                message = "unsupported command type: ${command.type}",
                details = command.commandDetails("command" to command.type),
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

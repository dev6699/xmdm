package com.xmdm.launcher.commands

import kotlinx.coroutines.CancellationException

class DeviceCommandCoordinator(
    private val gateway: DeviceCommandGateway,
    private val executor: DeviceCommandActionExecutor,
) {
    suspend fun handleIncomingCommand(
        serverUrl: String,
        deviceId: String,
        deviceSecret: String,
        command: DeviceCommandRecord,
    ): DeviceCommandRecord {
        val result = try {
            executor.execute(command)
        } catch (failure: Throwable) {
            if (failure is CancellationException) {
                throw failure
            }
            DeviceCommandExecutionResult(
                status = "failed",
                message = failure.message ?: failure.javaClass.simpleName,
                details = mapOf(
                    "commandId" to command.id,
                    "commandType" to command.type,
                    "error" to (failure.message ?: failure.javaClass.simpleName),
                ),
            )
        }
        return gateway.ack(
            serverUrl = serverUrl,
            deviceId = deviceId,
            deviceSecret = deviceSecret,
            commandId = command.id,
            request = DeviceCommandAckRequest(
                status = result.status,
                message = result.message,
                details = result.details,
            ),
        )
    }

    suspend fun pollAndExecute(serverUrl: String, deviceId: String, deviceSecret: String): List<DeviceCommandRecord> {
        val commands = gateway.poll(serverUrl, deviceId, deviceSecret)
        val handled = mutableListOf<DeviceCommandRecord>()
        for (command in commands) {
            handleIncomingCommand(serverUrl, deviceId, deviceSecret, command)
            handled += command
        }
        return handled
    }
}

package com.xmdm.launcher.commands

import kotlinx.coroutines.CancellationException

class DeviceCommandCoordinator(
    private val gateway: DeviceCommandGateway,
    private val executor: DeviceCommandActionExecutor,
    private val history: DeviceCommandResultJournal = NoOpDeviceCommandResultJournal,
) {
    suspend fun handleIncomingCommand(
        serverUrl: String,
        deviceId: String,
        deviceSecret: String,
        command: DeviceCommandRecord,
    ): DeviceCommandRecord {
        val result = history.lookup(command.id) ?: try {
            val executed = executor.execute(command)
            rememberCommandResult(command.id, executed)
            executed
        } catch (failure: Throwable) {
            if (failure is CancellationException) {
                throw failure
            }
            val executed = DeviceCommandExecutionResult(
                status = "failed",
                message = failure.message ?: failure.javaClass.simpleName,
                details = mapOf(
                    "commandId" to command.id,
                    "commandType" to command.type,
                    "error" to (failure.message ?: failure.javaClass.simpleName),
                ),
            )
            rememberCommandResult(command.id, executed)
            executed
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

    private suspend fun rememberCommandResult(commandId: String, result: DeviceCommandExecutionResult) {
        try {
            history.record(commandId, result)
        } catch (failure: Throwable) {
            if (failure is CancellationException) {
                throw failure
            }
        }
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

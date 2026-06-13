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
        transportSource: String,
        command: DeviceCommandRecord,
        onCommandExecuted: suspend (DeviceCommandRecord, DeviceCommandExecutionResult) -> Unit = { _, _ -> },
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
        onCommandExecuted(command, result)
        val ackDetails = buildAckDetails(command, result, transportSource)
        return gateway.ack(
            serverUrl = serverUrl,
            deviceId = deviceId,
            deviceSecret = deviceSecret,
            commandId = command.id,
            request = DeviceCommandAckRequest(
                status = result.status,
                message = result.message,
                details = ackDetails,
            ),
        )
    }

    private fun buildAckDetails(
        command: DeviceCommandRecord,
        result: DeviceCommandExecutionResult,
        transportSource: String,
    ): Map<String, Any>? {
        val details = linkedMapOf<String, Any>()
        result.details?.forEach { (key, value) ->
            details[key] = value
        }
        details.putAll(command.commandDetails("transportSource" to transportSource))
        return if (details.isEmpty()) null else details
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

    suspend fun pollAndExecute(
        serverUrl: String,
        deviceId: String,
        deviceSecret: String,
        transportSource: String = "polling",
        onCommandReceived: suspend (DeviceCommandRecord) -> Unit = {},
        onCommandExecuted: suspend (DeviceCommandRecord, DeviceCommandExecutionResult) -> Unit = { _, _ -> },
        onCommandAcked: suspend (DeviceCommandRecord, DeviceCommandRecord) -> Unit = { _, _ -> },
    ): List<DeviceCommandRecord> {
        val commands = gateway.poll(serverUrl, deviceId, deviceSecret)
        val handled = mutableListOf<DeviceCommandRecord>()
        for (command in commands) {
            onCommandReceived(command)
            val acked = handleIncomingCommand(
                serverUrl = serverUrl,
                deviceId = deviceId,
                deviceSecret = deviceSecret,
                transportSource = transportSource,
                command = command,
                onCommandExecuted = onCommandExecuted,
            )
            onCommandAcked(command, acked)
            handled += command
        }
        return handled
    }
}

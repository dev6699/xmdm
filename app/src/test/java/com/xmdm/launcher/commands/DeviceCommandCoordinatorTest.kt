package com.xmdm.launcher.commands

import com.google.gson.JsonParser
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DeviceCommandCoordinatorTest {
    @Test
    fun executesLightweightPingCommand() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-ping",
            type = "ping",
            status = "queued",
            payload = JsonParser.parseString("""{"message":"hello"}""").asJsonObject,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = DeviceCommandExecutor(
                rebootAction = object : DeviceRebooter {
                    override fun reboot() = error("not expected")
                },
            ),
        )

        val handled = coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )

        assertEquals(listOf("cmd-ping"), handled.map { it.id })
        assertEquals(1, gateway.acks.size)
        assertEquals("acked", gateway.acks[0].request.status)
        assertEquals("pong", gateway.acks[0].request.message)
    }

    @Test
    fun pollsExecutesAndAcksCommands() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-1",
            type = "reboot",
            status = "queued",
            payload = JsonParser.parseString("""{"force":true}""").asJsonObject,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = object : DeviceCommandActionExecutor {
                override suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult {
                    assertEquals("cmd-1", command.id)
                    return DeviceCommandExecutionResult(
                        status = "acked",
                        message = "done",
                        details = mapOf("result" to "ok"),
                    )
                }
            },
        )

        val handled = coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )

        assertEquals(listOf("cmd-1"), handled.map { it.id })
        assertEquals(1, gateway.acks.size)
        assertEquals("cmd-1", gateway.acks[0].commandId)
        assertEquals("acked", gateway.acks[0].request.status)
        assertEquals("done", gateway.acks[0].request.message)
        assertEquals("ok", gateway.acks[0].request.details?.get("result"))
        assertEquals("polling", gateway.acks[0].request.details?.get("transportSource"))
    }

    @Test
    fun enrichesAckDetailsWithTransportSource() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-mqtt",
            type = "ping",
            status = "queued",
            payload = null,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = object : DeviceCommandActionExecutor {
                override suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult {
                    return DeviceCommandExecutionResult(
                        status = "acked",
                        message = "pong",
                        details = mapOf("result" to "ok"),
                    )
                }
            },
        )

        coordinator.handleIncomingCommand(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
            transportSource = "mqtt",
            command = command,
        )

        assertEquals(1, gateway.acks.size)
        assertEquals("mqtt", gateway.acks[0].request.details?.get("transportSource"))
    }

    @Test
    fun reportsCommandExecutionBeforeAck() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-exec",
            type = "ping",
            status = "queued",
            payload = null,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = object : DeviceCommandActionExecutor {
                override suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult {
                    return DeviceCommandExecutionResult(
                        status = "acked",
                        message = "pong",
                    )
                }
            },
        )
        val observed = mutableListOf<String>()

        coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
            onCommandReceived = { observed += "received:${it.id}" },
            onCommandExecuted = { executed, result -> observed += "executed:${executed.id}:${result.status}" },
            onCommandAcked = { acked, _ -> observed += "acked:${acked.id}" },
        )

        assertEquals(
            listOf("received:cmd-exec", "executed:cmd-exec:acked", "acked:cmd-exec"),
            observed,
        )
    }

    @Test
    fun executesAndAcksConfigSyncCommands() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-sync",
            type = "sync_config",
            status = "queued",
            payload = null,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        var syncRequested = false
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = DeviceCommandExecutor(
                rebootAction = object : DeviceRebooter {
                    override fun reboot() = error("not expected")
                },
                configSyncAction = {
                    syncRequested = true
                    DeviceCommandExecutionResult(
                        status = "acked",
                        message = "config refreshed",
                        details = mapOf("configRevision" to 9L),
                    )
                },
            ),
        )

        val handled = coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )

        assertTrue(syncRequested)
        assertEquals(listOf("cmd-sync"), handled.map { it.id })
        assertEquals(1, gateway.acks.size)
        assertEquals("acked", gateway.acks[0].request.status)
        assertEquals("config refreshed", gateway.acks[0].request.message)
        assertEquals(9L, gateway.acks[0].request.details?.get("configRevision"))
    }

    @Test
    fun executesAndAcksExitKioskCommands() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-exit",
            type = "exit_kiosk",
            status = "queued",
            payload = null,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        var exitRequested = false
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = DeviceCommandExecutor(
                rebootAction = object : DeviceRebooter {
                    override fun reboot() = error("not expected")
                },
                kioskExitAction = {
                    exitRequested = true
                    DeviceCommandExecutionResult(
                        status = "acked",
                        message = "kiosk exit requested",
                        details = mapOf("policyVersion" to 9L),
                    )
                },
            ),
        )

        val handled = coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )

        assertTrue(exitRequested)
        assertEquals(listOf("cmd-exit"), handled.map { it.id })
        assertEquals(1, gateway.acks.size)
        assertEquals("acked", gateway.acks[0].request.status)
        assertEquals("kiosk exit requested", gateway.acks[0].request.message)
        assertEquals(9L, gateway.acks[0].request.details?.get("policyVersion"))
    }

    @Test
    fun executesAndAcksCompanionLaunchCommands() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-launch",
            type = "launch_companion_app",
            status = "queued",
            payload = JsonParser.parseString(
                """{"packageName":"com.example.companion","signatureSha256":"deadbeef"}""",
            ).asJsonObject,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        var launchRequested = false
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = DeviceCommandExecutor(
                rebootAction = object : DeviceRebooter {
                    override fun reboot() = error("not expected")
                },
                companionAppLaunchAction = { incoming ->
                    launchRequested = incoming.id == "cmd-launch"
                    DeviceCommandExecutionResult(
                        status = "acked",
                        message = "companion app launch requested",
                        details = mapOf("packageName" to "com.example.companion"),
                    )
                },
            ),
        )

        val handled = coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )

        assertTrue(launchRequested)
        assertEquals(listOf("cmd-launch"), handled.map { it.id })
        assertEquals(1, gateway.acks.size)
        assertEquals("acked", gateway.acks[0].request.status)
        assertEquals("companion app launch requested", gateway.acks[0].request.message)
        assertEquals("com.example.companion", gateway.acks[0].request.details?.get("packageName"))
    }

    @Test
    fun acksExecutionFailures() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-2",
            type = "unsupported",
            status = "queued",
            payload = null,
            expiresAt = null,
        )
        val gateway = RecordingGateway(commands = listOf(command))
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = object : DeviceCommandActionExecutor {
                override suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult {
                    error("boom")
                }
            },
        )

        val handled = coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )

        assertEquals(listOf("cmd-2"), handled.map { it.id })
        assertEquals(1, gateway.acks.size)
        assertEquals("failed", gateway.acks[0].request.status)
        assertTrue(gateway.acks[0].request.message!!.contains("boom"))
    }

    @Test
    fun reusesCachedResultsForDuplicateCommands() = runTest {
        val command = DeviceCommandRecord(
            id = "cmd-dup",
            type = "ping",
            status = "queued",
            payload = JsonParser.parseString("""{"message":"hello"}""").asJsonObject,
            expiresAt = null,
        )
        var executions = 0
        val history = RecordingHistory()
        val gateway = RecordingGateway(commands = listOf(command, command))
        val coordinator = DeviceCommandCoordinator(
            gateway = gateway,
            executor = object : DeviceCommandActionExecutor {
                override suspend fun execute(command: DeviceCommandRecord): DeviceCommandExecutionResult {
                    executions += 1
                    return DeviceCommandExecutionResult(
                        status = "acked",
                        message = "pong",
                        details = mapOf("sequence" to executions),
                    )
                }
            },
            history = history,
        )

        val handled = coordinator.pollAndExecute(
            serverUrl = "https://mdm.example",
            deviceId = "device-123",
            deviceSecret = "secret-abc",
        )

        assertEquals(listOf("cmd-dup", "cmd-dup"), handled.map { it.id })
        assertEquals(1, executions)
        assertEquals(2, gateway.acks.size)
        assertEquals(1, gateway.acks[0].request.details?.get("sequence"))
        assertEquals(1, gateway.acks[1].request.details?.get("sequence"))
    }

    private class RecordingGateway(
        private val commands: List<DeviceCommandRecord>,
    ) : DeviceCommandGateway {
        val polls = mutableListOf<Triple<String, String, String>>()
        val acks = mutableListOf<AckCall>()

        override suspend fun poll(serverUrl: String, deviceId: String, deviceSecret: String): List<DeviceCommandRecord> {
            polls += Triple(serverUrl, deviceId, deviceSecret)
            return commands
        }

        override suspend fun ack(
            serverUrl: String,
            deviceId: String,
            deviceSecret: String,
            commandId: String,
            request: DeviceCommandAckRequest,
        ): DeviceCommandRecord {
            acks += AckCall(serverUrl, deviceId, deviceSecret, commandId, request)
            return commands.first { it.id == commandId }
        }
    }

    private data class AckCall(
        val serverUrl: String,
        val deviceId: String,
        val deviceSecret: String,
        val commandId: String,
        val request: DeviceCommandAckRequest,
    )

    private class RecordingHistory : DeviceCommandResultJournal {
        private val results = linkedMapOf<String, DeviceCommandExecutionResult>()

        override suspend fun lookup(commandId: String): DeviceCommandExecutionResult? {
            return results[commandId]
        }

        override suspend fun record(commandId: String, result: DeviceCommandExecutionResult) {
            results[commandId] = result
        }
    }
}

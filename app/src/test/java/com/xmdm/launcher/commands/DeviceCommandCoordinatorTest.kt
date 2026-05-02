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
}

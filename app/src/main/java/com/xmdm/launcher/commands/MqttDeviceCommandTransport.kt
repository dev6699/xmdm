package com.xmdm.launcher.commands

import com.google.gson.JsonParser
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.BufferedInputStream
import java.io.BufferedOutputStream
import java.io.EOFException
import java.net.Socket
import java.net.URI
import java.net.SocketTimeoutException
import java.nio.charset.StandardCharsets
import java.time.Duration

data class MqttDeviceCommandConfig(
    val address: String,
    val clientId: String,
    val deviceId: String,
    val username: String,
    val password: String,
    val keepAliveSeconds: Int = 30,
    val connectTimeoutMillis: Int = 5_000,
)

class MqttDeviceCommandTransport(
    private val config: MqttDeviceCommandConfig,
) {
    suspend fun stream(
        onSubscribed: suspend () -> Unit = {},
        onCommand: suspend (DeviceCommandRecord) -> Unit,
    ) {
        withContext(Dispatchers.IO) {
            val endpoint = parseEndpoint(config.address)
            val socket = Socket()
            try {
                socket.connect(endpoint, config.connectTimeoutMillis)
                socket.soTimeout = 1_000
                val input = BufferedInputStream(socket.getInputStream())
                val output = BufferedOutputStream(socket.getOutputStream())
                writePacket(output, connectPacket(config))
                readConnack(input)
                writePacket(output, subscribePacket(deviceTopic(config.deviceId), 1))
                readSuback(input)
                onSubscribed()

                var lastActivity = System.currentTimeMillis()
                while (true) {
                    try {
                        val packet = readPacket(input)
                        lastActivity = System.currentTimeMillis()
                        when {
                            packet.isEmpty() -> continue
                            packet[0].toInt() and 0xF0 == 0x30 -> {
                                val command = parseCommand(packet)
                                onCommand(command)
                            }
                            packet[0].toInt() and 0xF0 == 0xD0 -> Unit
                        }
                    } catch (_: SocketTimeoutException) {
                        val idleMs = System.currentTimeMillis() - lastActivity
                        if (idleMs >= pingIntervalMillis(config.keepAliveSeconds)) {
                            writePacket(output, byteArrayOf(0xC0.toByte(), 0x00))
                            output.flush()
                            lastActivity = System.currentTimeMillis()
                        }
                    }
                }
            } finally {
                socket.close()
            }
        }
    }

    private fun parseCommand(packet: ByteArray): DeviceCommandRecord {
        val remainingOffset = 1 + decodeRemainingLength(packet, 1).second
        val topicLength = readUnsignedShort(packet, remainingOffset)
        val topicStart = remainingOffset + 2
        val topicEnd = topicStart + topicLength
        val payloadStart = topicEnd
        val payload = String(packet.copyOfRange(payloadStart, packet.size), StandardCharsets.UTF_8)
        return JsonParser.parseString(payload).asJsonObject.let { json ->
            DeviceCommandRecord(
                id = json.get("commandId")?.asString.orEmpty(),
                type = json.get("type")?.asString.orEmpty(),
                status = json.get("status")?.asString.orEmpty(),
                payload = json.getAsJsonObject("payload"),
                expiresAt = json.get("expiresAt")?.takeIf { !it.isJsonNull }?.asString,
            )
        }
    }

    private fun parseEndpoint(address: String): java.net.InetSocketAddress {
        val raw = address.trim()
        val uri = if (raw.contains("://")) URI(raw) else URI("mqtt://$raw")
        val host = uri.host ?: error("invalid mqtt address: $address")
        val port = if (uri.port > 0) uri.port else 1883
        return java.net.InetSocketAddress(host, port)
    }

    private fun deviceTopic(deviceId: String): String {
        return "devices/${deviceId.trim()}/commands"
    }

    private fun readConnack(input: BufferedInputStream) {
        val packet = readPacket(input)
        if (packet.size < 4 || packet[0].toInt() != 0x20 || packet[3].toInt() != 0x00) {
            error("mqtt connack rejected")
        }
    }

    private fun readSuback(input: BufferedInputStream) {
        while (true) {
            val packet = readPacket(input)
            when (packet.firstOrNull()?.toInt()?.and(0xF0)) {
                0x90 -> if (packet.size >= 5) return else error("mqtt suback rejected")
                0xE0 -> error("mqtt suback rejected")
                else -> continue
            }
        }
    }

    private fun connectPacket(cfg: MqttDeviceCommandConfig): ByteArray {
        val keepAlive = cfg.keepAliveSeconds.coerceAtLeast(5)
        val body = buildList<Byte> {
            addAll(encodeString("MQTT").toList())
            add(0x04)
            var flags = 0
            flags = flags or 0x80
            flags = flags or 0x40
            add(flags.toByte())
            add((keepAlive shr 8).toByte())
            add((keepAlive and 0xff).toByte())
            addAll(encodeString(cfg.clientId).toList())
            addAll(encodeString(cfg.username).toList())
            addAll(encodeString(cfg.password).toList())
        }.toByteArray()
        return packet(0x10, body)
    }

    private fun subscribePacket(topic: String, packetId: Int): ByteArray {
        val body = buildList<Byte> {
            add((packetId shr 8).toByte())
            add((packetId and 0xff).toByte())
            addAll(encodeString(topic).toList())
            add(0x00)
        }.toByteArray()
        return packet(0x82, body)
    }

    private fun packet(header: Int, body: ByteArray): ByteArray {
        val output = ByteArrayOutputStreamCompat()
        output.write(header)
        writeRemainingLength(output, body.size)
        output.write(body)
        return output.toByteArray()
    }

    private fun writePacket(output: BufferedOutputStream, packet: ByteArray) {
        output.write(packet)
        output.flush()
    }

    private fun readPacket(input: BufferedInputStream): ByteArray {
        val header = input.read()
        if (header < 0) {
            throw EOFException("mqtt stream closed")
        }
        val remaining = readRemainingLength(input)
        val body = ByteArray(remaining)
        var offset = 0
        while (offset < remaining) {
            val read = input.read(body, offset, remaining - offset)
            if (read < 0) {
                throw EOFException("mqtt stream closed")
            }
            offset += read
        }
        return byteArrayOf(header.toByte()) + encodeRemainingLength(remaining) + body
    }

    private fun readRemainingLength(input: BufferedInputStream): Int {
        var multiplier = 1
        var value = 0
        while (true) {
            val digit = input.read()
            if (digit < 0) throw EOFException("mqtt stream closed")
            value += (digit and 127) * multiplier
            if (digit and 128 == 0) {
                return value
            }
            multiplier *= 128
            if (multiplier > 128 * 128 * 128) {
                error("mqtt remaining length too large")
            }
        }
    }

    private fun decodeRemainingLength(packet: ByteArray, offset: Int): Pair<Int, Int> {
        var multiplier = 1
        var value = 0
        var consumed = 0
        while (true) {
            val digit = packet[offset + consumed].toInt() and 0xff
            consumed += 1
            value += (digit and 127) * multiplier
            if (digit and 128 == 0) {
                return value to consumed
            }
            multiplier *= 128
        }
    }

    private fun readUnsignedShort(data: ByteArray, offset: Int): Int {
        return ((data[offset].toInt() and 0xff) shl 8) or (data[offset + 1].toInt() and 0xff)
    }

    private fun encodeString(value: String): ByteArray {
        val bytes = value.toByteArray(StandardCharsets.UTF_8)
        return byteArrayOf((bytes.size shr 8).toByte(), (bytes.size and 0xff).toByte()) + bytes
    }

    private fun writeRemainingLength(output: ByteArrayOutputStreamCompat, value: Int) {
        var x = value
        do {
            var encoded = (x % 128).toByte()
            x /= 128
            if (x > 0) {
                encoded = (encoded.toInt() or 0x80).toByte()
            }
            output.write(encoded.toInt())
        } while (x > 0)
    }

    private fun encodeRemainingLength(value: Int): ByteArray {
        val output = ByteArrayOutputStreamCompat()
        writeRemainingLength(output, value)
        return output.toByteArray()
    }

    private fun pingIntervalMillis(keepAliveSeconds: Int): Long {
        return Duration.ofSeconds((keepAliveSeconds / 2).coerceAtLeast(1).toLong()).toMillis()
    }

    private class ByteArrayOutputStreamCompat {
        private var bytes = ByteArray(64)
        private var size = 0

        fun write(value: Int) {
            ensureCapacity(size + 1)
            bytes[size] = value.toByte()
            size += 1
        }

        fun write(data: ByteArray) {
            ensureCapacity(size + data.size)
            data.copyInto(bytes, size)
            size += data.size
        }

        fun toByteArray(): ByteArray = bytes.copyOf(size)

        private fun ensureCapacity(targetSize: Int) {
            if (targetSize <= bytes.size) {
                return
            }
            var next = bytes.size * 2
            while (next < targetSize) {
                next *= 2
            }
            bytes = bytes.copyOf(next)
        }
    }
}

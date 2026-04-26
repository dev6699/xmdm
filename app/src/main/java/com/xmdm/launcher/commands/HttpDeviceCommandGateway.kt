package com.xmdm.launcher.commands

import com.google.gson.Gson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import kotlin.text.Charsets

interface DeviceCommandGateway {
    suspend fun poll(serverUrl: String, deviceId: String, deviceSecret: String): List<DeviceCommandRecord>
    suspend fun ack(serverUrl: String, deviceId: String, deviceSecret: String, commandId: String, request: DeviceCommandAckRequest): DeviceCommandRecord
}

class HttpDeviceCommandGateway(
    private val gson: Gson = Gson(),
    private val connectTimeoutMs: Int = 10_000,
    private val readTimeoutMs: Int = 10_000,
) : DeviceCommandGateway {
    override suspend fun poll(serverUrl: String, deviceId: String, deviceSecret: String): List<DeviceCommandRecord> {
        return withContext(Dispatchers.IO) {
            val url = URL(serverUrl.trimEnd('/') + "/api/v1/devices/$deviceId/commands")
            val connection = (url.openConnection() as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = connectTimeoutMs
                readTimeout = readTimeoutMs
                setRequestProperty(DEVICE_SECRET_HEADER, deviceSecret)
                setRequestProperty("Accept", "application/json")
            }
            try {
                val statusCode = connection.responseCode
                val body = connection.responseBody()
                if (statusCode !in 200..299) {
                    error("command poll failed with HTTP $statusCode: $body")
                }
                val response = gson.fromJson(body, DeviceCommandPollResponse::class.java)
                response.commands.orEmpty()
            } finally {
                connection.disconnect()
            }
        }
    }

    override suspend fun ack(
        serverUrl: String,
        deviceId: String,
        deviceSecret: String,
        commandId: String,
        request: DeviceCommandAckRequest,
    ): DeviceCommandRecord {
        return withContext(Dispatchers.IO) {
            val url = URL(serverUrl.trimEnd('/') + "/api/v1/devices/$deviceId/commands/$commandId/ack")
            val connection = (url.openConnection() as HttpURLConnection).apply {
                requestMethod = "POST"
                connectTimeout = connectTimeoutMs
                readTimeout = readTimeoutMs
                doOutput = true
                setRequestProperty(DEVICE_SECRET_HEADER, deviceSecret)
                setRequestProperty("Content-Type", "application/json")
                setRequestProperty("Accept", "application/json")
            }
            try {
                connection.outputStream.use { output ->
                    output.write(gson.toJson(request).toByteArray(Charsets.UTF_8))
                }
                val statusCode = connection.responseCode
                val body = connection.responseBody()
                if (statusCode !in 200..299) {
                    error("command ack failed with HTTP $statusCode: $body")
                }
                gson.fromJson(body, DeviceCommandRecord::class.java)
            } finally {
                connection.disconnect()
            }
        }
    }

    private fun HttpURLConnection.responseBody(): String {
        val stream = if (responseCode in 200..299) inputStream else errorStream
        if (stream == null) {
            return ""
        }
        return stream.use { input ->
            BufferedReader(InputStreamReader(input, Charsets.UTF_8)).use { reader ->
                reader.readText()
            }
        }
    }

    private companion object {
        const val DEVICE_SECRET_HEADER = "X-XMDM-Device-Secret"
    }
}

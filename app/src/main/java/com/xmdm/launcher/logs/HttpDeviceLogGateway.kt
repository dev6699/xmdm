package com.xmdm.launcher.logs

import com.google.gson.Gson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import kotlin.text.Charsets

class HttpDeviceLogGateway(
    private val gson: Gson = Gson(),
    private val connectTimeoutMs: Int = 10_000,
    private val readTimeoutMs: Int = 10_000,
) : DeviceLogGateway {
    override suspend fun upload(
        serverUrl: String,
        deviceId: String,
        deviceSecret: String,
        request: DeviceLogUploadRequest,
    ) {
        withContext(Dispatchers.IO) {
            val url = URL(serverUrl.trimEnd('/') + "/api/v1/devices/$deviceId/logs")
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
                    val safeBody = body
                        .replace(deviceSecret, "[redacted]")
                        .take(MAX_ERROR_BODY_CHARS)
                    error("device log upload failed with HTTP $statusCode: $safeBody")
                }
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
        const val MAX_ERROR_BODY_CHARS = 512
    }
}


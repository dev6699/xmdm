package com.xmdm.launcher.deviceinfo

import com.google.gson.GsonBuilder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import kotlin.text.Charsets

data class DeviceInfoUploadRequest(
    val observedAt: String,
    val payload: Map<String, Any?>,
)

class HttpDeviceInfoGateway(
    private val connectTimeoutMs: Int = 10_000,
    private val readTimeoutMs: Int = 10_000,
) {
    private val gson = GsonBuilder().serializeNulls().create()

    suspend fun upload(
        serverUrl: String,
        deviceId: String,
        deviceSecret: String,
        request: DeviceInfoUploadRequest,
    ) {
        withContext(Dispatchers.IO) {
            val url = URL(serverUrl.trimEnd('/') + "/api/v1/devices/$deviceId/info")
            val connection = (url.openConnection() as HttpURLConnection).apply {
                requestMethod = "POST"
                doOutput = true
                connectTimeout = connectTimeoutMs
                readTimeout = readTimeoutMs
                setRequestProperty("X-XMDM-Device-Secret", deviceSecret)
                setRequestProperty("Content-Type", "application/json")
                setRequestProperty("Accept", "application/json")
            }
            try {
                connection.outputStream.use { output ->
                    val body = gson.toJson(
                        mapOf(
                            "observedAt" to request.observedAt,
                            "payload" to request.payload,
                        ),
                    )
                    output.write(body.toByteArray(Charsets.UTF_8))
                }
                val statusCode = connection.responseCode
                val body = connection.responseBody()
                if (statusCode !in 200..299) {
                    error("device info upload failed with HTTP $statusCode: $body")
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
}

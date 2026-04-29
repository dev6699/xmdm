package com.xmdm.launcher.sync

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import kotlin.text.Charsets

class HttpConfigSnapshotFetcher(
    private val connectTimeoutMs: Int = 10_000,
    private val readTimeoutMs: Int = 10_000,
) : ConfigSnapshotFetcher {
    override suspend fun fetch(request: ConfigFetchRequest): String {
        return withContext(Dispatchers.IO) {
            val url = URL(request.serverUrl.trimEnd('/') + "/api/v1/devices/${request.deviceId}/config")
            val connection = (url.openConnection() as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = connectTimeoutMs
                readTimeout = readTimeoutMs
                setRequestProperty("X-XMDM-Device-Secret", request.deviceSecret)
                setRequestProperty("Accept", "application/json")
            }
            try {
                val statusCode = connection.responseCode
                val body = connection.responseBody()
                if (statusCode !in 200..299) {
                    error("config sync failed with HTTP $statusCode: $body")
                }
                body
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

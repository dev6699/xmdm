package com.xmdm.launcher.enrollment

import com.google.gson.Gson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import kotlin.text.Charsets

class HttpEnrollmentGateway(
    private val gson: Gson = Gson(),
    private val connectTimeoutMs: Int = 10_000,
    private val readTimeoutMs: Int = 10_000,
) : EnrollmentGateway {
    override suspend fun enroll(serverUrl: String, request: EnrollmentRequest): EnrollmentResponse {
        return withContext(Dispatchers.IO) {
            val url = URL(serverUrl.trimEnd('/') + "/api/v1/enrollment")
            val connection = (url.openConnection() as HttpURLConnection).apply {
                requestMethod = "POST"
                connectTimeout = connectTimeoutMs
                readTimeout = readTimeoutMs
                doOutput = true
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
                    error("enrollment request failed with HTTP $statusCode: $body")
                }
                gson.fromJson(body, EnrollmentResponse::class.java)
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

package com.xmdm.launcher.apps

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.File
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URL

class HttpManagedAppDownloaderTest {
    @Test
    fun sendsDeviceSecretHeader() = runTest {
        val connection = FakeHttpURLConnection(URL("http://127.0.0.1/artifact"))
        val downloader = HttpManagedAppDownloader { connection }
        val destination = File.createTempFile("download-test-", ".apk")

        try {
            var lastDownloadedBytes = -1L
            var lastTotalBytes: Long? = null
            downloader.download(
                url = "http://127.0.0.1/artifact",
                deviceSecret = "secret-123",
                destination = destination,
            ) { downloadedBytes, totalBytes ->
                lastDownloadedBytes = downloadedBytes
                lastTotalBytes = totalBytes
            }

            assertEquals("apk-bytes", destination.readText())
            assertEquals("secret-123", connection.requestHeaders[HttpManagedAppDownloader.DEVICE_SECRET_HEADER])
            assertEquals(9L, lastDownloadedBytes)
            assertEquals(9L, lastTotalBytes)
        } finally {
            destination.delete()
        }
    }

    private class FakeHttpURLConnection(url: URL) : HttpURLConnection(url) {
        val requestHeaders = linkedMapOf<String, String>()

        private val body = "apk-bytes".toByteArray()

        override fun disconnect() = Unit

        override fun usingProxy(): Boolean = false

        override fun connect() = Unit

        override fun setRequestProperty(key: String, value: String?) {
            if (value != null) {
                requestHeaders[key] = value
            }
        }

        override fun getContentLengthLong(): Long = body.size.toLong()

        override fun getHeaderFieldLong(name: String?, DefaultValue: Long): Long {
            return if (name == HttpManagedAppDownloader.ARTIFACT_SIZE_HEADER) {
                body.size.toLong()
            } else {
                DefaultValue
            }
        }

        override fun getInputStream(): InputStream {
            return ByteArrayInputStream(body)
        }
    }
}

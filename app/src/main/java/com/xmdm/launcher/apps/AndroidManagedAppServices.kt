package com.xmdm.launcher.apps

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.PackageInstaller
import android.os.Build
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

class HttpManagedAppDownloader(
    private val connectionFactory: (URL) -> HttpURLConnection = { url ->
        url.openConnection() as HttpURLConnection
    },
) : ManagedAppDownloader {

    override suspend fun download(
        url: String,
        deviceSecret: String,
        destination: File,
        onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit,
    ): Long {
        val connection = connectionFactory(URL(url)).apply {
            connectTimeout = 15_000
            readTimeout = 30_000
            requestMethod = "GET"
            setRequestProperty(DEVICE_SECRET_HEADER, deviceSecret)
        }
        return try {
            connection.connect()
            val totalBytes = listOf(
                connection.contentLengthLong.takeIf { it > 0 },
                connection.getHeaderField(ARTIFACT_SIZE_HEADER)?.toLongOrNull()?.takeIf { it > 0 },
                connection.getHeaderField("Content-Length")?.toLongOrNull()?.takeIf { it > 0 },
            ).firstNotNullOfOrNull { it }
            var downloadedBytes = 0L
            var lastReportedBytes = 0L
            var lastReportedAt = 0L
            connection.inputStream.use { input ->
                destination.outputStream().use { output ->
                    val buffer = ByteArray(64 * 1024)
                    while (true) {
                        val read = input.read(buffer)
                        if (read < 0) {
                            break
                        }
                        output.write(buffer, 0, read)
                        downloadedBytes += read
                        val now = System.nanoTime()
                        val shouldReport = downloadedBytes == totalBytes
                            || downloadedBytes - lastReportedBytes >= REPORT_BYTES_INTERVAL
                            || now - lastReportedAt >= REPORT_TIME_INTERVAL_NANOS
                        if (shouldReport) {
                            onProgress(downloadedBytes, totalBytes)
                            lastReportedBytes = downloadedBytes
                            lastReportedAt = now
                        }
                    }
                }
            }
            if (downloadedBytes != lastReportedBytes) {
                onProgress(downloadedBytes, totalBytes)
            }
            destination.length()
        } finally {
            connection.disconnect()
        }
    }

    companion object {
        const val DEVICE_SECRET_HEADER = "X-XMDM-Device-Secret"
        const val ARTIFACT_SIZE_HEADER = "X-XMDM-Artifact-Size"
        private const val REPORT_BYTES_INTERVAL = 256 * 1024L
        private const val REPORT_TIME_INTERVAL_NANOS = 200_000_000L
    }
}

class AndroidManagedAppInstaller(
    private val context: Context,
) : ManagedAppInstaller {
    override fun listInstalledApps(): List<InstalledManagedApp> {
        val packages = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            context.packageManager.getInstalledPackages(PackageManager.PackageInfoFlags.of(0))
        } else {
            @Suppress("DEPRECATION")
            context.packageManager.getInstalledPackages(0)
        }
        return packages.map {
            val versionCode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                it.longVersionCode
            } else {
                @Suppress("DEPRECATION")
                it.versionCode.toLong()
            }
            InstalledManagedApp(
                packageName = it.packageName,
                versionCode = versionCode,
            )
        }
    }

    override suspend fun install(app: ManagedAppSpec, apkFile: File) {
        val packageInstaller = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
        params.setAppPackageName(app.packageName)
        val sessionId = packageInstaller.createSession(params)
        val action = "install:${app.packageName}:$sessionId"
        val completion = ManagedAppInstallResultRegistry.register(action)
        try {
            packageInstaller.openSession(sessionId).use { session ->
                session.openWrite("base.apk", 0, apkFile.length()).use { output ->
                    apkFile.inputStream().use { input ->
                        input.copyTo(output)
                    }
                }
                session.commit(resultIntentSender(action))
            }
            completion.await()
        } finally {
            ManagedAppInstallResultRegistry.clear(action)
        }
    }

    override suspend fun uninstall(packageName: String) {
        val action = "uninstall:$packageName:${System.nanoTime()}"
        val completion = ManagedAppInstallResultRegistry.register(action)
        try {
            context.packageManager.packageInstaller.uninstall(packageName, resultIntentSender(action))
            completion.await()
        } finally {
            ManagedAppInstallResultRegistry.clear(action)
        }
    }

    private fun resultIntentSender(action: String) = PendingIntent.getBroadcast(
        context,
        action.hashCode(),
        Intent(context, ManagedAppInstallResultReceiver::class.java).setAction(action),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    ).intentSender
}

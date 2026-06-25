package com.xmdm.launcher.apps

import android.app.admin.DevicePolicyManager
import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.content.pm.PackageInstaller
import android.os.Build
import android.util.Log
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
        Log.w(TAG, "install managed app package=${app.packageName} version=${app.versionName} starting")
        Log.w(TAG, "install managed app package=${app.packageName} using PackageInstaller session")
        val packageInstaller = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
        params.setAppPackageName(app.packageName)
        if (app.packageName == context.packageName && isTestOnlyApp()) {
            runCatching {
                PackageInstaller.SessionParams::class.java
                    .getMethod("setInstallAsTestOnly", Boolean::class.javaPrimitiveType)
                    .invoke(params, true)
            }
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            params.setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
        }
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
            try {
                completion.await()
            } catch (t: Throwable) {
                Log.w(TAG, "install managed app package=${app.packageName} session install failed, trying restore", t)
                if (installExistingSystemPackageIfPossible(app)) {
                    Log.w(TAG, "install managed app package=${app.packageName} restored existing system package after session failure")
                    return
                }
                throw t
            }
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

    private fun installExistingSystemPackageIfPossible(app: ManagedAppSpec): Boolean {
        val packageName = app.packageName
        val packageInfo = runCatching {
            getPackageInfoIncludingUninstalled(packageName)
        }.getOrElse {
            Log.w(TAG, "install managed app package=$packageName could not load package info for restore", it)
            return false
        }
        val appInfo = packageInfo.applicationInfo ?: return false
        if (!isSystemPackage(appInfo)) {
            Log.w(TAG, "install managed app package=$packageName is not a system package, skipping restore")
            return false
        }
        val devicePolicyManager = context.getSystemService(DevicePolicyManager::class.java) ?: return false
        if (!devicePolicyManager.isDeviceOwnerApp(context.packageName)) {
            Log.w(TAG, "install managed app package=$packageName is system package but app is not device owner, skipping restore")
            return false
        }
        Log.w(TAG, "install managed app package=$packageName restoring existing system package for device owner")
        val result = devicePolicyManager.installExistingPackage(
            ComponentName(context, com.xmdm.launcher.AdminReceiver::class.java),
            packageName,
        )
        if (!result) {
            Log.w(TAG, "install managed app package=$packageName installExistingPackage returned false")
            throw IllegalStateException("managed app restore failed")
        }
        val restoredVersion = getPackageVersionCode(packageName)
        if (restoredVersion != app.versionCode) {
            throw IllegalStateException(
                "managed app restore mismatch for $packageName (expected=${app.versionCode}, actual=$restoredVersion)",
            )
        }
        return true
    }

    private fun getPackageInfoIncludingUninstalled(packageName: String): PackageInfo {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            context.packageManager.getPackageInfo(
                packageName,
                PackageManager.PackageInfoFlags.of(PackageManager.MATCH_UNINSTALLED_PACKAGES.toLong()),
            )
        } else {
            @Suppress("DEPRECATION")
            context.packageManager.getPackageInfo(packageName, PackageManager.GET_UNINSTALLED_PACKAGES)
        }
    }

    private fun getPackageVersionCode(packageName: String): Long {
        val packageInfo = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            context.packageManager.getPackageInfo(packageName, PackageManager.PackageInfoFlags.of(0))
        } else {
            @Suppress("DEPRECATION")
            context.packageManager.getPackageInfo(packageName, 0)
        }
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            packageInfo.longVersionCode
        } else {
            @Suppress("DEPRECATION")
            packageInfo.versionCode.toLong()
        }
    }

    private fun isSystemPackage(appInfo: ApplicationInfo): Boolean {
        return appInfo.flags and ApplicationInfo.FLAG_SYSTEM != 0 ||
            appInfo.flags and ApplicationInfo.FLAG_UPDATED_SYSTEM_APP != 0
    }

    private fun isTestOnlyApp(): Boolean {
        return context.applicationInfo.flags and ApplicationInfo.FLAG_TEST_ONLY != 0
    }

    companion object {
        private const val TAG = "XmdmLauncher"
    }
}

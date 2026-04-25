package com.xmdm.launcher.apps

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.artifacts.ArtifactChecksumVerifier
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import java.io.File
import java.net.URI

data class ManagedAppSpec(
    val appId: String,
    val packageName: String,
    val name: String?,
    val versionId: String,
    val versionName: String,
    val versionCode: Long,
    val checksum: String,
    val downloadPath: String,
)

data class InstalledManagedApp(
    val packageName: String,
    val versionCode: Long,
)

sealed interface ManagedAppInstallProgress {
    data object Idle : ManagedAppInstallProgress
    data object VerifyingSnapshot : ManagedAppInstallProgress

    data class Downloading(
        val app: ManagedAppSpec,
        val index: Int,
        val total: Int,
        val downloadedBytes: Long,
        val totalBytes: Long?,
    ) : ManagedAppInstallProgress

    data class Installing(
        val app: ManagedAppSpec,
        val index: Int,
        val total: Int,
    ) : ManagedAppInstallProgress

    data class Uninstalling(
        val packageName: String,
        val index: Int,
        val total: Int,
    ) : ManagedAppInstallProgress

    data class Queued(
        val installed: List<String>,
        val uninstalled: List<String>,
    ) : ManagedAppInstallProgress

    data class Completed(
        val installed: List<String>,
        val uninstalled: List<String>,
    ) : ManagedAppInstallProgress

    data class Failed(
        val message: String,
    ) : ManagedAppInstallProgress
}

data class ManagedAppInstallResult(
    val installed: List<String>,
    val uninstalled: List<String>,
)

interface ManagedAppDownloader {
    suspend fun download(
        url: String,
        deviceSecret: String,
        destination: File,
        onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit = { _, _ -> },
    ): Long
}

interface ManagedAppInstaller {
    fun listInstalledApps(): List<InstalledManagedApp>
    suspend fun install(app: ManagedAppSpec, apkFile: File)
    suspend fun uninstall(packageName: String)
}

class ManagedAppInstallCoordinator(
    private val downloader: ManagedAppDownloader,
    private val installer: ManagedAppInstaller,
    private val snapshotVerifier: ConfigSnapshotVerifier = ConfigSnapshotVerifier(),
    private val checksumVerifier: ArtifactChecksumVerifier = ArtifactChecksumVerifier(),
) {
    suspend fun apply(
        snapshotJson: String,
        deviceSecret: String,
        serverUrl: String,
        previousSnapshotJson: String? = null,
        onProgress: (ManagedAppInstallProgress) -> Unit = {},
    ): ManagedAppInstallResult {
        onProgress(ManagedAppInstallProgress.VerifyingSnapshot)
        val verified = snapshotVerifier.verify(snapshotJson, deviceSecret)
        val desiredApps = parseApps(verified)
        val previousApps = previousSnapshotJson
            ?.let { snapshotVerifier.verify(it, deviceSecret) }
            ?.let(::parseApps)
            .orEmpty()

        val installedByPackage = installer.listInstalledApps().associateBy { it.packageName }
        val installed = mutableListOf<String>()
        for ((index, app) in desiredApps.withIndex()) {
            val current = installedByPackage[app.packageName]
            if (current != null && current.versionCode == app.versionCode) {
                continue
            }
            onProgress(
                ManagedAppInstallProgress.Downloading(
                    app = app,
                    index = index + 1,
                    total = desiredApps.size,
                    downloadedBytes = 0L,
                    totalBytes = null,
                ),
            )
            val resolvedUrl = resolveUrl(serverUrl, app.downloadPath)
            val apkFile = File.createTempFile("managed-app-", ".apk")
            try {
                downloader.download(
                    url = resolvedUrl,
                    deviceSecret = deviceSecret,
                    destination = apkFile,
                ) { downloadedBytes, totalBytes ->
                    onProgress(
                        ManagedAppInstallProgress.Downloading(
                            app = app,
                            index = index + 1,
                            total = desiredApps.size,
                            downloadedBytes = downloadedBytes,
                            totalBytes = totalBytes,
                        ),
                    )
                }
                checksumVerifier.verify(apkFile, app.checksum)
                onProgress(ManagedAppInstallProgress.Installing(app, index + 1, desiredApps.size))
                installer.install(app, apkFile)
                installed += app.packageName
            } finally {
                if (!apkFile.delete()) {
                    apkFile.deleteOnExit()
                }
            }
        }

        val desiredPackages = desiredApps.map { it.packageName }.toSet()
        val previousPackages = previousApps.map { it.packageName }.toSet()
        val removed = previousPackages - desiredPackages
        val uninstalled = mutableListOf<String>()
        for ((index, packageName) in removed.withIndex()) {
            onProgress(ManagedAppInstallProgress.Uninstalling(packageName, index + 1, removed.size))
            installer.uninstall(packageName)
            uninstalled += packageName
        }

        return ManagedAppInstallResult(
            installed = installed,
            uninstalled = uninstalled,
        )
    }

    private fun parseApps(snapshot: JsonObject): List<ManagedAppSpec> {
        val apps = snapshot.getAsJsonArray("apps") ?: return emptyList()
        return apps.mapNotNull { element -> parseApp(element) }
    }

    private fun parseApp(element: JsonElement): ManagedAppSpec? {
        if (!element.isJsonObject) {
            return null
        }
        val app = element.asJsonObject
        val appId = app.string("appId") ?: return null
        val packageName = app.string("packageName") ?: return null
        val name = app.string("name")
        val versionId = app.string("versionId") ?: return null
        val versionName = app.string("versionName") ?: return null
        val versionCode = app.long("versionCode") ?: return null
        val checksum = app.string("checksum") ?: return null
        val downloadPath = app.string("downloadPath") ?: return null
        return ManagedAppSpec(
            appId = appId,
            packageName = packageName,
            name = name,
            versionId = versionId,
            versionName = versionName,
            versionCode = versionCode,
            checksum = checksum,
            downloadPath = downloadPath,
        )
    }

    private fun resolveUrl(serverUrl: String, downloadPath: String): String {
        return URI(serverUrl).resolve(downloadPath).toString()
    }

    private fun JsonObject.string(name: String): String? {
        val value = get(name) ?: return null
        if (value.isJsonNull) {
            return null
        }
        return value.asString.takeIf { it.isNotBlank() }
    }

    private fun JsonObject.long(name: String): Long? {
        val value = get(name) ?: return null
        if (value.isJsonNull) {
            return null
        }
        return runCatching { value.asLong }.getOrNull()
    }
}

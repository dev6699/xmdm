package com.xmdm.launcher.apps

import com.xmdm.launcher.artifacts.ArtifactChecksumVerifier
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import com.xmdm.launcher.BuildConfig
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class ManagedAppInstallCoordinatorTest {
    @Test
    fun installsDesiredAppsAndRemovesPreviouslyManagedApps() = runTest {
        val verifier = ConfigSnapshotVerifier()
        val checksumVerifier = ArtifactChecksumVerifier()
        val appBytes = "app-payload".toByteArray()
        val previousBytes = "old-payload".toByteArray()
        val appChecksum = checksumVerifier.sha256Base64Url(appBytes)
        val previousChecksum = checksumVerifier.sha256Base64Url(previousBytes)
        val unsignedCurrent = """
            {
              "version":"7",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"app-1",
                  "packageName":"com.example.app",
                  "versionId":"ver-1",
                  "versionName":"1.0.0",
                  "versionCode":100,
                  "checksum":"$appChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/app-1/versions/ver-1/artifact"
                }
              ],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val unsignedPrevious = """
            {
              "version":"6",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"app-old",
                  "packageName":"com.example.old",
                  "versionId":"ver-old",
                  "versionName":"0.9.0",
                  "versionCode":9,
                  "checksum":"$previousChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/app-old/versions/ver-old/artifact"
                }
              ],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val current = """
            {
              "version":"7",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"app-1",
                  "packageName":"com.example.app",
                  "versionId":"ver-1",
                  "versionName":"1.0.0",
                  "versionCode":100,
                  "checksum":"$appChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/app-1/versions/ver-1/artifact"
                }
              ],
              "files":[],
              "certificates":[],
              "signature":"${verifier.sign(unsignedCurrent, "secret-abc")}"
            }
        """.trimIndent()
        val previous = """
            {
              "version":"6",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"app-old",
                  "packageName":"com.example.old",
                  "versionId":"ver-old",
                  "versionName":"0.9.0",
                  "versionCode":9,
                  "checksum":"$previousChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/app-old/versions/ver-old/artifact"
                }
              ],
              "files":[],
              "certificates":[],
              "signature":"${verifier.sign(unsignedPrevious, "secret-abc")}"
            }
        """.trimIndent()

        val downloads = mutableListOf<String>()
        val installs = mutableListOf<ManagedAppSpec>()
        val uninstalls = mutableListOf<String>()
        val downloadProgress = mutableListOf<Pair<Long, Long?>>()
        val coordinator = ManagedAppInstallCoordinator(
            downloader = object : ManagedAppDownloader {
                override suspend fun download(
                    url: String,
                    deviceSecret: String,
                    destination: File,
                    onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit,
                ): Long {
                    downloads += url
                    assertEquals("secret-abc", deviceSecret)
                    destination.writeBytes(appBytes)
                    onProgress(appBytes.size.toLong(), appBytes.size.toLong())
                    downloadProgress += appBytes.size.toLong() to appBytes.size.toLong()
                    return appBytes.size.toLong()
                }
            },
            installer = object : ManagedAppInstaller {
                override fun listInstalledApps(): List<InstalledManagedApp> {
                    return listOf(
                        InstalledManagedApp(packageName = "com.example.app", versionCode = 50),
                        InstalledManagedApp(packageName = "com.example.old", versionCode = 9),
                    )
                }

                override suspend fun install(app: ManagedAppSpec, apkFile: File) {
                    installs += app
                    assertTrue(apkFile.readBytes().contentEquals(appBytes))
                }

                override suspend fun uninstall(packageName: String) {
                    uninstalls += packageName
                }
            },
        )

        val result = coordinator.apply(
            snapshotJson = current,
            deviceSecret = "secret-abc",
            serverUrl = "https://mdm.example",
            previousSnapshotJson = previous,
        )

        assertEquals(listOf("https://mdm.example/api/v1/devices/device-123/apps/app-1/versions/ver-1/artifact"), downloads)
        assertEquals(listOf(appBytes.size.toLong() to appBytes.size.toLong()), downloadProgress)
        assertEquals(listOf("com.example.app"), installs.map { it.packageName })
        assertEquals(listOf("com.example.old"), uninstalls)
        assertEquals(listOf("com.example.app"), result.installed)
        assertEquals(listOf("com.example.old"), result.uninstalled)
    }

    @Test
    fun installsLauncherLastWhenItIsPartOfTheSameBatch() = runTest {
        val verifier = ConfigSnapshotVerifier()
        val checksumVerifier = ArtifactChecksumVerifier()
        val launcherBytes = "launcher-payload".toByteArray()
        val otherBytes = "other-payload".toByteArray()
        val launcherChecksum = checksumVerifier.sha256Base64Url(launcherBytes)
        val otherChecksum = checksumVerifier.sha256Base64Url(otherBytes)
        val unsigned = """
            {
              "version":"8",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"launcher-app",
                  "packageName":"${BuildConfig.APPLICATION_ID}",
                  "versionId":"launcher-ver",
                  "versionName":"0.2.0",
                  "versionCode":2,
                  "checksum":"$launcherChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/launcher-app/versions/launcher-ver/artifact"
                },
                {
                  "appId":"other-app",
                  "packageName":"com.example.other",
                  "versionId":"other-ver",
                  "versionName":"1.0.0",
                  "versionCode":10,
                  "checksum":"$otherChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/other-app/versions/other-ver/artifact"
                }
              ],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val signed = """
            {
              "version":"8",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"launcher-app",
                  "packageName":"${BuildConfig.APPLICATION_ID}",
                  "versionId":"launcher-ver",
                  "versionName":"0.2.0",
                  "versionCode":2,
                  "checksum":"$launcherChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/launcher-app/versions/launcher-ver/artifact"
                },
                {
                  "appId":"other-app",
                  "packageName":"com.example.other",
                  "versionId":"other-ver",
                  "versionName":"1.0.0",
                  "versionCode":10,
                  "checksum":"$otherChecksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/other-app/versions/other-ver/artifact"
                }
              ],
              "files":[],
              "certificates":[],
              "signature":"${verifier.sign(unsigned, "secret-abc")}"
            }
        """.trimIndent()

        val installOrder = mutableListOf<String>()
        val coordinator = ManagedAppInstallCoordinator(
            downloader = object : ManagedAppDownloader {
                override suspend fun download(
                    url: String,
                    deviceSecret: String,
                    destination: File,
                    onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit,
                ): Long {
                    assertEquals("secret-abc", deviceSecret)
                    when {
                        url.contains("launcher-app") -> destination.writeBytes(launcherBytes)
                        url.contains("other-app") -> destination.writeBytes(otherBytes)
                        else -> error("unexpected url: $url")
                    }
                    onProgress(destination.length(), destination.length())
                    return destination.length()
                }
            },
            installer = object : ManagedAppInstaller {
                override fun listInstalledApps(): List<InstalledManagedApp> = emptyList()

                override suspend fun install(app: ManagedAppSpec, apkFile: File) {
                    installOrder += app.packageName
                    when (app.packageName) {
                        BuildConfig.APPLICATION_ID -> assertTrue(apkFile.readBytes().contentEquals(launcherBytes))
                        "com.example.other" -> assertTrue(apkFile.readBytes().contentEquals(otherBytes))
                        else -> error("unexpected package: ${app.packageName}")
                    }
                }

                override suspend fun uninstall(packageName: String) = Unit
            },
        )

        coordinator.apply(
            snapshotJson = signed,
            deviceSecret = "secret-abc",
            serverUrl = "https://mdm.example",
        )

        assertEquals(listOf("com.example.other", BuildConfig.APPLICATION_ID), installOrder)
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsTamperedAppArtifacts() = runTest {
        val verifier = ConfigSnapshotVerifier()
        val checksumVerifier = ArtifactChecksumVerifier()
        val checksum = checksumVerifier.sha256Base64Url("app-payload".toByteArray())
        val unsigned = """
            {
              "version":"7",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"app-1",
                  "packageName":"com.example.app",
                  "versionId":"ver-1",
                  "versionName":"1.0.0",
                  "versionCode":100,
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/app-1/versions/ver-1/artifact"
                }
              ],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val signed = """
            {
              "version":"7",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"app-1",
                  "packageName":"com.example.app",
                  "versionId":"ver-1",
                  "versionName":"1.0.0",
                  "versionCode":100,
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/apps/app-1/versions/ver-1/artifact"
                }
              ],
              "files":[],
              "certificates":[],
              "signature":"${verifier.sign(unsigned, "secret-abc")}"
            }
        """.trimIndent()

        val coordinator = ManagedAppInstallCoordinator(
            downloader = object : ManagedAppDownloader {
                override suspend fun download(
                    url: String,
                    deviceSecret: String,
                    destination: File,
                    onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit,
                ): Long {
                    destination.writeBytes("tampered".toByteArray())
                    return destination.length()
                }
            },
            installer = object : ManagedAppInstaller {
                override fun listInstalledApps(): List<InstalledManagedApp> = emptyList()
                override suspend fun install(app: ManagedAppSpec, apkFile: File) = Unit
                override suspend fun uninstall(packageName: String) = Unit
            },
        )

        coordinator.apply(
            snapshotJson = signed,
            deviceSecret = "secret-abc",
            serverUrl = "https://mdm.example",
        )
    }
}

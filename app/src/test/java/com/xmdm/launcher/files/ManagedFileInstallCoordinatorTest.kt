package com.xmdm.launcher.files

import com.xmdm.launcher.apps.ManagedAppDownloader
import com.xmdm.launcher.artifacts.ArtifactChecksumVerifier
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files

class ManagedFileInstallCoordinatorTest {
    @Test
    fun downloadsFilesAndRemovesMissingFiles() = runTest {
        val verifier = ConfigSnapshotVerifier()
        val checksumVerifier = ArtifactChecksumVerifier()
        val templateBytes = "hello DEVICE_NUMBER".toByteArray()
        val checksum = checksumVerifier.sha256Base64Url(templateBytes)
        val unsignedCurrent = """
            {
              "version":"9",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[
                {
                  "fileId":"file-1",
                  "name":"device-config.txt",
                  "path":"configs/device-config.txt",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/managed-files/file-1/artifact",
                  "mimeType":"text/plain",
                  "description":"Device config",
                  "remove":false,
                  "replaceVariables":true
                }
              ],
              "certificates":[]
            }
        """.trimIndent()
        val unsignedPrevious = """
            {
              "version":"8",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[
                {
                  "fileId":"file-old",
                  "name":"old.txt",
                  "path":"configs/old.txt",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/managed-files/file-old/artifact",
                  "mimeType":"text/plain",
                  "description":"Old file",
                  "remove":false,
                  "replaceVariables":false
                }
              ],
              "certificates":[]
            }
        """.trimIndent()
        val current = """
            {
              "version":"9",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[
                {
                  "fileId":"file-1",
                  "name":"device-config.txt",
                  "path":"configs/device-config.txt",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/managed-files/file-1/artifact",
                  "mimeType":"text/plain",
                  "description":"Device config",
                  "remove":false,
                  "replaceVariables":true
                }
              ],
              "certificates":[],
              "signature":"${verifier.sign(unsignedCurrent, "secret-abc")}"
            }
        """.trimIndent()
        val previous = """
            {
              "version":"8",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[
                {
                  "fileId":"file-old",
                  "name":"old.txt",
                  "path":"configs/old.txt",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/managed-files/file-old/artifact",
                  "mimeType":"text/plain",
                  "description":"Old file",
                  "remove":false,
                  "replaceVariables":false
                }
              ],
              "certificates":[],
              "signature":"${verifier.sign(unsignedPrevious, "secret-abc")}"
            }
        """.trimIndent()

        val rootDir = Files.createTempDirectory("managed-files").toFile()
        val downloads = mutableListOf<String>()
        val coordinator = ManagedFileInstallCoordinator(
            downloader = object : ManagedAppDownloader {
                override suspend fun download(
                    url: String,
                    deviceSecret: String,
                    destination: File,
                    onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit,
                ): Long {
                    downloads += url
                    assertEquals("secret-abc", deviceSecret)
                    destination.writeBytes(templateBytes)
                    onProgress(templateBytes.size.toLong(), templateBytes.size.toLong())
                    return templateBytes.size.toLong()
                }
            },
            rootDir = rootDir,
        )

        val result = coordinator.apply(
            snapshotJson = current,
            deviceSecret = "secret-abc",
            serverUrl = "https://mdm.example",
            previousSnapshotJson = previous,
        )

        assertEquals(listOf("https://mdm.example/api/v1/devices/device-123/managed-files/file-1/artifact"), downloads)
        assertEquals(listOf("configs/device-config.txt"), result.written)
        assertEquals(listOf("configs/old.txt"), result.removed)

        val writtenFile = File(rootDir, "configs/device-config.txt")
        assertTrue(writtenFile.exists())
        assertEquals("hello DEVICE_NUMBER", writtenFile.readText())
        assertFalse(File(rootDir, "configs/old.txt").exists())
    }

    @Test
    fun emptySnapshotRemovesPreviouslyAppliedFiles() = runTest {
        val verifier = ConfigSnapshotVerifier()
        val checksumVerifier = ArtifactChecksumVerifier()
        val checksum = checksumVerifier.sha256Base64Url("unused".toByteArray())
        val unsignedPrevious = """
            {
              "version":"8",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[
                {
                  "fileId":"file-old",
                  "name":"old.txt",
                  "path":"configs/old.txt",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/managed-files/file-old/artifact",
                  "mimeType":"text/plain",
                  "description":"Old file",
                  "remove":false,
                  "replaceVariables":false
                }
              ],
              "certificates":[]
            }
        """.trimIndent()
        val previous = """
            {
              "version":"8",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[
                {
                  "fileId":"file-old",
                  "name":"old.txt",
                  "path":"configs/old.txt",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/managed-files/file-old/artifact",
                  "mimeType":"text/plain",
                  "description":"Old file",
                  "remove":false,
                  "replaceVariables":false
                }
              ],
              "certificates":[],
              "signature":"${verifier.sign(unsignedPrevious, "secret-abc")}"
            }
        """.trimIndent()
        val unsignedCurrent = """
            {
              "version":"9",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[],
              "certificates":[]
            }
        """.trimIndent()
        val current = """
            {
              "version":"9",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[],
              "files":[],
              "certificates":[],
              "signature":"${verifier.sign(unsignedCurrent, "secret-abc")}"
            }
        """.trimIndent()

        val rootDir = Files.createTempDirectory("managed-files-empty").toFile()
        val existing = File(rootDir, "configs/old.txt")
        existing.parentFile?.mkdirs()
        existing.writeText("stale")

        val coordinator = ManagedFileInstallCoordinator(
            downloader = object : ManagedAppDownloader {
                override suspend fun download(
                    url: String,
                    deviceSecret: String,
                    destination: File,
                    onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit,
                ): Long {
                    error("did not expect a download for empty snapshot")
                }
            },
            rootDir = rootDir,
        )

        val result = coordinator.apply(
            snapshotJson = current,
            deviceSecret = "secret-abc",
            serverUrl = "https://mdm.example",
            previousSnapshotJson = previous,
        )

        assertEquals(emptyList<String>(), result.written)
        assertEquals(listOf("configs/old.txt"), result.removed)
        assertFalse(existing.exists())
    }
}

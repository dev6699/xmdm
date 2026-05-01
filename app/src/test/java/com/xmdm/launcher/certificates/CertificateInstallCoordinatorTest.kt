package com.xmdm.launcher.certificates

import com.xmdm.launcher.apps.ManagedAppDownloader
import com.xmdm.launcher.artifacts.ArtifactChecksumVerifier
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class CertificateInstallCoordinatorTest {
    @Test
    fun downloadsAndInstallsActiveCertificates() = runTest {
        val verifier = ConfigSnapshotVerifier()
        val checksumVerifier = ArtifactChecksumVerifier()
        val certBytes = "certificate-payload".toByteArray()
        val checksum = checksumVerifier.sha256Base64Url(certBytes)
        val unsigned = """
            {
              "version":"13",
              "runtime":{
                "mqttAddress":"127.0.0.1:1883",
                "commandPollIntervalMs":1000,
                "configSyncIntervalMs":1000
              },
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"kioskMode":false},
              "apps":[],
              "files":[],
              "certificates":[
                {
                  "id":"cert-1",
                  "name":"wifi-root-ca",
                  "artifactId":"artifact-1",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/certificates/cert-1/artifact"
                }
              ]
            }
        """.trimIndent()
        val signed = """
            {
              "version":"13",
              "runtime":{
                "mqttAddress":"127.0.0.1:1883",
                "commandPollIntervalMs":1000,
                "configSyncIntervalMs":1000
              },
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"kioskMode":false},
              "apps":[],
              "files":[],
              "certificates":[
                {
                  "id":"cert-1",
                  "name":"wifi-root-ca",
                  "artifactId":"artifact-1",
                  "checksum":"$checksum",
                  "downloadPath":"/api/v1/devices/device-123/certificates/cert-1/artifact"
                }
              ],
              "signature":"${verifier.sign(unsigned, "secret-abc")}"
            }
        """.trimIndent()

        val downloads = mutableListOf<String>()
        val installs = mutableListOf<ByteArray>()
        val coordinator = CertificateInstallCoordinator(
            downloader = object : ManagedAppDownloader {
                override suspend fun download(
                    url: String,
                    deviceSecret: String,
                    destination: File,
                    onProgress: (downloadedBytes: Long, totalBytes: Long?) -> Unit,
                ): Long {
                    downloads += url
                    assertEquals("secret-abc", deviceSecret)
                    destination.writeBytes(certBytes)
                    onProgress(certBytes.size.toLong(), certBytes.size.toLong())
                    return certBytes.size.toLong()
                }
            },
            installer = object : CertificateInstaller {
                override fun isDeviceOwnerApp(): Boolean = true
                override fun install(certBytes: ByteArray): Boolean {
                    installs += certBytes
                    return true
                }
            },
        )

        val result = coordinator.apply(
            snapshotJson = signed,
            deviceSecret = "secret-abc",
            serverUrl = "https://mdm.example",
        )

        assertEquals(listOf("https://mdm.example/api/v1/devices/device-123/certificates/cert-1/artifact"), downloads)
        assertEquals(1, result.installed.size)
        assertEquals("cert-1", result.installed.single())
        assertEquals(1, installs.size)
        assertTrue(installs.single().contentEquals(certBytes))
    }

    @Test
    fun certificateBucketVersionChangesWhenCertificatesChange() {
        val checksum1 = ArtifactChecksumVerifier().sha256Base64Url("cert-1".toByteArray())
        val checksum2 = ArtifactChecksumVerifier().sha256Base64Url("cert-2".toByteArray())
        val snapshot1 = """
            {
              "version":"13",
              "runtime":{"mqttAddress":"127.0.0.1:1883","commandPollIntervalMs":1000,"configSyncIntervalMs":1000},
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"kioskMode":false},
              "apps":[],
              "files":[],
              "certificates":[
                {"id":"cert-1","name":"wifi-root-ca","artifactId":"artifact-1","checksum":"$checksum1","downloadPath":"/api/v1/devices/device-123/certificates/cert-1/artifact"}
              ]
            }
        """.trimIndent()
        val snapshot2 = """
            {
              "version":"13",
              "runtime":{"mqttAddress":"127.0.0.1:1883","commandPollIntervalMs":1000,"configSyncIntervalMs":1000},
              "device":{"deviceId":"device-123","deviceIdUse":"serial"},
              "policy":{"kioskMode":false},
              "apps":[],
              "files":[],
              "certificates":[
                {"id":"cert-2","name":"wifi-root-ca-2","artifactId":"artifact-2","checksum":"$checksum2","downloadPath":"/api/v1/devices/device-123/certificates/cert-2/artifact"}
              ]
            }
        """.trimIndent()

        val version1 = certificateBucketVersion(snapshot1)
        val version2 = certificateBucketVersion(snapshot2)

        assertNotEquals(version1, version2)
    }
}

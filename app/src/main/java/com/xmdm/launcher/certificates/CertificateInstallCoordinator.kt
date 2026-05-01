package com.xmdm.launcher.certificates

import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.AdminReceiver
import com.xmdm.launcher.artifacts.ArtifactChecksumVerifier
import com.xmdm.launcher.apps.ManagedAppDownloader
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import java.io.File
import java.net.URI
import java.nio.ByteBuffer
import java.security.MessageDigest
import java.nio.charset.StandardCharsets

data class ManagedCertificateSpec(
    val certificateId: String,
    val name: String?,
    val artifactId: String,
    val checksum: String,
    val downloadPath: String,
)

data class CertificateInstallResult(
    val installed: List<String>,
)

interface CertificateInstaller {
    fun isDeviceOwnerApp(): Boolean
    fun install(certBytes: ByteArray): Boolean
}

class AndroidCertificateInstaller(
    private val context: Context,
) : CertificateInstaller {
    private val devicePolicyManager: DevicePolicyManager? by lazy {
        context.getSystemService(DevicePolicyManager::class.java)
    }
    private val adminComponent by lazy {
        ComponentName(context, AdminReceiver::class.java)
    }

    override fun isDeviceOwnerApp(): Boolean {
        return devicePolicyManager?.isDeviceOwnerApp(context.packageName) == true
    }

    override fun install(certBytes: ByteArray): Boolean {
        val dpm = devicePolicyManager ?: return false
        if (!isDeviceOwnerApp()) {
            return false
        }
        return runCatching { dpm.installCaCert(adminComponent, certBytes) }.getOrDefault(false)
    }
}

class CertificateInstallCoordinator(
    private val downloader: ManagedAppDownloader,
    private val installer: CertificateInstaller,
    private val snapshotVerifier: ConfigSnapshotVerifier = ConfigSnapshotVerifier(),
    private val checksumVerifier: ArtifactChecksumVerifier = ArtifactChecksumVerifier(),
) {
    suspend fun apply(
        snapshotJson: String,
        deviceSecret: String,
        serverUrl: String,
    ): CertificateInstallResult {
        require(installer.isDeviceOwnerApp()) { "device owner app is unavailable" }

        val verified = snapshotVerifier.verify(snapshotJson, deviceSecret)
        val desiredCertificates = parseCertificates(verified)
        val installed = mutableListOf<String>()

        for (cert in desiredCertificates) {
            val resolvedUrl = resolveUrl(serverUrl, cert.downloadPath)
            val certFile = File.createTempFile("managed-cert-", ".pem")
            try {
                downloader.download(
                    url = resolvedUrl,
                    deviceSecret = deviceSecret,
                    destination = certFile,
                )
                checksumVerifier.verify(certFile, cert.checksum)
                val bytes = certFile.readBytes()
                if (!installer.install(bytes)) {
                    throw IllegalStateException("certificate install failed for ${cert.certificateId}")
                }
                installed += cert.certificateId
            } finally {
                if (!certFile.delete()) {
                    certFile.deleteOnExit()
                }
            }
        }

        return CertificateInstallResult(installed = installed)
    }

    private fun parseCertificates(snapshot: JsonObject): List<ManagedCertificateSpec> {
        val certificates = snapshot.getAsJsonArray("certificates") ?: return emptyList()
        return certificates.mapNotNull { element -> parseCertificate(element) }
            .sortedBy { it.certificateId }
    }

    private fun parseCertificate(element: JsonElement): ManagedCertificateSpec? {
        if (!element.isJsonObject) {
            return null
        }
        val cert = element.asJsonObject
        val certificateId = cert.string("id") ?: return null
        val checksum = cert.string("checksum") ?: return null
        val artifactId = cert.string("artifactId") ?: return null
        val downloadPath = cert.string("downloadPath") ?: return null
        return ManagedCertificateSpec(
            certificateId = certificateId,
            name = cert.string("name"),
            artifactId = artifactId,
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
}

fun certificateBucketVersion(snapshotJson: String): Long {
    val root = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
        ?: return 0L
    val certificates = root.getAsJsonArray("certificates") ?: return 0L
    if (certificates.size() == 0) {
        return 0L
    }
    val normalized = certificates.mapNotNull { element ->
        if (!element.isJsonObject) {
            return@mapNotNull null
        }
        val cert = element.asJsonObject
        linkedMapOf(
            "id" to cert.string("id"),
            "name" to cert.string("name"),
            "artifactId" to cert.string("artifactId"),
            "checksum" to cert.string("checksum"),
            "downloadPath" to cert.string("downloadPath"),
        )
    }.sortedBy { it["id"].orEmpty() + "|" + it["artifactId"].orEmpty() + "|" + it["checksum"].orEmpty() }
    if (normalized.isEmpty()) {
        return 0L
    }
    val canonical = normalized.toString()
    return fingerprint64(canonical)
}

private fun fingerprint64(value: String): Long {
    val digest = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(StandardCharsets.UTF_8))
    return ByteBuffer.wrap(digest, 0, Long.SIZE_BYTES).long
}

private fun JsonObject.string(name: String): String? {
    val value = get(name) ?: return null
    if (value.isJsonNull) {
        return null
    }
    return value.asString.takeIf { it.isNotBlank() }
}

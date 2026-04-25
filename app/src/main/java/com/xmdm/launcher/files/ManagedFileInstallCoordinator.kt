package com.xmdm.launcher.files

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.artifacts.ArtifactChecksumVerifier
import com.xmdm.launcher.apps.ManagedAppDownloader
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import java.io.File
import java.net.URI

data class ManagedFileSpec(
    val fileId: String,
    val name: String?,
    val path: String,
    val checksum: String,
    val downloadPath: String,
    val mimeType: String?,
    val description: String?,
    val remove: Boolean,
    val replaceVariables: Boolean,
)

data class ManagedFileInstallResult(
    val written: List<String>,
    val removed: List<String>,
)

class ManagedFileInstallCoordinator(
    private val downloader: ManagedAppDownloader,
    private val rootDir: File,
    private val snapshotVerifier: ConfigSnapshotVerifier = ConfigSnapshotVerifier(),
    private val checksumVerifier: ArtifactChecksumVerifier = ArtifactChecksumVerifier(),
) {
    suspend fun apply(
        snapshotJson: String,
        deviceSecret: String,
        serverUrl: String,
        deviceId: String,
        deviceIdUse: String,
        bootstrapExtrasJson: String,
        previousSnapshotJson: String? = null,
    ): ManagedFileInstallResult {
        val verified = snapshotVerifier.verify(snapshotJson, deviceSecret)
        val desiredFiles = parseFiles(verified)
        val previousFiles = previousSnapshotJson
            ?.let { snapshotVerifier.verify(it, deviceSecret) }
            ?.let(::parseFiles)
            .orEmpty()

        val written = mutableListOf<String>()
        val removed = mutableListOf<String>()
        val desiredPaths = desiredFiles.map { it.path }.toSet()
        val bootstrapValues = templateValues(deviceId, deviceIdUse, bootstrapExtrasJson)

        for (file in desiredFiles) {
            val target = resolveTarget(file.path)
            if (file.remove) {
                deleteIfPresent(target)
                removed += file.path
                continue
            }

            val resolvedUrl = resolveUrl(serverUrl, file.downloadPath)
            val tempFile = File.createTempFile("managed-file-", ".tmp")
            try {
                downloader.download(
                    url = resolvedUrl,
                    deviceSecret = deviceSecret,
                    destination = tempFile,
                )
                checksumVerifier.verify(tempFile, file.checksum)
                if (file.replaceVariables) {
                    val content = tempFile.readText(Charsets.UTF_8)
                    target.parentFile?.mkdirs()
                    target.writeText(renderTemplate(content, bootstrapValues), Charsets.UTF_8)
                } else {
                    target.parentFile?.mkdirs()
                    tempFile.copyTo(target, overwrite = true)
                }
                written += file.path
            } finally {
                if (!tempFile.delete()) {
                    tempFile.deleteOnExit()
                }
            }
        }

        for (file in previousFiles) {
            if (file.path in desiredPaths) {
                continue
            }
            val target = resolveTarget(file.path)
            deleteIfPresent(target)
            removed += file.path
        }

        return ManagedFileInstallResult(
            written = written.toList(),
            removed = removed.toList(),
        )
    }

    private fun parseFiles(snapshot: JsonObject): List<ManagedFileSpec> {
        val files = snapshot.getAsJsonArray("files") ?: return emptyList()
        return files.mapNotNull { element -> parseFile(element) }
    }

    private fun parseFile(element: JsonElement): ManagedFileSpec? {
        if (!element.isJsonObject) {
            return null
        }
        val file = element.asJsonObject
        val fileId = file.string("fileId") ?: return null
        val path = file.string("path") ?: file.string("name") ?: fileId
        val checksum = file.string("checksum") ?: return null
        val downloadPath = file.string("downloadPath") ?: return null
        return ManagedFileSpec(
            fileId = fileId,
            name = file.string("name"),
            path = path,
            checksum = checksum,
            downloadPath = downloadPath,
            mimeType = file.string("mimeType"),
            description = file.string("description"),
            remove = file.bool("remove") ?: false,
            replaceVariables = file.bool("replaceVariables") ?: false,
        )
    }

    private fun resolveTarget(path: String): File {
        val safePath = path.replace('\\', '/')
            .split('/')
            .mapNotNull { segment ->
                val trimmed = segment.trim()
                when (trimmed) {
                    "", "." -> null
                    ".." -> null
                    else -> trimmed
                }
            }
            .joinToString(File.separator)
        return File(rootDir, safePath.ifBlank { "managed-file" })
    }

    private fun resolveUrl(serverUrl: String, downloadPath: String): String {
        return URI(serverUrl).resolve(downloadPath).toString()
    }

    private fun deleteIfPresent(file: File) {
        if (file.exists() && !file.delete()) {
            file.deleteOnExit()
        }
    }

    private fun renderTemplate(content: String, values: Map<String, String>): String {
        var rendered = content
        for ((key, value) in values) {
            rendered = rendered.replace(key, value)
        }
        return rendered
    }

    private fun templateValues(
        deviceId: String,
        deviceIdUse: String,
        bootstrapExtrasJson: String,
    ): Map<String, String> {
        val values = linkedMapOf(
            "DEVICE_NUMBER" to deviceId,
            "DEVICE_ID" to deviceId,
            "DEVICE_ID_USE" to deviceIdUse,
            "IMEI" to if (deviceIdUse.equals("imei", ignoreCase = true)) deviceId else "",
        )
        if (bootstrapExtrasJson.isBlank()) {
            return values
        }
        val extras = JsonParser.parseString(bootstrapExtrasJson).asJsonObject
        for ((name, element) in extras.entrySet()) {
            val value = when {
                element.isJsonNull -> ""
                element.isJsonPrimitive -> element.asString
                else -> element.toString()
            }
            if (value.isNotBlank()) {
                values[name] = value
                values[name.uppercase()] = value
            }
        }
        return values
    }

    private fun JsonObject.string(name: String): String? {
        val value = get(name) ?: return null
        if (value.isJsonNull) {
            return null
        }
        return value.asString.takeIf { it.isNotBlank() }
    }

    private fun JsonObject.bool(name: String): Boolean? {
        val value = get(name) ?: return null
        if (value.isJsonNull) {
            return null
        }
        return runCatching { value.asBoolean }.getOrNull()
    }
}

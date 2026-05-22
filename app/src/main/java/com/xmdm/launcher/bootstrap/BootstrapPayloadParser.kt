package com.xmdm.launcher.bootstrap

import com.google.gson.Gson
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.state.BootstrapState

data class ParsedBootstrapPayload(
    val bootstrap: BootstrapState,
    val extrasJson: String,
)

class BootstrapPayloadParser(
    private val gson: Gson = Gson(),
) {
    fun parse(rawJson: String): ParsedBootstrapPayload {
        val root = JsonParser.parseString(rawJson).asJsonObject
        val source = provisioningSource(root)

        val serverUrl = stringValue(source, SERVER_URL_KEYS)
            ?: error("bootstrap payload is missing a server URL")
        val secondaryServerUrl = stringValue(source, SECONDARY_SERVER_URL_KEYS)
        val enrollmentToken = stringValue(source, ENROLLMENT_TOKEN_KEYS)
            ?: error("bootstrap payload is missing an enrollment token")
        val deviceId = stringValue(source, DEVICE_ID_KEYS)
        val extras = provisioningExtras(source)

        return ParsedBootstrapPayload(
            bootstrap = BootstrapState(
                serverUrl = serverUrl,
                secondaryServerUrl = secondaryServerUrl,
                enrollmentToken = enrollmentToken,
                deviceId = deviceId,
                bootstrapExtrasJson = gson.toJson(extras),
                rawJson = rawJson,
            ),
            extrasJson = gson.toJson(extras),
        )
    }

    private fun provisioningSource(root: JsonObject): JsonObject {
        val provisioningExtras = root.getObject(PROVISIONING_ADMIN_EXTRAS_BUNDLE)
        return provisioningExtras ?: root
    }

    private fun provisioningExtras(source: JsonObject): JsonObject {
        val extras = JsonObject()
        for ((name, value) in source.entrySet()) {
            if (name !in EXTRACTED_KEYS) {
                extras.add(name, value.deepCopy())
            }
        }
        return extras
    }

    private fun stringValue(source: JsonObject, names: Set<String>): String? {
        for (name in names) {
            val value = source.getValue(name) ?: continue
            if (value.isJsonNull) continue
            val stringValue = value.asString.takeIf { it.isNotBlank() }
            if (stringValue != null) {
                return stringValue
            }
        }
        return null
    }

    private fun JsonObject.getValue(name: String): JsonElement? {
        return if (has(name)) get(name) else null
    }

    private fun JsonObject.getObject(name: String): JsonObject? {
        val value = getValue(name) ?: return null
        return if (value.isJsonObject) value.asJsonObject else null
    }

    companion object {
        private const val PROVISIONING_ADMIN_EXTRAS_BUNDLE =
            "android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE"

        private val SERVER_URL_KEYS = setOf(
            "android.app.extra.PROVISIONING_SERVER_URL",
            "com.xmdm.BASE_URL",
        )
        private val SECONDARY_SERVER_URL_KEYS = setOf(
            "com.xmdm.SECONDARY_BASE_URL",
        )
        private val ENROLLMENT_TOKEN_KEYS = setOf(
            "com.xmdm.ENROLLMENT_TOKEN",
        )
        private val DEVICE_ID_KEYS = setOf(
            "com.xmdm.DEVICE_ID",
        )
        private val EXTRACTED_KEYS = setOf(
            PROVISIONING_ADMIN_EXTRAS_BUNDLE,
            "android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME",
            "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION",
            "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM",
            "android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED",
            "com.xmdm.BASE_URL",
            "com.xmdm.SECONDARY_BASE_URL",
            "com.xmdm.ENROLLMENT_TOKEN",
            "com.xmdm.DEVICE_ID",
        )
    }
}

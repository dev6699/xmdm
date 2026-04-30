package com.xmdm.launcher.sync

import com.google.gson.Gson
import com.google.gson.JsonArray
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import java.nio.charset.StandardCharsets
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

class ConfigSnapshotVerifier(
    private val gson: Gson = Gson(),
) {
    fun verify(rawJson: String, deviceSecret: String): JsonObject {
        require(deviceSecret.isNotBlank()) { "device secret must not be blank" }

        val snapshot = JsonParser.parseString(rawJson).asJsonObject
        val signature = snapshot.get("signature")?.takeIf { !it.isJsonNull }?.asString
            ?: error("missing config snapshot signature")
        snapshot.addProperty("signature", "")
        val expectedSignature = signCanonical(snapshot, deviceSecret)
        if (!constantTimeEquals(signature, expectedSignature)) {
            error("invalid config snapshot signature")
        }
        snapshot.addProperty("signature", signature)
        return snapshot
    }

    fun sign(rawJsonWithoutSignature: String, deviceSecret: String): String {
        val snapshot = JsonParser.parseString(rawJsonWithoutSignature).asJsonObject
        snapshot.addProperty("signature", "")
        return signCanonical(snapshot, deviceSecret)
    }

    private fun signCanonical(snapshot: JsonObject, deviceSecret: String): String {
        val canonical = canonicalJson(snapshot)
        val mac = Mac.getInstance(HMAC_ALGORITHM)
        mac.init(SecretKeySpec(deviceSecret.toByteArray(StandardCharsets.UTF_8), HMAC_ALGORITHM))
        val signature = mac.doFinal(canonical.toByteArray(StandardCharsets.UTF_8))
        return java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(signature)
    }

    private fun canonicalJson(element: JsonElement): String {
        val out = StringBuilder()
        appendCanonical(out, element)
        return out.toString()
    }

    private fun appendCanonical(out: StringBuilder, element: JsonElement) {
        when {
            element.isJsonNull -> out.append("null")
            element.isJsonPrimitive -> out.append(gson.toJson(element))
            element.isJsonArray -> appendArray(out, element.asJsonArray)
            element.isJsonObject -> appendObject(out, element.asJsonObject)
        }
    }

    private fun appendArray(out: StringBuilder, array: JsonArray) {
        out.append('[')
        array.forEachIndexed { index, element ->
            if (index > 0) {
                out.append(',')
            }
            appendCanonical(out, element)
        }
        out.append(']')
    }

    private fun appendObject(out: StringBuilder, objectValue: JsonObject) {
        out.append('{')
        val entries = orderedEntries(objectValue)
        entries.forEachIndexed { index, entry ->
            if (index > 0) {
                out.append(',')
            }
            out.append(gson.toJson(entry.key))
            out.append(':')
            appendCanonical(out, entry.value)
        }
        out.append('}')
    }

    private fun orderedEntries(objectValue: JsonObject): List<Map.Entry<String, JsonElement>> {
        val entriesByName = objectValue.entrySet().associateBy { it.key }
        val ordered = SNAPSHOT_FIELD_ORDER.mapNotNull { entriesByName[it] }
        if (ordered.isNotEmpty() && ordered.size == objectValue.entrySet().size) {
            return ordered
        }
        return objectValue.entrySet().sortedBy { it.key }
    }

    private fun constantTimeEquals(expected: String, actual: String): Boolean {
        if (expected.length != actual.length) {
            return false
        }
        var result = 0
        for (index in expected.indices) {
            result = result or (expected[index].code xor actual[index].code)
        }
        return result == 0
    }

    companion object {
        private const val HMAC_ALGORITHM = "HmacSHA256"
        private val SNAPSHOT_FIELD_ORDER = listOf(
            "version",
            "runtime",
            "device",
            "policy",
            "apps",
            "files",
            "certificates",
            "signature",
        )
    }
}

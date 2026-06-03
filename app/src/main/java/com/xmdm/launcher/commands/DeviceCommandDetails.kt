package com.xmdm.launcher.commands

internal fun DeviceCommandRecord.commandDetails(vararg values: Pair<String, Any?>): Map<String, Any> {
    val result = linkedMapOf<String, Any>()
    result["commandId"] = id
    result["commandType"] = type
    for ((key, value) in values) {
        if (value != null) {
            result[key] = value
        }
    }
    payload?.let { payloadJson ->
        result["payload"] = payloadJson
    }
    expiresAt?.let { result["expiresAt"] = it }
    return result
}

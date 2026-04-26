package com.xmdm.launcher.commands

import com.google.gson.JsonObject

data class DeviceCommandRecord(
    val id: String,
    val type: String,
    val status: String,
    val payload: JsonObject?,
    val expiresAt: String? = null,
)

data class DeviceCommandPollResponse(
    val commands: List<DeviceCommandRecord>? = null,
)

data class DeviceCommandAckRequest(
    val status: String,
    val message: String? = null,
    val details: Map<String, Any>? = null,
)

data class DeviceCommandExecutionResult(
    val status: String,
    val message: String? = null,
    val details: Map<String, Any>? = null,
)

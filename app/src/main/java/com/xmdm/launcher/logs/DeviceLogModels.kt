package com.xmdm.launcher.logs

import java.util.UUID

data class DeviceLogEntry(
    val id: String = UUID.randomUUID().toString(),
    val observedAt: String,
    val source: String,
    val level: String,
    val message: String,
    val payload: Map<String, Any?>? = null,
)

data class DeviceLogUploadRequest(
    val schemaVersion: Int = 1,
    val observedAt: String,
    val entries: List<DeviceLogEntry>,
)

interface DeviceLogGateway {
    suspend fun upload(
        serverUrl: String,
        deviceId: String,
        deviceSecret: String,
        request: DeviceLogUploadRequest,
    )
}

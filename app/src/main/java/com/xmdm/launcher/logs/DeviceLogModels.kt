package com.xmdm.launcher.logs

data class DeviceLogEntry(
    val observedAt: String,
    val source: String,
    val level: String,
    val message: String,
    val payload: Map<String, Any?>? = null,
)

data class DeviceLogUploadRequest(
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


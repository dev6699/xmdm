package com.xmdm.launcher.enrollment

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState

data class DeviceIdentityPolicy(
    val deviceId: String,
)

data class EnrollmentRequest(
    val enrollmentToken: String,
    val deviceIdentityPolicy: DeviceIdentityPolicy,
    val bootstrapExtras: JsonObject,
)

data class EnrollmentResponse(
    val deviceId: String,
    val deviceSecret: String,
    val status: String,
)

data class EnrollmentResult(
    val identity: DeviceIdentityState,
)

interface EnrollmentGateway {
    suspend fun enroll(serverUrl: String, request: EnrollmentRequest): EnrollmentResponse
}

class EnrollmentCoordinator(
    private val stateStore: AgentStateStore,
    private val gateway: EnrollmentGateway,
) {
    suspend fun enroll(bootstrap: BootstrapState): EnrollmentResult {
        val deviceId = bootstrap.deviceId?.takeIf { it.isNotBlank() }
            ?: error("bootstrap is missing a device id")

        val response = gateway.enroll(
            bootstrap.serverUrl,
            EnrollmentRequest(
                enrollmentToken = bootstrap.enrollmentToken,
                deviceIdentityPolicy = DeviceIdentityPolicy(
                    deviceId = deviceId,
                ),
                bootstrapExtras = JsonParser.parseString(bootstrap.bootstrapExtrasJson).asJsonObject,
            ),
        )

        require(response.status == "enrolled") {
            "unexpected enrollment status: ${response.status}"
        }
        require(response.deviceId == deviceId) {
            "enrollment response device id mismatch"
        }

        val identity = DeviceIdentityState(
            deviceId = response.deviceId,
            deviceSecret = response.deviceSecret,
        )

        stateStore.saveDeviceIdentity(identity)
        return EnrollmentResult(identity = identity)
    }
}

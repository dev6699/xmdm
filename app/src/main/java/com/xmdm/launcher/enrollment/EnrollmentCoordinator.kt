package com.xmdm.launcher.enrollment

import com.google.gson.Gson
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.BootstrapState
import com.xmdm.launcher.state.DeviceIdentityState
import com.xmdm.launcher.state.PolicyCacheState
import com.xmdm.launcher.sync.ConfigSnapshotVerifier
import java.time.Clock

data class DeviceIdentityPolicy(
    val deviceId: String,
    val deviceIdUse: String,
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
    val config: JsonObject,
)

data class EnrollmentResult(
    val identity: DeviceIdentityState,
    val policyCache: PolicyCacheState,
)

interface EnrollmentGateway {
    suspend fun enroll(serverUrl: String, request: EnrollmentRequest): EnrollmentResponse
}

class EnrollmentCoordinator(
    private val stateStore: AgentStateStore,
    private val gateway: EnrollmentGateway,
    private val verifier: ConfigSnapshotVerifier = ConfigSnapshotVerifier(),
    private val gson: Gson = Gson(),
    private val clock: Clock = Clock.systemUTC(),
) {
    suspend fun enroll(bootstrap: BootstrapState): EnrollmentResult {
        val deviceId = bootstrap.deviceId?.takeIf { it.isNotBlank() }
            ?: error("bootstrap is missing a device id")
        val deviceIdUse = bootstrap.deviceIdUse?.takeIf { it.isNotBlank() }
            ?: error("bootstrap is missing a device id use")

        val response = gateway.enroll(
            bootstrap.serverUrl,
            EnrollmentRequest(
                enrollmentToken = bootstrap.enrollmentToken,
                deviceIdentityPolicy = DeviceIdentityPolicy(
                    deviceId = deviceId,
                    deviceIdUse = deviceIdUse,
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
            deviceIdUse = deviceIdUse,
            deviceSecret = response.deviceSecret,
        )
        val snapshotJson = gson.toJson(response.config)
        val verified = verifier.verify(snapshotJson, identity.deviceSecret)
        val version = verified.get("version")?.takeIf { !it.isJsonNull }?.asString?.toLongOrNull()
            ?: error("enrollment config version must be a numeric string")
        val policyCache = PolicyCacheState(
            snapshotJson = snapshotJson,
            version = version,
            lastSyncAtEpochMillis = clock.millis(),
        )

        stateStore.saveDeviceIdentity(identity)
        stateStore.savePolicyCache(policyCache)
        return EnrollmentResult(identity = identity, policyCache = policyCache)
    }
}

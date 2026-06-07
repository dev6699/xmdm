package com.xmdm.launcher.commands

import com.google.gson.JsonParser
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.DeviceIdentityState
import com.xmdm.launcher.state.ManagedAppsState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class CompanionAppLaunchCoordinatorTest {
    @Test
    fun launchesDeclaredPackageWhenSignatureMatches() = runTest {
        val secret = "device-secret"
        val packageName = "com.example.companion"
        val snapshot = signedSnapshot(
            deviceSecret = secret,
            packageName = packageName,
        )
        val host = FakeCompanionAppLaunchHost(
            installedSignatures = mapOf(packageName to setOf("deadbeef")),
            launchablePackages = mutableSetOf(packageName),
        )
        val coordinator = CompanionAppLaunchCoordinator(
            host = host,
            logger = CompanionAppLaunchLogger { },
            launchDispatcher = Dispatchers.Unconfined,
        )
        val command = command(
            """{"packageName":"$packageName","signatureSha256":"deadbeef"}""",
        )

        val result = coordinator.execute(agentState(secret, snapshot), command)

        assertEquals("acked", result.status)
        assertEquals("companion app launch requested", result.message)
        assertEquals(listOf(packageName), host.packageLaunches)
        assertTrue(host.activityLaunches.isEmpty())
    }

    @Test
    fun forwardsBootstrapPayloadToLaunchedPackage() = runTest {
        val secret = "device-secret"
        val packageName = "com.example.companion"
        val snapshot = signedSnapshot(
            deviceSecret = secret,
            packageName = packageName,
        )
        val host = FakeCompanionAppLaunchHost(
            installedSignatures = mapOf(packageName to setOf("deadbeef")),
            launchablePackages = mutableSetOf(packageName),
        )
        val coordinator = CompanionAppLaunchCoordinator(
            host = host,
            logger = CompanionAppLaunchLogger { },
            launchDispatcher = Dispatchers.Unconfined,
        )
        val command = command(
            """{"packageName":"$packageName","signatureSha256":"deadbeef","bootstrapPayload":"base64url:eyJmZWF0dXJlSWQiOiJyZW1vdGUtY29udHJvbCIsInNlc3Npb25JZCI6InNlc3Npb24tMSIsImRldmljZUlkIjoiZGV2aWNlLTEiLCJyZWxheUVuZHBvaW50IjoiaHR0cDovL3JlbGF5IiwiYWRtaW5Ub2tlbiI6ImFkbWluLXRva2VuIiwiZGV2aWNlVG9rZW4iOiJkZXZpY2UtdG9rZW4iLCJleHBpcmVzQXQiOiIyMDMwLTAxLTAxVDAwOjAwOjAwWiJ9"}""",
        )

        val result = coordinator.execute(agentState(secret, snapshot), command)

        assertEquals("acked", result.status)
        assertEquals(listOf(packageName), host.packageLaunches)
        assertEquals(1, host.packageBootstrapPayloads.size)
        val bootstrap = host.packageBootstrapPayloads[0]
        assertTrue(bootstrap.startsWith("base64url:"))
        val decoded = String(java.util.Base64.getUrlDecoder().decode(bootstrap.removePrefix("base64url:")))
        assertTrue(decoded.contains("\"sessionId\":\"session-1\""))
    }

    @Test
    fun launchesDeclaredActivityWhenSignatureMatches() = runTest {
        val secret = "device-secret"
        val packageName = "com.example.companion"
        val activityName = "com.example.companion.LaunchActivity"
        val snapshot = signedSnapshot(
            deviceSecret = secret,
            packageName = packageName,
        )
        val host = FakeCompanionAppLaunchHost(
            installedSignatures = mapOf(packageName to setOf("deadbeef")),
            launchableActivities = mutableSetOf(activityName),
        )
        val coordinator = CompanionAppLaunchCoordinator(
            host = host,
            logger = CompanionAppLaunchLogger { },
            launchDispatcher = Dispatchers.Unconfined,
        )
        val command = command(
            """{"packageName":"$packageName","activityName":"$activityName","signatureSha256":"deadbeef"}""",
        )

        val result = coordinator.execute(agentState(secret, snapshot), command)

        assertEquals("acked", result.status)
        assertEquals(listOf(activityName), host.activityLaunches)
        assertTrue(host.packageLaunches.isEmpty())
    }

    @Test
    fun rejectsMissingPackageDeclaration() = runTest {
        val secret = "device-secret"
        val snapshot = signedSnapshot(
            deviceSecret = secret,
            packageName = "com.example.other",
        )
        val host = FakeCompanionAppLaunchHost(
            installedSignatures = mapOf("com.example.companion" to setOf("deadbeef")),
            launchablePackages = mutableSetOf("com.example.companion"),
        )
        val coordinator = CompanionAppLaunchCoordinator(
            host = host,
            logger = CompanionAppLaunchLogger { },
            launchDispatcher = Dispatchers.Unconfined,
        )
        val command = command(
            """{"packageName":"com.example.companion","signatureSha256":"deadbeef"}""",
        )

        val result = coordinator.execute(agentState(secret, snapshot), command)

        assertEquals("failed", result.status)
        assertEquals("companion package declaration missing", result.message)
        assertTrue(host.packageLaunches.isEmpty())
    }

    @Test
    fun rejectsMissingSignature() = runTest {
        val secret = "device-secret"
        val packageName = "com.example.companion"
        val snapshot = signedSnapshot(
            deviceSecret = secret,
            packageName = packageName,
        )
        val host = FakeCompanionAppLaunchHost(
            installedSignatures = mapOf(packageName to setOf("deadbeef")),
            launchablePackages = mutableSetOf(packageName),
        )
        val coordinator = CompanionAppLaunchCoordinator(
            host = host,
            logger = CompanionAppLaunchLogger { },
            launchDispatcher = Dispatchers.Unconfined,
        )
        val command = command(
            """{"packageName":"$packageName"}""",
        )

        val result = coordinator.execute(agentState(secret, snapshot), command)

        assertEquals("failed", result.status)
        assertEquals("companion app signature is required", result.message)
        assertTrue(host.packageLaunches.isEmpty())
    }

    @Test
    fun acceptsAnySignerDigestFromInstalledApp() = runTest {
        val secret = "device-secret"
        val packageName = "com.example.companion"
        val snapshot = signedSnapshot(
            deviceSecret = secret,
            packageName = packageName,
        )
        val host = FakeCompanionAppLaunchHost(
            installedSignatures = mapOf(packageName to setOf("aaaa", "deadbeef")),
            launchablePackages = mutableSetOf(packageName),
        )
        val coordinator = CompanionAppLaunchCoordinator(
            host = host,
            logger = CompanionAppLaunchLogger { },
            launchDispatcher = Dispatchers.Unconfined,
        )
        val command = command(
            """{"packageName":"$packageName","signatureSha256":"deadbeef"}""",
        )

        val result = coordinator.execute(agentState(secret, snapshot), command)

        assertEquals("acked", result.status)
        assertEquals(listOf(packageName), host.packageLaunches)
    }

    private fun agentState(deviceSecret: String, managedAppsSnapshot: String): AgentState {
        return AgentState(
            identity = DeviceIdentityState(
                deviceId = "device-123",
                deviceSecret = deviceSecret,
            ),
            managedApps = ManagedAppsState(
                snapshotJson = managedAppsSnapshot,
                version = 1,
                lastAppliedAtEpochMillis = 1,
            ),
        )
    }

    private fun command(payloadJson: String): DeviceCommandRecord {
        return DeviceCommandRecord(
            id = "cmd-1",
            type = "launch_companion_app",
            status = "queued",
            payload = JsonParser.parseString(payloadJson).asJsonObject,
            expiresAt = null,
        )
    }

    private fun signedSnapshot(deviceSecret: String, packageName: String): String {
        val raw = JsonParser.parseString(
            """
            {
              "version":"1",
              "device":{"deviceId":"device-123"},
              "policy":{},
              "apps":[
                {
                  "appId":"app-1",
                  "packageName":"$packageName",
                  "name":"Companion",
                  "versionId":"version-1",
                  "versionName":"1.0.0",
                  "versionCode":1,
                  "artifactId":"artifact-1",
                  "checksum":"checksum-1",
                  "downloadPath":"/download/app-1"
                }
              ],
              "files":[],
              "certificates":[],
              "signature":""
            }
            """.trimIndent(),
        ).asJsonObject
        val verifier = com.xmdm.launcher.sync.ConfigSnapshotVerifier()
        val signature = verifier.sign(raw.toString(), deviceSecret)
        raw.addProperty("signature", signature)
        return raw.toString()
    }

    private class FakeCompanionAppLaunchHost(
        private val installedSignatures: Map<String, Set<String>>,
        var launchablePackages: MutableSet<String> = mutableSetOf(),
        var launchableActivities: MutableSet<String> = mutableSetOf(),
    ) : CompanionAppLaunchHost {
        val packageLaunches = mutableListOf<String>()
        val activityLaunches = mutableListOf<String>()

        override fun isPackageInstalled(packageName: String): Boolean {
            return packageName in installedSignatures
        }

        override fun packageSignatureDigests(packageName: String): Set<String>? {
            return installedSignatures[packageName]
        }

        override fun canLaunchPackage(packageName: String): Boolean {
            return packageName in launchablePackages
        }

        override fun canLaunchActivity(packageName: String, activityName: String): Boolean {
            return "$packageName/$activityName" in launchableActivities || activityName in launchableActivities
        }

        override fun launchPackage(packageName: String, bootstrapPayload: String?): Boolean {
            packageLaunches += packageName
            if (!bootstrapPayload.isNullOrBlank()) {
                packageBootstrapPayloads += bootstrapPayload
            }
            return packageName in launchablePackages
        }

        override fun launchActivity(packageName: String, activityName: String, bootstrapPayload: String?): Boolean {
            activityLaunches += activityName
            if (!bootstrapPayload.isNullOrBlank()) {
                activityBootstrapPayloads += bootstrapPayload
            }
            return "$packageName/$activityName" in launchableActivities || activityName in launchableActivities
        }

        val packageBootstrapPayloads = mutableListOf<String>()
        val activityBootstrapPayloads = mutableListOf<String>()
    }
}

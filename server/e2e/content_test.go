package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// ── Test cases ───────────────────────────────────────────────────────────────

// TestManagedAppsAndFiles enrolls a device and verifies that the managed file
// is rendered on-device (with variable substitution) and that the Chrome APK
// is installed via the managed app policy.
func TestManagedAppsAndFiles(t *testing.T) {
	env := newContentTestEnv(t)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	env.requests.waitFor(t, time.Minute, "managed file artifact download", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			strings.Contains(r.path, "/api/v1/devices/") &&
			strings.Contains(r.path, "/managed-files/") &&
			strings.HasSuffix(r.path, "/artifact")
	})
	env.requests.waitFor(t, time.Minute, "managed app artifact download", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			strings.Contains(r.path, "/api/v1/devices/") &&
			strings.Contains(r.path, "/apps/") &&
			strings.HasSuffix(r.path, "/artifact")
	})
	waitForManagedFileOnDevice(t, env.serial, env.deviceID)
	waitForChromeInstalled(t, env.serial)
}

// TestManagedAppsAndFilesRemoval verifies that managed file and app removals in
// a later config snapshot are reflected on the device without re-enrollment.
func TestManagedAppsAndFilesRemoval(t *testing.T) {
	env := newContentTestEnvWithExtras(t, nil)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	env.requests.waitFor(t, time.Minute, "managed file artifact download", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			strings.Contains(r.path, "/api/v1/devices/") &&
			strings.Contains(r.path, "/managed-files/") &&
			strings.HasSuffix(r.path, "/artifact")
	})
	env.requests.waitFor(t, time.Minute, "managed app artifact download", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			strings.Contains(r.path, "/api/v1/devices/") &&
			strings.Contains(r.path, "/apps/") &&
			strings.HasSuffix(r.path, "/artifact")
	})
	waitForManagedFileOnDevice(t, env.serial, env.deviceID)
	waitForChromeInstalled(t, env.serial)

	requestMarker := env.requests.len()
	deleteJSON(t, env.client, env.baseURL+"/api/v1/managed-files/"+env.managedFile.managedFileID)
	deleteJSON(t, env.client, env.baseURL+"/api/v1/apps/"+env.chromeAppID)

	env.requests.waitForAfter(t, requestMarker, time.Minute, "config sync after managed content removal", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/config"
	})
	waitForManagedFileRemovedFromDevice(t, env.serial)
	waitForChromeUninstalled(t, env.serial)
}

// TestCommandMQTT enrolls a device using MQTT transport and verifies that a
// ping command is acknowledged by the device over the MQTT connection.
func TestCommandMQTT(t *testing.T) {
	env := newCommandTestEnv(t)

	env.reverseMQTTPort(t)

	token := env.mustCreateEnrollmentToken(t)
	bootstrapURI := env.mustBuildBootstrapURI(t, token)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForCommandTransportWarmup(t)

	commandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, commandID, "pong")
	env.requests.assertNever(t, "polling command fetch", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			strings.Contains(r.path, "/api/v1/devices/") &&
			strings.HasSuffix(r.path, "/commands")
	})
}

// TestCommandMQTTSyncConfig verifies that a pushed sync_config command causes
// the launcher to fetch the latest config snapshot immediately over MQTT.
func TestCommandMQTTSyncConfig(t *testing.T) {
	env := newCommandTestEnv(t)

	env.reverseMQTTPort(t)

	token := env.mustCreateEnrollmentToken(t)
	bootstrapURI := env.mustBuildBootstrapURI(t, token)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForCommandTransportWarmup(t)

	initialMarker := env.requests.len()
	commandID := env.mustIssueSyncConfigCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, commandID, "config refreshed")
	env.requests.waitForAfter(t, initialMarker, time.Minute, "config snapshot fetch after sync_config command", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/config"
	})
	env.requests.assertNever(t, "polling command fetch", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			strings.Contains(r.path, "/api/v1/devices/") &&
			strings.HasSuffix(r.path, "/commands")
	})
}

// TestCommandPolling enrolls a device using HTTP polling transport and verifies
// that a ping command is acknowledged, and that a command issued after the
// launcher is stopped expires before the device can collect it.
func TestCommandPolling(t *testing.T) {
	env := newPollingCommandTestEnv(t)

	token := env.mustCreateEnrollmentToken(t)
	bootstrapURI := env.mustBuildBootstrapURIWithExtras(t, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)

	commandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.requests.waitFor(t, time.Minute, "HTTP polling command fetch", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/commands"
	})
	env.requests.waitFor(t, time.Minute, "HTTP polling command ack", func(r requestRecord) bool {
		return r.method == http.MethodPost &&
			r.path == "/api/v1/devices/"+env.deviceID+"/commands/"+commandID+"/ack"
	})
	env.waitForCommandAck(t, commandID, "pong")
}

// TestCommandBrokerOutageRecovery verifies that the launcher falls back to HTTP
// polling when MQTT goes away, then resumes MQTT command delivery after the
// broker comes back.
func TestCommandBrokerOutageRecovery(t *testing.T) {
	env := newCommandTestEnv(t)
	ensureMQTTBrokerRunning(t)
	env.reverseMQTTPort(t)

	token := env.mustCreateEnrollmentToken(t)
	bootstrapURI := env.mustBuildBootstrapURIWithExtras(t, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForCommandTransportWarmup(t)

	initialCommandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, initialCommandID, "pong")

	stopMQTTBroker(t)
	time.Sleep(2 * time.Second)

	outageCommandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "HTTP polling command fetch during broker outage", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/commands"
	})
	env.requests.waitFor(t, time.Minute, "HTTP polling command ack during broker outage", func(r requestRecord) bool {
		return r.method == http.MethodPost &&
			r.path == "/api/v1/devices/"+env.deviceID+"/commands/"+outageCommandID+"/ack"
	})
	env.waitForCommandAck(t, outageCommandID, "pong")

	startMQTTBroker(t)
	waitForCommandTransportWarmup(t)

	recoveryMarker := env.requests.len()
	recoveryCommandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands after broker recovery", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, recoveryCommandID, "pong")
	env.requests.assertNeverAfter(t, recoveryMarker, "HTTP polling after broker recovery", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/commands"
	})
}

// TestKioskMode verifies that a kiosk policy snapshot pushes the launcher into
// lock-task mode on a physical device.
func TestKioskMode(t *testing.T) {
	env := newBaseTestEnv(t, false)
	mustCreatePolicy(t, env.client, env.baseURL, `{
		"name":"kiosk-mode",
		"version":1,
		"kioskMode":true,
		"restrictions":{}
	}`)

	token := mustCreateEnrollmentToken(t, env.client, env.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, env.client, env.baseURL, env.launcherChecksum, env.deviceID, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForKioskModeOnDevice(t, env.serial)
}

// TestPackageRules verifies that the launcher suspends packages that are
// blocked by the signed policy snapshot on a physical device.
func TestPackageRules(t *testing.T) {
	env := newPackageRulesTestEnv(t)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForChromeInstalled(t, env.serial)
	waitForPackageSuspendedOnDevice(t, env.serial, chromePackage)
}

// TestPolicySync verifies that admin policy updates are picked up by the
// running launcher without re-enrollment and are reflected in the device state.
func TestPolicySync(t *testing.T) {
	env := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)

	mustRegisterChromeApp(t, env.client, env.baseURL, artifactStore)
	policyResp := mustCreatePolicy(t, env.client, env.baseURL, `{
		"name":"policy-sync",
		"version":1,
		"kioskMode":false,
		"restrictions":{}
	}`)
	policyID, _ := policyResp["id"].(string)
	if policyID == "" {
		t.Fatalf("policy create returned empty id: %#v", policyResp)
	}

	token := mustCreateEnrollmentToken(t, env.client, env.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, env.client, env.baseURL, env.launcherChecksum, env.deviceID, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	env.requests.waitFor(t, time.Minute, "managed app artifact download", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			strings.Contains(r.path, "/api/v1/devices/") &&
			strings.Contains(r.path, "/apps/") &&
			strings.HasSuffix(r.path, "/artifact")
	})
	waitForChromeInstalled(t, env.serial)

	requestMarker := env.requests.len()
	patchJSON(t, env.client, env.baseURL+"/api/v1/policies/"+policyID, `{
		"name":"policy-sync",
		"version":2,
		"kioskMode":false,
		"restrictions":{
			"blockPackages":["com.android.chrome"]
		}
	}`)
	env.requests.waitForAfter(t, requestMarker, time.Minute, "config sync after policy update", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/config"
	})
	waitForPackageSuspendedOnDevice(t, env.serial, chromePackage)
}

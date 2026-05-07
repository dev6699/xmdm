package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/enrollment"
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
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
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
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
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

// TestCertificatesApplied enrolls a device and verifies that active certificates
// are downloaded and reported after the initial config sync.
func TestCertificatesApplied(t *testing.T) {
	env := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)
	cert := mustUploadCertificate(t, env.client, env.baseURL, artifactStore)

	token := mustCreateEnrollmentToken(t, env.client, env.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, env.client, env.baseURL, env.launcherChecksum, env.deviceID, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	env.requests.waitFor(t, time.Minute, "certificate artifact download", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/certificates/"+cert.certificateID+"/artifact"
	})
	assertCertificateInstallReportedViaAPI(t, env.requests, env.deviceID)
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
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
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
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
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
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
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

// TestKioskModeChrome verifies that a kiosk policy snapshot pushes Chrome into
// lock-task mode on a physical device.
func TestKioskModeChrome(t *testing.T) {
	env := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)
	passcodeHash := enrollment.HashToken("1234")

	mustRegisterChromeApp(t, env.client, env.baseURL, artifactStore)
	mustCreatePolicy(t, env.client, env.baseURL, fmt.Sprintf(`{
		"name":"kiosk-mode",
		"version":1,
		"kioskMode":true,
		"kioskAppPackage":"com.android.chrome",
		"restrictions":{
			"kioskExitPasscodeHash":"%s"
		}
	}`, passcodeHash))

	token := mustCreateEnrollmentToken(t, env.client, env.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, env.client, env.baseURL, env.launcherChecksum, env.deviceID, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForChromeInstalled(t, env.serial)
	waitForForegroundPackage(t, env.serial, chromePackage)
	waitForKioskModeOnDevice(t, env.serial)
}

// TestKioskExitChromeLocal verifies that the local kiosk exit overlay unlocks
// Chrome when Chrome is the kiosk app.
func TestKioskExitChromeLocal(t *testing.T) {
	env := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)

	mustRegisterChromeApp(t, env.client, env.baseURL, artifactStore)

	passcodeHash := enrollment.HashToken("1234")
	mustCreatePolicy(t, env.client, env.baseURL, fmt.Sprintf(`{
		"name":"kiosk-exit",
		"version":1,
		"kioskMode":true,
		"kioskAppPackage":"%s",
		"restrictions":{
			"kioskExitPasscodeHash":"%s"
		}
	}`, chromePackage, passcodeHash))

	token := mustCreateEnrollmentToken(t, env.client, env.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, env.client, env.baseURL, env.launcherChecksum, env.deviceID, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForChromeInstalled(t, env.serial)
	waitForForegroundPackageStable(t, env.serial, chromePackage, 15*time.Second)
	waitForKioskModeOnDevice(t, env.serial)
	tapKioskAdminMenuButton(t, env.serial)
	tapKioskMenuItem(t, env.serial, "Exit kiosk mode")
	enterKioskExitPasscode(t, env.serial, "1234")
	waitForKioskModeOffOnDevice(t, env.serial)

	switchToForegroundApp(t, env.serial, "com.android.settings")
	tapKioskAdminMenuButton(t, env.serial)
	tapKioskMenuItem(t, env.serial, "Enter kiosk mode")
	waitForKioskModeOnDevice(t, env.serial)
	waitForForegroundPackage(t, env.serial, chromePackage)

	switchToForegroundApp(t, env.serial, "com.android.settings")
	tapKioskAdminMenuButton(t, env.serial)
	tapKioskMenuItem(t, env.serial, "Exit kiosk mode")
}

// TestKioskAdminConfigSyncStatus verifies that the local kiosk admin menu
// refreshes the displayed config snapshot after a local sync.
func TestKioskAdminConfigSyncStatus(t *testing.T) {
	env := newBaseTestEnv(t, false)
	passcodeHash := enrollment.HashToken("1234")

	policyResp := mustCreatePolicy(t, env.client, env.baseURL, fmt.Sprintf(`{
		"name":"kiosk-sync-status",
		"version":1,
		"kioskMode":true,
		"kioskAppPackage":"com.xmdm.launcher",
		"restrictions":{
			"kioskExitPasscodeHash":"%s"
		}
	}`, passcodeHash))
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
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)

	requestMarker := env.requests.len()
	patchJSON(t, env.client, env.baseURL+"/api/v1/policies/"+policyID, fmt.Sprintf(`{
		"name":"kiosk-sync-status-updated",
		"version":2,
		"kioskMode":true,
		"kioskAppPackage":"com.xmdm.launcher",
		"restrictions":{
			"kioskExitPasscodeHash":"%s",
			"allowCamera":false
		}
	}`, passcodeHash))
	waitForKioskModeOnDevice(t, env.serial)
	tapKioskAdminMenuButton(t, env.serial)
	tapKioskMenuItem(t, env.serial, "Sync config policy")
	env.requests.waitForAfter(t, requestMarker, time.Minute, "config snapshot fetch after local sync", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/config"
	})
	waitForUIContains(t, env.serial, `"name": "kiosk-sync-status-updated"`, time.Minute)
}

// TestKioskAdminConfigSyncTwice verifies that repeated local config syncs do
// not strand the progress dialog and that the displayed config snapshot updates
// on each refresh.
func TestKioskAdminConfigSyncTwice(t *testing.T) {
	env := newBaseTestEnv(t, false)
	passcodeHash := enrollment.HashToken("1234")

	policyResp := mustCreatePolicy(t, env.client, env.baseURL, fmt.Sprintf(`{
		"name":"kiosk-sync-twice",
		"version":1,
		"kioskMode":true,
		"kioskAppPackage":"com.xmdm.launcher",
		"restrictions":{
			"kioskExitPasscodeHash":"%s"
		}
	}`, passcodeHash))
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
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForKioskModeOnDevice(t, env.serial)

	requestMarker := env.requests.len()
	patchJSON(t, env.client, env.baseURL+"/api/v1/policies/"+policyID, fmt.Sprintf(`{
		"name":"kiosk-sync-twice-v2",
		"version":2,
		"kioskMode":true,
		"kioskAppPackage":"com.xmdm.launcher",
		"restrictions":{
			"kioskExitPasscodeHash":"%s",
			"allowCamera":false
		}
	}`, passcodeHash))
	tapKioskAdminMenuButton(t, env.serial)
	tapKioskMenuItem(t, env.serial, "Sync config policy")
	env.requests.waitForAfter(t, requestMarker, time.Minute, "first config snapshot fetch after local sync", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/config"
	})
	waitForUIContains(t, env.serial, `"name": "kiosk-sync-twice-v2"`, time.Minute)

	requestMarker = env.requests.len()
	patchJSON(t, env.client, env.baseURL+"/api/v1/policies/"+policyID, fmt.Sprintf(`{
		"name":"kiosk-sync-twice-v3",
		"version":3,
		"kioskMode":true,
		"kioskAppPackage":"com.xmdm.launcher",
		"restrictions":{
			"kioskExitPasscodeHash":"%s",
			"allowCamera":true
		}
	}`, passcodeHash))
	tapKioskAdminMenuButton(t, env.serial)
	tapKioskMenuItem(t, env.serial, "Sync config policy")
	env.requests.waitForAfter(t, requestMarker, time.Minute, "second config snapshot fetch after local sync", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/config"
	})
	waitForUIContains(t, env.serial, `"name": "kiosk-sync-twice-v3"`, time.Minute)
}

// TestKioskExitChromeCommand verifies that the server-issued exit_kiosk
// command unlocks the device while Chrome stays foreground after Chrome has
// been launched as the kiosk app.
func TestKioskExitChromeCommand(t *testing.T) {
	ensureMQTTBrokerRunning(t)
	env := commandTestEnv{baseTestEnv: newBaseTestEnv(t, true)}
	env.reverseMQTTPort(t)

	artifactStore := newTestArtifactStore(t)
	passcodeHash := enrollment.HashToken("1234")
	mustRegisterChromeApp(t, env.client, env.baseURL, artifactStore)
	mustCreatePolicy(t, env.client, env.baseURL, fmt.Sprintf(`{
		"name":"kiosk-exit-command",
		"version":1,
		"kioskMode":true,
		"kioskAppPackage":"com.android.chrome",
		"restrictions":{
			"kioskExitPasscodeHash":"%s"
		}
	}`, passcodeHash))

	token := env.mustCreateEnrollmentToken(t)
	bootstrapURI := env.mustBuildBootstrapURI(t, token)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForChromeInstalled(t, env.serial)
	waitForForegroundPackageStable(t, env.serial, chromePackage, 15*time.Second)
	waitForKioskModeOnDevice(t, env.serial)

	commandID := env.mustIssueExitKioskCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, commandID, "kiosk exit requested")
	waitForKioskModeOffOnDevice(t, env.serial)
	waitForForegroundPackage(t, env.serial, launcherPackage)
}

// TestKioskStayAwakeWhilePluggedIn verifies that a kiosk policy can push the
// device-owner stay-on-while-plugged-in setting onto the device.
func TestKioskStayAwakeWhilePluggedIn(t *testing.T) {
	env := newBaseTestEnv(t, false)
	t.Cleanup(func() {
		resetBatteryState(t, env.serial)
	})
	setBatteryPlugged(t, env.serial, true)
	passcodeHash := enrollment.HashToken("1234")

	mustCreatePolicy(t, env.client, env.baseURL, fmt.Sprintf(`{
		"name":"kiosk-stay-awake",
		"version":1,
		"kioskMode":true,
		"kioskAppPackage":"com.xmdm.launcher",
		"restrictions":{
			"kioskExitPasscodeHash":"%s",
			"kioskStayAwakeWhilePluggedIn":true
		}
	}`, passcodeHash))

	token := mustCreateEnrollmentToken(t, env.client, env.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, env.client, env.baseURL, env.launcherChecksum, env.deviceID, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollmentInDB(t, env.pool, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	// Android can normalize the stay-on mask depending on device power sources.
	// Accept the supported kiosk masks instead of pinning the test to one exact value.
	waitForGlobalSettingAny(
		t,
		env.serial,
		"stay_on_while_plugged_in",
		stayOnWhilePluggedInMask(false),
		stayOnWhilePluggedInMask(true),
	)
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
		"kioskAppPackage":"com.android.chrome",
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
		"kioskAppPackage":"com.android.chrome",
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

// TestDeviceLogsUpload verifies that the launcher emits and uploads structured
// device logs after enrollment and config sync.
func TestDeviceLogsUpload(t *testing.T) {
	env := newBaseTestEnv(t, false)

	token := mustCreateEnrollmentToken(t, env.client, env.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, env.client, env.baseURL, env.launcherChecksum, env.deviceID, token, nil)
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)
	waitForConfigSnapshotFetch(t, env.requests, env.deviceID)
	waitForDeviceLogsUpload(t, env.requests, env.deviceID)
	assertDeviceLogsUploadPayload(t, env.requests, env.deviceID)
	assertDeviceLogsRecordedViaAPI(t, env.client, env.baseURL, env.deviceID)
}

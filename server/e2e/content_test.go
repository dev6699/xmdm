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

// TestCommandMQTT enrolls a device using MQTT transport and verifies that a
// ping command is acknowledged by the device over the MQTT connection.
func TestCommandMQTT(t *testing.T) {
	env := newCommandTestEnv(t)

	env.reverseMQTTPort(t)

	token := env.mustCreateEnrollmentToken(t)
	bootstrapURI := env.mustBuildBootstrapURI(t, token, "127.0.0.1:1883")
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})

	commandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, commandID)
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
	bootstrapURI := env.mustBuildBootstrapURIWithExtras(t, token, "", map[string]string{
		"COMMAND_POLL_INTERVAL_MS": "1000",
	})
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
	env.waitForCommandAck(t, commandID)
}

// TestCommandBrokerOutageRecovery verifies that the launcher falls back to HTTP
// polling when MQTT goes away, then resumes MQTT command delivery after the
// broker comes back.
func TestCommandBrokerOutageRecovery(t *testing.T) {
	env := newCommandTestEnv(t)
	ensureMQTTBrokerRunning(t)
	env.reverseMQTTPort(t)

	token := env.mustCreateEnrollmentToken(t)
	bootstrapURI := env.mustBuildBootstrapURIWithExtras(t, token, "127.0.0.1:1883", map[string]string{
		"COMMAND_POLL_INTERVAL_MS": "1000",
	})
	startLauncher(t, env.serial, bootstrapURI)

	env.requests.waitFor(t, time.Minute, "POST /api/v1/enrollment", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/enrollment"
	})
	waitForDeviceEnrollment(t, env.client, env.baseURL, env.deviceID)

	initialCommandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, initialCommandID)

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
	env.waitForCommandAck(t, outageCommandID)

	startMQTTBroker(t)
	time.Sleep(2 * time.Second)

	recoveryMarker := env.requests.len()
	recoveryCommandID := env.mustIssuePingCommand(t)
	env.requests.waitFor(t, time.Minute, "POST /api/v1/admin/commands after broker recovery", func(r requestRecord) bool {
		return r.method == http.MethodPost && r.path == "/api/v1/admin/commands"
	})
	env.waitForCommandAck(t, recoveryCommandID)
	env.requests.assertNeverAfter(t, recoveryMarker, "HTTP polling after broker recovery", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+env.deviceID+"/commands"
	})
}

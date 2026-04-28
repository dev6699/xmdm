package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "xmdm/server/internal/api/v1"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/plugins"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	adminUsername     = "admin"
	adminPassword     = "secret"
	chromePackage     = "com.android.chrome"
	chromeName        = "Chrome"
	chromeVersionStr  = "138.0.7204.179"
	chromeVersionCode = 720417920
	launcherPackage   = "com.xmdm.launcher"
)

// baseTestEnv holds the infrastructure shared by all three tests.
type baseTestEnv struct {
	pool             *pgxpool.Pool
	client           *http.Client
	baseURL          string
	serial           string
	deviceID         string
	launcherAPKPath  string
	launcherChecksum string
	requests         *requestRecorder
}

// newBaseTestEnv sets up the DB, HTTP server, ADB port reverse, launcher reset,
// and a fresh device ID. It is the common preamble for both content and command tests.
func newBaseTestEnv(t *testing.T, enableMQTT bool) baseTestEnv {
	t.Helper()

	serial := strings.TrimSpace(os.Getenv("XMDM_ADB_SERIAL"))
	if serial == "" {
		t.Skip("XMDM_ADB_SERIAL must be set for e2e tests")
	}
	if _, err := exec.LookPath("adb"); err != nil {
		t.Skipf("adb not available: %v", err)
	}

	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := newFrozenAuthService(t)
	auditStore := auditpg.NewDBStore(pool)
	artifactStore := newTestArtifactStore(t)
	handler := v1.NewMux(svc, testDeps(pool, auditStore, plugins.Disabled(), artifactStore, enableMQTT))

	launcherAPKPath := defaultLauncherAPKPath()
	launcherAPK := mustReadFile(t, "launcher apk", launcherAPKPath)
	launcherChecksum := checksum.SHA256Base64URL(launcherAPK)

	requests := newRequestRecorder()
	server := buildTestServer(t, handler, launcherAPKPath, requests)
	t.Cleanup(func() { server.Close() })

	baseURL := server.URL
	client := newHTTPClient(t)
	login(client, t, baseURL, adminUsername, adminPassword)

	reverseADBPort(t, serial, baseURL)
	t.Cleanup(func() { _ = removeADBPortReverse(serial, baseURL) })

	resetADBLauncherState(t, serial, launcherAPKPath)
	resetDeviceEnrollmentState(t, pool)

	return baseTestEnv{
		pool:             pool,
		client:           client,
		baseURL:          baseURL,
		serial:           serial,
		deviceID:         "e2e-" + uuid.NewString(),
		launcherAPKPath:  launcherAPKPath,
		launcherChecksum: launcherChecksum,
		requests:         requests,
	}
}

// contentTestEnv extends baseTestEnv with the managed-file and Chrome app
// fixtures required by TestManagedAppsAndFiles.
type contentTestEnv struct {
	baseTestEnv
	managedFile managedFileFixture
}

// packageRulesTestEnv extends baseTestEnv with the managed app and policy
// fixtures required by TestPackageRules.
type packageRulesTestEnv struct {
	baseTestEnv
}

// newContentTestEnv builds a baseTestEnv, uploads the managed file and Chrome
// app fixtures, then starts the launcher so device assertions can begin
// immediately after this call returns.
func newContentTestEnv(t *testing.T) contentTestEnv {
	t.Helper()

	base := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)

	mf := mustUploadManagedFile(t, base.client, base.baseURL, artifactStore)
	mustRegisterChromeApp(t, base.client, base.baseURL, artifactStore)

	token := mustCreateEnrollmentToken(t, base.client, base.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, base.client, base.baseURL, base.launcherChecksum, base.deviceID, token, "", nil)
	startLauncher(t, base.serial, bootstrapURI)

	return contentTestEnv{
		baseTestEnv: base,
		managedFile: mf,
	}
}

// newPackageRulesTestEnv builds a baseTestEnv, uploads Chrome, creates a
// blocking policy, then starts the launcher so device assertions can begin
// immediately after this call returns.
func newPackageRulesTestEnv(t *testing.T) packageRulesTestEnv {
	t.Helper()

	base := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)

	mustRegisterChromeApp(t, base.client, base.baseURL, artifactStore)
	mustCreatePolicy(t, base.client, base.baseURL, `{
		"name":"package-rules",
		"version":1,
		"kioskMode":false,
		"restrictions":{
			"blockPackages":["com.android.chrome"]
		}
	}`)

	token := mustCreateEnrollmentToken(t, base.client, base.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, base.client, base.baseURL, base.launcherChecksum, base.deviceID, token, "", nil)
	startLauncher(t, base.serial, bootstrapURI)

	return packageRulesTestEnv{baseTestEnv: base}
}

// commandTestEnv extends baseTestEnv with helpers for issuing and inspecting
// device commands. It carries no managed-file or app fixtures — enrollment
// and command delivery are its sole concern.
type commandTestEnv struct {
	baseTestEnv
}

// newCommandTestEnv builds a baseTestEnv ready for command tests.
// The launcher is NOT started here; each test chooses its own transport
// (MQTT or polling) before calling startLauncher.
func newCommandTestEnv(t *testing.T) commandTestEnv {
	t.Helper()
	return commandTestEnv{baseTestEnv: newBaseTestEnv(t, true)}
}

// newPollingCommandTestEnv builds a baseTestEnv for HTTP polling command tests.
// MQTT is disabled so polling tests exercise the HTTP-only transport path.
func newPollingCommandTestEnv(t *testing.T) commandTestEnv {
	t.Helper()
	return commandTestEnv{baseTestEnv: newBaseTestEnv(t, false)}
}

// ensureMQTTBrokerRunning makes sure the local broker container is up before a
// device-backed command transport test starts.
func ensureMQTTBrokerRunning(t *testing.T) {
	t.Helper()
	runDockerCompose(t, "up", "-d", "mqtt")
}

// stopMQTTBroker stops the local MQTT broker container so launcher traffic
// must fall back to HTTP polling.
func stopMQTTBroker(t *testing.T) {
	t.Helper()
	runDockerCompose(t, "stop", "mqtt")
}

// startMQTTBroker starts the local MQTT broker container again after an outage.
func startMQTTBroker(t *testing.T) {
	t.Helper()
	runDockerCompose(t, "start", "mqtt")
}

func mustCreatePolicy(t *testing.T, client *http.Client, baseURL, body string) map[string]any {
	t.Helper()
	return postJSON(t, client, baseURL+"/api/v1/policies", body)
}

// ── commandTestEnv methods ───────────────────────────────────────────────────

func (e *commandTestEnv) reverseMQTTPort(t *testing.T) {
	t.Helper()
	if _, err := adb(e.serial, "reverse", "tcp:1883", "tcp:1883"); err != nil {
		t.Fatalf("adb reverse mqtt: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adb(e.serial, "reverse", "--remove", "tcp:1883", "tcp:1883")
	})
}

func (e *commandTestEnv) mustCreateEnrollmentToken(t *testing.T) string {
	t.Helper()
	return mustCreateEnrollmentToken(t, e.client, e.baseURL)
}

func (e *commandTestEnv) mustBuildBootstrapURI(t *testing.T, token, mqttAddress string) string {
	t.Helper()
	return mustBuildBootstrapURI(t, e.client, e.baseURL, e.launcherChecksum, e.deviceID, token, mqttAddress, nil)
}

func (e *commandTestEnv) mustBuildBootstrapURIWithExtras(t *testing.T, token, mqttAddress string, extraBootstrapExtras map[string]string) string {
	t.Helper()
	return mustBuildBootstrapURI(t, e.client, e.baseURL, e.launcherChecksum, e.deviceID, token, mqttAddress, extraBootstrapExtras)
}

func (e *commandTestEnv) mustIssuePingCommand(t *testing.T) string {
	t.Helper()

	resp := postJSON(t, e.client, e.baseURL+"/api/v1/admin/commands", fmt.Sprintf(`{
		"type":"ping",
		"target":{
			"type":"device",
			"deviceId":"%s"
		}
	}`, e.deviceID))

	commands, _ := resp["commands"].([]any)
	if len(commands) != 1 {
		t.Fatalf("expected one command row, got %#v", resp["commands"])
	}
	cmd, _ := commands[0].(map[string]any)
	id, _ := cmd["id"].(string)
	if id == "" {
		t.Fatalf("command response did not include an id: %#v", cmd)
	}
	if cmd["type"] != "ping" {
		t.Fatalf("unexpected command type: %#v", cmd["type"])
	}
	return id
}

func (e *commandTestEnv) waitForCommandAck(t *testing.T, commandID string) {
	t.Helper()
	waitForCondition(t, time.Minute, "command ack to reach the server",
		func() string { return commandStatusSnapshot(t, e.pool, commandID) },
		func() (bool, error) {
			var status string
			var resultJSON []byte
			if err := e.pool.QueryRow(context.Background(),
				`SELECT status, result_json FROM commands WHERE id = $1`, commandID,
			).Scan(&status, &resultJSON); err != nil {
				return false, nil
			}
			if status != "acked" {
				return false, nil
			}
			var result map[string]any
			if err := json.Unmarshal(resultJSON, &result); err != nil {
				return false, nil
			}
			return result["message"] == "pong", nil
		},
	)
}

// ── Infrastructure helpers ───────────────────────────────────────────────────

// newFrozenAuthService returns an auth.Service with a clock fixed at the moment of the call.
func newFrozenAuthService(t *testing.T) *auth.Service {
	t.Helper()
	now := time.Now()
	svc := auth.NewService(adminUsername, adminPassword, time.Minute)
	svc.SetNow(func() time.Time { return now })
	return svc
}

// buildTestServer creates an httptest.Server that serves the launcher APK alongside the API handler.
func buildTestServer(t *testing.T, handler *http.ServeMux, launcherAPKPath string, requests *requestRecorder) *httptest.Server {
	t.Helper()
	recordingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			requests.record(r.Method, r.URL.Path)
		}
		handler.ServeHTTP(w, r)
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/launcher.apk", func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			requests.record(r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		http.ServeFile(w, r, launcherAPKPath)
	})
	mux.Handle("/", recordingHandler)
	return httptest.NewServer(mux)
}

// defaultLauncherAPKPath returns the conventional debug build output path for the launcher APK.
func defaultLauncherAPKPath() string {
	return filepath.Join("..", "..", "app", "build", "outputs", "apk", "debug", "xmdm-agent-debug.apk")
}

// mustReadFile reads a file and fails the test if it cannot be read.
func mustReadFile(t *testing.T, label, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", label, err)
	}
	return data
}

type requestRecord struct {
	method string
	path   string
}

type requestRecorder struct {
	mu       sync.Mutex
	requests []requestRecord
}

func newRequestRecorder() *requestRecorder {
	return &requestRecorder{}
}

func (r *requestRecorder) record(method, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, requestRecord{method: method, path: path})
}

func (r *requestRecorder) waitFor(t *testing.T, timeout time.Duration, description string, match func(requestRecord) bool) {
	t.Helper()
	waitForCondition(t, timeout, description, func() string {
		r.mu.Lock()
		defer r.mu.Unlock()
		parts := make([]string, 0, len(r.requests))
		for _, req := range r.requests {
			parts = append(parts, req.method+" "+req.path)
		}
		return strings.Join(parts, " | ")
	}, func() (bool, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, req := range r.requests {
			if match(req) {
				return true, nil
			}
		}
		return false, nil
	})
}

func (r *requestRecorder) assertNever(t *testing.T, description string, match func(requestRecord) bool) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.requests {
		if match(req) {
			t.Fatalf("unexpected %s request: %s %s", description, req.method, req.path)
		}
	}
}

func (r *requestRecorder) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *requestRecorder) assertNeverAfter(t *testing.T, start int, description string, match func(requestRecord) bool) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if start < 0 {
		start = 0
	}
	if start > len(r.requests) {
		start = len(r.requests)
	}
	for _, req := range r.requests[start:] {
		if match(req) {
			t.Fatalf("unexpected %s request after index %d: %s %s", description, start, req.method, req.path)
		}
	}
}

func runDockerCompose(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = filepath.Join("..", "..", "infra")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

// resetDeviceEnrollmentState deletes all devices, telemetry rows, and enrollment tokens.
func resetDeviceEnrollmentState(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`DELETE FROM device_telemetry; DELETE FROM enrollment_tokens; DELETE FROM devices;`,
	); err != nil {
		t.Fatalf("reset enrollment state: %v", err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM devices`).Scan(&count); err != nil {
		t.Fatalf("count devices after reset: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 devices after reset, got %d", count)
	}
}

// mustCreateEnrollmentToken creates a short-lived enrollment token and returns it.
func mustCreateEnrollmentToken(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	resp := postJSON(t, client, baseURL+"/api/v1/enrollment/tokens", `{"ttlSeconds":3600}`)
	token, _ := resp["token"].(string)
	if token == "" {
		t.Fatalf("enrollment token response did not include token: %#v", resp)
	}
	return token
}

// mustBuildBootstrapURI generates and encodes the QR bootstrap payload for the device.
// Pass mqttAddress="" to use HTTP polling.
func mustBuildBootstrapURI(t *testing.T, client *http.Client, baseURL, launcherChecksum, deviceID, token, mqttAddress string, extraBootstrapExtras map[string]string) string {
	t.Helper()
	extras := map[string]string{"CUSTOMER": "Acme"}
	for key, value := range extraBootstrapExtras {
		extras[key] = value
	}
	extrasJSON, err := json.Marshal(extras)
	if err != nil {
		t.Fatalf("marshal bootstrap extras: %v", err)
	}
	if mqttAddress != "" {
		var merged map[string]any
		if err := json.Unmarshal(extrasJSON, &merged); err != nil {
			t.Fatalf("unmarshal bootstrap extras: %v", err)
		}
		merged["MQTT_ADDRESS"] = mqttAddress
		extrasJSON, err = json.Marshal(merged)
		if err != nil {
			t.Fatalf("marshal bootstrap extras with mqtt: %v", err)
		}
	}
	qrJSON := postJSON(t, client, baseURL+"/api/v1/enrollment/qr/json", fmt.Sprintf(`{
		"serverUrl":"%s",
		"serverProject":"rest",
		"enrollmentToken":"%s",
		"deviceAdminPackageDownloadLocation":"%s/launcher.apk",
		"deviceAdminPackageChecksum":"%s",
		"deviceIdentityPolicy":{
			"deviceId":"%s",
			"deviceIdUse":"serial"
		},
		"bootstrapExtras":%s
	}`, baseURL, token, baseURL, launcherChecksum, deviceID, string(extrasJSON)))
	return encodeBootstrapURI(t, qrJSON)
}

// ── Content fixtures ─────────────────────────────────────────────────────────

// managedFileFixture holds the uploaded managed file's server-side identifiers.
type managedFileFixture struct {
	sourceID string
	content  []byte
}

// mustUploadManagedFile uploads the managed file source artifact, registers it
// as a managed file with variable substitution enabled, and schedules cleanup.
func mustUploadManagedFile(t *testing.T, client *http.Client, baseURL string, artifactStore interface {
	Delete(context.Context, string) error
}) managedFileFixture {
	t.Helper()

	content := []byte("managed-file-on-device DEVICE_NUMBER CUSTOMER")
	cs := checksum.SHA256Base64URL(content)
	storageKey := "artifacts/content-e2e/" + uuid.NewString() + "/managed-file.txt"

	resp := postMultipartFile(t, client, baseURL+"/api/v1/files", map[string]string{
		"name":       "managed-file.txt",
		"storageKey": storageKey,
		"checksum":   cs,
		"sizeBytes":  fmt.Sprintf("%d", len(content)),
		"mimeType":   "text/plain",
	}, "file", "managed-file.txt", content)

	sourceID, _ := resp["id"].(string)
	if sourceID == "" {
		t.Fatalf("managed file source create returned empty id: %#v", resp)
	}
	t.Cleanup(func() { _ = artifactStore.Delete(context.Background(), storageKey) })

	mf := postJSON(t, client, baseURL+"/api/v1/managed-files", `{
		"fileId":"`+sourceID+`",
		"path":"adb-managed-file.txt",
		"replaceVariables":true
	}`)
	if mf["path"] != "adb-managed-file.txt" {
		t.Fatalf("managed file create returned unexpected path: %v", mf["path"])
	}
	if mf["replaceVariables"] != true {
		t.Fatalf("managed file create returned unexpected replaceVariables: %v", mf["replaceVariables"])
	}

	return managedFileFixture{sourceID: sourceID, content: content}
}

// mustRegisterChromeApp uploads the Chrome APK artifact and publishes it as a managed app version.
func mustRegisterChromeApp(t *testing.T, client *http.Client, baseURL string, artifactStore interface {
	Delete(context.Context, string) error
}) {
	t.Helper()

	apkPath := filepath.Join("..", "..", "artifacts", "chrome.apk")
	apk := mustReadFile(t, "chrome apk", apkPath)
	cs := checksum.SHA256Base64URL(apk)
	storageKey := "artifacts/content-e2e/" + uuid.NewString() + "/chrome.apk"

	fileResp := postMultipartFile(t, client, baseURL+"/api/v1/files", map[string]string{
		"name":       "chrome.apk",
		"storageKey": storageKey,
		"checksum":   cs,
		"sizeBytes":  fmt.Sprintf("%d", len(apk)),
		"mimeType":   "application/vnd.android.package-archive",
	}, "file", "chrome.apk", apk)

	artifact, _ := fileResp["artifact"].(map[string]any)
	artifactID, _ := artifact["id"].(string)
	if artifactID == "" {
		t.Fatalf("chrome file create returned empty artifact id: %#v", fileResp)
	}
	t.Cleanup(func() { _ = artifactStore.Delete(context.Background(), storageKey) })

	appResp := postJSON(t, client, baseURL+"/api/v1/apps", fmt.Sprintf(`{
		"packageName":"%s",
		"name":"%s"
	}`, chromePackage, chromeName))
	appID, _ := appResp["id"].(string)
	if appID == "" {
		t.Fatalf("chrome app create returned empty id: %#v", appResp)
	}

	versionResp := postJSON(t, client, baseURL+"/api/v1/apps/"+appID+"/versions", fmt.Sprintf(`{
		"versionName":"%s",
		"versionCode":%d,
		"artifactId":"%s",
		"checksum":"%s",
		"publish":true
	}`, chromeVersionStr, chromeVersionCode, artifactID, cs))
	if versionResp["status"] != "published" {
		t.Fatalf("chrome version create returned unexpected status: %v", versionResp["status"])
	}
}

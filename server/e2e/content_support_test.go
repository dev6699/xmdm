package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "xmdm/server/internal/api/v1"
	apps "xmdm/server/internal/apps"
	appspg "xmdm/server/internal/apps/postgres"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/plugins"
	policy "xmdm/server/internal/policy"
	policypg "xmdm/server/internal/policy/postgres"

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
	policyID         string
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
	handler := v1.NewMux(svc, testDeps(t, pool, auditStore, plugins.Disabled(), artifactStore, enableMQTT))

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
	t.Cleanup(func() {
		if err := removeADBPortReverse(serial, baseURL); err != nil {
			t.Logf("removeADBPortReverse: %v", err)
		}
	})

	resetADBLauncherState(t, serial, launcherAPKPath)
	resetDeviceEnrollmentState(t, pool)

	deviceRowID := uuid.NewString()
	policyID := seedDefaultDevicePolicy(t, pool)
	seedPendingDevice(t, pool, deviceRowID, policyID)

	return baseTestEnv{
		pool:             pool,
		client:           client,
		baseURL:          baseURL,
		serial:           serial,
		deviceID:         deviceRowID,
		policyID:         policyID,
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
	chromeAppID string
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
	return newContentTestEnvWithExtras(t, nil)
}

// newContentTestEnvWithExtras builds the same content fixture set with extra
// bootstrap values.
func newContentTestEnvWithExtras(t *testing.T, extraBootstrapExtras map[string]string) contentTestEnv {
	t.Helper()

	base := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)

	mf := mustUploadManagedFile(t, base.client, base.baseURL, artifactStore)
	chromeAppID := mustRegisterChromeApp(t, base.pool, base.client, base.baseURL, artifactStore)
	seedPolicyManagedFile(t, base.pool, base.policyID, mf.managedFileID)
	seedPolicyApp(t, base.pool, base.policyID, chromeAppID)

	token := mustCreateEnrollmentToken(t, base.client, base.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, base.client, base.baseURL, base.launcherChecksum, base.deviceID, token, extraBootstrapExtras)
	startLauncher(t, base.serial, bootstrapURI)

	return contentTestEnv{
		baseTestEnv: base,
		managedFile: mf,
		chromeAppID: chromeAppID,
	}
}

// mustReadLauncherAPKFixture builds a launcher APK variant with the requested
// version metadata and returns its bytes. The default launcher APK output is
// restored at test cleanup time.
func mustReadLauncherAPKFixture(t *testing.T, versionCode int64, versionName string) []byte {
	t.Helper()
	defaultPath := defaultLauncherAPKPath()
	original := mustReadFile(t, "launcher apk", defaultPath)
	t.Cleanup(func() {
		if err := os.WriteFile(defaultPath, original, 0o644); err != nil {
			t.Logf("restore launcher apk: %v", err)
		}
	})

	cmd := exec.Command(
		"./gradlew",
		"--no-daemon",
		"assembleDebug",
		fmt.Sprintf("-Pxmdm.versionCode=%d", versionCode),
		"-Pxmdm.versionName="+versionName,
		"-Pxmdm.testOnly=false",
	)
	cmd.Dir = filepath.Join("..", "..", "app")
	cmd.Env = append(os.Environ(), "GRADLE_USER_HOME="+filepath.Join(os.TempDir(), "xmdm-gradle"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build launcher apk version %d (%s): %v\n%s", versionCode, versionName, err, strings.TrimSpace(string(output)))
	}
	return mustReadFile(t, "launcher apk", defaultPath)
}

// newPackageRulesTestEnv builds a baseTestEnv, uploads Chrome, creates a
// blocking policy, then starts the launcher so device assertions can begin
// immediately after this call returns.
func newPackageRulesTestEnv(t *testing.T) packageRulesTestEnv {
	t.Helper()

	base := newBaseTestEnv(t, false)
	artifactStore := newTestArtifactStore(t)

	chromeAppID := mustRegisterChromeApp(t, base.pool, base.client, base.baseURL, artifactStore)
	policyResp := mustCreatePolicy(t, base.pool, `{
		"name":"package-rules",
		"version":1,
		"kioskMode":false,
		"restrictions":{
			"blockPackages":["com.android.chrome"]
		}
	}`)
	policyID, _ := policyResp["id"].(string)
	if policyID == "" {
		t.Fatalf("policy create returned empty id: %#v", policyResp)
	}
	seedPolicyApp(t, base.pool, policyID, chromeAppID)
	updateDevicePolicy(t, base.pool, base.deviceID, policyID)

	token := mustCreateEnrollmentToken(t, base.client, base.baseURL)
	bootstrapURI := mustBuildBootstrapURI(t, base.client, base.baseURL, base.launcherChecksum, base.deviceID, token, nil)
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

func mustCreatePolicy(t *testing.T, pool *pgxpool.Pool, body string) map[string]any {
	t.Helper()
	var req policy.PolicyUpsert
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode policy payload: %v", err)
	}
	created, err := policypg.New(pool).CreatePolicy(context.Background(), bootstrap.SeedTenantID, req)
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	return map[string]any{
		"id":              created.ID,
		"name":            created.Name,
		"version":         created.Version,
		"kioskMode":       created.KioskMode,
		"kioskAppPackage": created.KioskAppPackage,
		"restrictions":    created.Restrictions,
		"status":          created.Status,
	}
}

func mustUpdatePolicy(t *testing.T, pool *pgxpool.Pool, policyID, body string) map[string]any {
	t.Helper()
	var req policy.PolicyUpsert
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode policy payload: %v", err)
	}
	updated, err := policypg.New(pool).UpdatePolicy(context.Background(), bootstrap.SeedTenantID, policyID, req)
	if err != nil {
		t.Fatalf("update policy: %v", err)
	}
	return map[string]any{
		"id":              updated.ID,
		"name":            updated.Name,
		"version":         updated.Version,
		"kioskMode":       updated.KioskMode,
		"kioskAppPackage": updated.KioskAppPackage,
		"restrictions":    updated.Restrictions,
		"status":          updated.Status,
	}
}

// ── commandTestEnv methods ───────────────────────────────────────────────────

func (e *commandTestEnv) reverseMQTTPort(t *testing.T) {
	t.Helper()
	if _, err := adb(e.serial, "reverse", "tcp:1883", "tcp:1883"); err != nil {
		t.Fatalf("adb reverse mqtt: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adb(e.serial, "reverse", "--remove", "tcp:1883"); err != nil {
			t.Fatalf("adb reverse mqtt: %v", err)
		}
	})
}

func (e *commandTestEnv) mustCreateEnrollmentToken(t *testing.T) string {
	t.Helper()
	return mustCreateEnrollmentToken(t, e.client, e.baseURL)
}

func (e *commandTestEnv) mustBuildBootstrapURI(t *testing.T, token string) string {
	t.Helper()
	return mustBuildBootstrapURI(t, e.client, e.baseURL, e.launcherChecksum, e.deviceID, token, nil)
}

func (e *commandTestEnv) mustBuildBootstrapURIWithExtras(t *testing.T, token string, extraBootstrapExtras map[string]string) string {
	t.Helper()
	return mustBuildBootstrapURI(t, e.client, e.baseURL, e.launcherChecksum, e.deviceID, token, extraBootstrapExtras)
}

func (e *commandTestEnv) mustIssuePingCommand(t *testing.T) string {
	t.Helper()
	return e.mustIssueDashboardCommand(t, "ping", "")
}

func (e *commandTestEnv) mustIssueSyncConfigCommand(t *testing.T) string {
	t.Helper()
	return e.mustIssueDashboardCommand(t, "sync_config", "")
}

func (e *commandTestEnv) mustIssueExitKioskCommand(t *testing.T) string {
	t.Helper()
	return e.mustIssueDashboardCommand(t, "exit_kiosk", `{"packageName":"com.android.chrome"}`)
}

func (e *commandTestEnv) mustIssueDashboardCommand(t *testing.T, commandType, payload string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, e.baseURL+"/admin/commands", nil)
	if err != nil {
		t.Fatalf("build command page request: %v", err)
	}
	res, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("load command page: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected command page, got %d", res.StatusCode)
	}

	meReq, err := http.NewRequest(http.MethodGet, e.baseURL+"/admin/me", nil)
	if err != nil {
		t.Fatalf("build csrf request: %v", err)
	}
	meRes, err := e.client.Do(meReq)
	if err != nil {
		t.Fatalf("load csrf token: %v", err)
	}
	defer meRes.Body.Close()
	if meRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meRes.Body)
		t.Fatalf("expected admin me response, got %d: %s", meRes.StatusCode, strings.TrimSpace(string(body)))
	}
	var meData map[string]any
	if err := json.NewDecoder(meRes.Body).Decode(&meData); err != nil {
		t.Fatalf("decode csrf token: %v", err)
	}
	csrfToken, _ := meData["csrfToken"].(string)
	if csrfToken == "" {
		t.Fatalf("expected csrf token in admin me response: %#v", meData)
	}

	form := url.Values{}
	form.Set("type", commandType)
	if payload != "" {
		form.Set("payload", payload)
	}
	form.Set("targetType", "device")
	form.Set("targetDeviceId", e.deviceID)
	form.Set("csrfToken", csrfToken)

	postReq, err := http.NewRequest(http.MethodPost, e.baseURL+"/admin/commands/create", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build command form request: %v", err)
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	postRes, err := e.client.Do(postReq)
	if err != nil {
		t.Fatalf("issue command form request: %v", err)
	}
	defer postRes.Body.Close()
	if postRes.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(postRes.Body)
		t.Fatalf("expected command redirect, got %d: %s", postRes.StatusCode, strings.TrimSpace(string(body)))
	}

	var commandID string
	err = e.pool.QueryRow(context.Background(), `
		SELECT id
		FROM commands
		WHERE tenant_id = $1 AND device_id = $2 AND type = $3
		ORDER BY created_at DESC
		LIMIT 1
	`, bootstrap.SeedTenantID, e.deviceID, commandType).Scan(&commandID)
	if err != nil {
		t.Fatalf("load issued command: %v", err)
	}
	if commandID == "" {
		t.Fatalf("command id was empty for type %s", commandType)
	}
	return commandID
}

func mustAdminSessionID(t *testing.T, baseURL string) string {
	t.Helper()
	client := newHTTPClient(t)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/admin/login", strings.NewReader(fmt.Sprintf("username=%s&password=%s", adminUsername, adminPassword)))
	if err != nil {
		t.Fatalf("build admin login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("admin login request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected login redirect, got %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var sessionCookie *http.Cookie
	for _, cookie := range res.Cookies() {
		if cookie.Name == auth.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("admin session cookie was not returned")
	}
	if strings.TrimSpace(sessionCookie.Value) == "" {
		t.Fatalf("admin session cookie was empty")
	}
	return sessionCookie.Value
}

func (e *commandTestEnv) waitForCommandAck(t *testing.T, commandID, expectedMessage string, expectedTransportSource string) {
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
			if result["message"] != expectedMessage {
				return false, nil
			}
			if expectedTransportSource == "" {
				return true, nil
			}
			details, _ := result["details"].(map[string]any)
			if details == nil {
				return false, nil
			}
			return details["transportSource"] == expectedTransportSource, nil
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
func buildTestServer(t *testing.T, handler http.Handler, launcherAPKPath string, requests *requestRecorder) *httptest.Server {
	t.Helper()
	recordingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			body := ""
			if r.Body != nil {
				raw, err := io.ReadAll(r.Body)
				if err == nil {
					body = string(raw)
				}
				r.Body = io.NopCloser(bytes.NewReader(raw))
			}
			requests.record(r.Method, r.URL.Path, body)
		}
		handler.ServeHTTP(w, r)
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/launcher.apk", func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			requests.record(r.Method, r.URL.Path, "")
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
	body   string
}

type requestRecorder struct {
	mu       sync.Mutex
	requests []requestRecord
}

func newRequestRecorder() *requestRecorder {
	return &requestRecorder{}
}

func (r *requestRecorder) record(method, path, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, requestRecord{method: method, path: path, body: body})
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

func (r *requestRecorder) waitForAfter(t *testing.T, start int, timeout time.Duration, description string, match func(requestRecord) bool) {
	t.Helper()
	waitForCondition(t, timeout, description, func() string {
		r.mu.Lock()
		defer r.mu.Unlock()
		if start < 0 {
			start = 0
		}
		if start > len(r.requests) {
			start = len(r.requests)
		}
		parts := make([]string, 0, len(r.requests[start:]))
		for _, req := range r.requests[start:] {
			parts = append(parts, req.method+" "+req.path)
		}
		return strings.Join(parts, " | ")
	}, func() (bool, error) {
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
				return true, nil
			}
		}
		return false, nil
	})
}

func waitForConfigSnapshotFetch(t *testing.T, requests *requestRecorder, deviceID string) {
	t.Helper()
	requests.waitFor(t, time.Minute, "config snapshot fetch", func(r requestRecord) bool {
		return r.method == http.MethodGet &&
			r.path == "/api/v1/devices/"+deviceID+"/config"
	})
}

func waitForDeviceLogsUpload(t *testing.T, requests *requestRecorder, deviceID string) {
	t.Helper()
	requests.waitFor(t, time.Minute, "device log upload", func(r requestRecord) bool {
		return r.method == http.MethodPost &&
			r.path == "/api/v1/devices/"+deviceID+"/logs"
	})
}

func waitForDeviceInfoUpload(t *testing.T, requests *requestRecorder, deviceID string) {
	t.Helper()
	requests.waitFor(t, time.Minute, "device info upload", func(r requestRecord) bool {
		return r.method == http.MethodPost &&
			r.path == "/api/v1/devices/"+deviceID+"/info"
	})
}

func assertDeviceLogsUploadPayload(t *testing.T, requests *requestRecorder, deviceID string) {
	t.Helper()
	requests.mu.Lock()
	defer requests.mu.Unlock()
	var payloadBody string
	for _, req := range requests.requests {
		if req.method == http.MethodPost && req.path == "/api/v1/devices/"+deviceID+"/logs" && strings.TrimSpace(req.body) != "" {
			payloadBody = req.body
			break
		}
	}
	if payloadBody == "" {
		t.Fatalf("did not capture a device log upload body for %s", deviceID)
	}
	var upload struct {
		ObservedAt string `json:"observedAt"`
		Entries    []struct {
			Source  string         `json:"source"`
			Level   string         `json:"level"`
			Message string         `json:"message"`
			Payload map[string]any `json:"payload"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(payloadBody), &upload); err != nil {
		t.Fatalf("decode device log upload body: %v", err)
	}
	if len(upload.Entries) == 0 {
		t.Fatalf("expected at least one device log entry, got none")
	}
	required := map[string]bool{
		"launcher|info|launcher started":           false,
		"bootstrap|info|bootstrap intent received": false,
		"bootstrap|info|bootstrap persisted":       false,
		"enrollment|info|enrollment started":       false,
		"enrollment|info|enrollment succeeded":     false,
	}
	var launcherStartedPayload map[string]any
	for _, entry := range upload.Entries {
		key := entry.Source + "|" + entry.Level + "|" + entry.Message
		if _, ok := required[key]; ok {
			required[key] = true
		}
		if entry.Source == "launcher" && entry.Level == "info" && entry.Message == "launcher started" {
			launcherStartedPayload = entry.Payload
		}
	}
	for key, seen := range required {
		if !seen {
			t.Fatalf("device log upload body did not include expected entry %q; body=%s", key, payloadBody)
		}
	}
	if launcherStartedPayload == nil {
		t.Fatalf("did not find launcher started payload in device log upload body; body=%s", payloadBody)
	}
	if _, ok := launcherStartedPayload["appVersionName"].(string); !ok || launcherStartedPayload["appVersionName"].(string) == "" {
		t.Fatalf("launcher started payload missing appVersionName; payload=%v", launcherStartedPayload)
	}
	if versionCode, ok := launcherStartedPayload["appVersionCode"].(float64); !ok || versionCode <= 0 {
		t.Fatalf("launcher started payload missing appVersionCode; payload=%v", launcherStartedPayload)
	}
}

func assertDeviceLogsRecordedViaAPI(t *testing.T, client *http.Client, baseURL, deviceID string) {
	t.Helper()
	time.Sleep(2 * time.Second)
	logs := mustFetchDeviceLogs(t, client, baseURL, deviceID)
	if !deviceLogsMatch(logs, deviceID) {
		t.Fatalf("device logs API did not include the expected launcher records: %s", deviceLogsSnapshot(logs))
	}
}

func assertDeviceInfoUploadPayload(t *testing.T, requests *requestRecorder, deviceID string) {
	t.Helper()
	requests.waitFor(t, time.Minute, "device info upload body", func(r requestRecord) bool {
		if r.method != http.MethodPost || r.path != "/api/v1/devices/"+deviceID+"/info" || strings.TrimSpace(r.body) == "" {
			return false
		}
		var upload struct {
			ObservedAt string         `json:"observedAt"`
			Payload    map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(r.body), &upload); err != nil || upload.Payload == nil {
			return false
		}
		return upload.Payload["deviceId"] == deviceID &&
			upload.Payload["model"] != nil &&
			upload.Payload["appPackage"] == "com.xmdm.launcher" &&
			upload.Payload["configRevision"] != nil &&
			upload.Payload["managedAppsVersion"] != nil &&
			upload.Payload["managedFilesVersion"] != nil
	})
	requests.mu.Lock()
	defer requests.mu.Unlock()
	var payloads []string
	for _, req := range requests.requests {
		if req.method == http.MethodPost && req.path == "/api/v1/devices/"+deviceID+"/info" && strings.TrimSpace(req.body) != "" {
			payloads = append(payloads, req.body)
		}
	}
	if len(payloads) == 0 {
		t.Fatalf("did not capture a device info upload body for %s", deviceID)
	}
	type deviceInfoUpload struct {
		ObservedAt string         `json:"observedAt"`
		Payload    map[string]any `json:"payload"`
	}
	hasBasic := false
	hasConfig := false
	for _, payloadBody := range payloads {
		var upload deviceInfoUpload
		if err := json.Unmarshal([]byte(payloadBody), &upload); err != nil {
			t.Fatalf("decode device info upload body: %v", err)
		}
		if upload.Payload == nil {
			t.Fatalf("device info upload body did not include payload: %s", payloadBody)
		}
		if upload.Payload["deviceId"] == deviceID &&
			upload.Payload["model"] != nil &&
			upload.Payload["appPackage"] == "com.xmdm.launcher" {
			hasBasic = true
		}
		if upload.Payload["configRevision"] != nil &&
			upload.Payload["managedAppsVersion"] != nil &&
			upload.Payload["managedFilesVersion"] != nil {
			hasConfig = true
		}
	}
	if !hasBasic {
		t.Fatalf("device info upload body did not include expected launcher inventory fields; first uploads=%s", deviceInfoUploadDiagnostics(payloads, deviceID))
	}
	if !hasConfig {
		t.Fatalf("device info upload body did not include expected config fields; first uploads=%s", deviceInfoUploadDiagnostics(payloads, deviceID))
	}
}

func deviceInfoUploadDiagnostics(payloads []string, deviceID string) string {
	type deviceInfoUpload struct {
		ObservedAt string         `json:"observedAt"`
		Payload    map[string]any `json:"payload"`
	}
	parts := make([]string, 0, len(payloads))
	for i, payloadBody := range payloads {
		var upload deviceInfoUpload
		if err := json.Unmarshal([]byte(payloadBody), &upload); err != nil {
			parts = append(parts, fmt.Sprintf("#%d decode_error=%v body=%q", i+1, err, payloadBody))
			continue
		}
		if upload.Payload == nil {
			parts = append(parts, fmt.Sprintf("#%d missing_payload body=%q", i+1, payloadBody))
			continue
		}
		missing := make([]string, 0, 6)
		if upload.Payload["deviceId"] != deviceID {
			missing = append(missing, "deviceId")
		}
		if upload.Payload["model"] == nil {
			missing = append(missing, "model")
		}
		if upload.Payload["appPackage"] != "com.xmdm.launcher" {
			missing = append(missing, "appPackage")
		}
		if upload.Payload["configRevision"] == nil {
			missing = append(missing, "configRevision")
		}
		if upload.Payload["managedAppsVersion"] == nil {
			missing = append(missing, "managedAppsVersion")
		}
		if upload.Payload["managedFilesVersion"] == nil {
			missing = append(missing, "managedFilesVersion")
		}
		if len(missing) == 0 {
			missing = append(missing, "none")
		}
		parts = append(parts, fmt.Sprintf("#%d observedAt=%s missing=%s", i+1, upload.ObservedAt, strings.Join(missing, ",")))
	}
	return strings.Join(parts, " | ")
}

func assertCertificateInstallReportedViaAPI(t *testing.T, requests *requestRecorder, deviceID string) {
	t.Helper()
	requests.waitFor(t, time.Minute, "device info upload after certificate install", func(r requestRecord) bool {
		if r.method != http.MethodPost || r.path != "/api/v1/devices/"+deviceID+"/info" || strings.TrimSpace(r.body) == "" {
			return false
		}
		var upload struct {
			ObservedAt string         `json:"observedAt"`
			Payload    map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(r.body), &upload); err != nil || upload.Payload == nil {
			return false
		}
		count, ok := upload.Payload["installedCaCertsCount"].(float64)
		if !ok || count <= 0 {
			return false
		}
		_, hasVersion := upload.Payload["certificatesVersion"]
		return hasVersion
	})
}

func assertDeviceInfoRecordedViaAPI(t *testing.T, client *http.Client, baseURL, deviceID string) {
	t.Helper()
	waitForCondition(t, time.Minute, "device info API record", func() string {
		info := mustFetchDeviceInfo(t, client, baseURL, deviceID)
		return deviceInfoSnapshot(info)
	}, func() (bool, error) {
		info := mustFetchDeviceInfo(t, client, baseURL, deviceID)
		return deviceInfoMatch(info, deviceID), nil
	})
}

func mustFetchDeviceLogs(t *testing.T, client *http.Client, baseURL, deviceID string) []any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/logs?deviceId="+deviceID+"&limit=50", nil)
	if err != nil {
		t.Fatalf("build device logs request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("device logs request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("device logs request got %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode device logs response: %v", err)
	}
	logs, _ := payload["logs"].([]any)
	return logs
}

func mustFetchDeviceInfo(t *testing.T, client *http.Client, baseURL, deviceID string) []any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/device-info?deviceId="+deviceID+"&limit=50", nil)
	if err != nil {
		t.Fatalf("build device info request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("device info request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("device info request got %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode device info response: %v", err)
	}
	info, _ := payload["deviceInfo"].([]any)
	return info
}

func deviceLogsMatch(logs []any, deviceID string) bool {
	required := map[string]bool{
		"bootstrap|info|bootstrap intent received": false,
		"bootstrap|info|bootstrap persisted":       false,
		"enrollment|info|enrollment started":       false,
		"enrollment|info|enrollment succeeded":     false,
		"config|info|config changed":               false,
	}
	for _, item := range logs {
		rec, _ := item.(map[string]any)
		source, _ := rec["source"].(string)
		level, _ := rec["level"].(string)
		message, _ := rec["message"].(string)
		key := source + "|" + level + "|" + message
		payload, _ := rec["payload"].(map[string]any)
		switch key {
		case "bootstrap|info|bootstrap intent received":
			if payload != nil {
				if _, ok := payload["bootstrapHash"]; ok {
					required[key] = true
				}
			}
		case "bootstrap|info|bootstrap persisted":
			if payload != nil {
				if _, ok := payload["bootstrapHash"]; ok {
					required[key] = true
				}
			}
		case "enrollment|info|enrollment started":
			if payload != nil {
				if _, ok := payload["bootstrapHash"]; ok {
					required[key] = true
				}
			}
		case "enrollment|info|enrollment succeeded":
			if payload != nil && payload["deviceId"] == deviceID {
				required[key] = true
			}
		case "config|info|config changed":
			if payload != nil {
				if payload["configRevision"] != nil &&
					payload["snapshotHash"] != nil &&
					payload["appCount"] != nil &&
					payload["fileCount"] != nil &&
					payload["certificateCount"] != nil {
					required[key] = true
				}
			}
		}
	}
	for _, seen := range required {
		if !seen {
			return false
		}
	}
	return true
}

func deviceLogsSnapshot(logs []any) string {
	if len(logs) == 0 {
		return "<empty>"
	}
	parts := make([]string, 0, len(logs))
	for _, item := range logs {
		rec, _ := item.(map[string]any)
		source, _ := rec["source"].(string)
		level, _ := rec["level"].(string)
		message, _ := rec["message"].(string)
		parts = append(parts, source+"|"+level+"|"+message)
	}
	return strings.Join(parts, " | ")
}

func deviceInfoMatch(info []any, deviceID string) bool {
	required := map[string]bool{
		"model":               false,
		"androidVersion":      false,
		"appPackage":          false,
		"configRevision":      false,
		"managedAppsVersion":  false,
		"managedFilesVersion": false,
	}
	for _, item := range info {
		rec, _ := item.(map[string]any)
		if rec["deviceId"] != deviceID {
			continue
		}
		payload, _ := rec["payload"].(map[string]any)
		if payload == nil {
			continue
		}
		for key := range required {
			if payload[key] != nil {
				required[key] = true
			}
		}
	}
	for _, seen := range required {
		if !seen {
			return false
		}
	}
	return true
}

func deviceInfoSnapshot(info []any) string {
	if len(info) == 0 {
		return "<empty>"
	}
	parts := make([]string, 0, len(info))
	for _, item := range info {
		rec, _ := item.(map[string]any)
		deviceID, _ := rec["deviceId"].(string)
		payload, _ := rec["payload"].(map[string]any)
		parts = append(parts, deviceID+"|"+fmt.Sprintf("%v", payload))
	}
	return strings.Join(parts, " | ")
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

func seedDefaultDevicePolicy(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	policyID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO policies (id, tenant_id, name, version, kiosk_mode, restrictions_json, status, updated_at)
		VALUES ($1, $2, $3, 1, false, '{}'::jsonb, 'active', NOW())
	`, policyID, bootstrap.SeedTenantID, "e2e-default-policy"); err != nil {
		t.Fatalf("seed default policy: %v", err)
	}
	return policyID
}

func seedPendingDevice(t *testing.T, pool *pgxpool.Pool, deviceRowID, policyID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO devices (id, tenant_id, display_name, secret_hash, policy_id, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', NOW())
	`, deviceRowID, bootstrap.SeedTenantID, deviceRowID, "hash-"+deviceRowID, policyID); err != nil {
		t.Fatalf("seed pending device: %v", err)
	}
}

func seedPolicyApp(t *testing.T, pool *pgxpool.Pool, policyID, appID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO policy_apps (id, tenant_id, policy_id, app_id, status, updated_at)
		VALUES ($1, $2, $3, $4, 'active', NOW())
		ON CONFLICT (tenant_id, policy_id, app_id) DO UPDATE SET status = 'active', updated_at = NOW()
	`, uuid.NewString(), bootstrap.SeedTenantID, policyID, appID); err != nil {
		t.Fatalf("seed policy app: %v", err)
	}
}

func seedPolicyManagedFile(t *testing.T, pool *pgxpool.Pool, policyID, managedFileID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO policy_managed_files (id, tenant_id, policy_id, managed_file_id, status, updated_at)
		VALUES ($1, $2, $3, $4, 'active', NOW())
		ON CONFLICT (tenant_id, policy_id, managed_file_id) DO UPDATE SET status = 'active', updated_at = NOW()
	`, uuid.NewString(), bootstrap.SeedTenantID, policyID, managedFileID); err != nil {
		t.Fatalf("seed policy managed file: %v", err)
	}
}

func seedPolicyCertificate(t *testing.T, pool *pgxpool.Pool, policyID, certificateID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO policy_certificates (id, tenant_id, policy_id, certificate_id, status, updated_at)
		VALUES ($1, $2, $3, $4, 'active', NOW())
		ON CONFLICT (tenant_id, policy_id, certificate_id) DO UPDATE SET status = 'active', updated_at = NOW()
	`, uuid.NewString(), bootstrap.SeedTenantID, policyID, certificateID); err != nil {
		t.Fatalf("seed policy certificate: %v", err)
	}
}

func updateDevicePolicy(t *testing.T, pool *pgxpool.Pool, deviceRowID, policyID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		UPDATE devices
		SET policy_id = $2, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $3
	`, bootstrap.SeedTenantID, policyID, deviceRowID); err != nil {
		t.Fatalf("update device policy: %v", err)
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
func mustBuildBootstrapURI(t *testing.T, client *http.Client, baseURL, launcherChecksum, deviceID, token string, extraBootstrapExtras map[string]string) string {
	t.Helper()
	extras := map[string]string{"CUSTOMER": "Acme"}
	for key, value := range extraBootstrapExtras {
		extras[key] = value
	}
	extrasJSON, err := json.Marshal(extras)
	if err != nil {
		t.Fatalf("marshal bootstrap extras: %v", err)
	}
	qrJSON := postJSON(t, client, baseURL+"/api/v1/enrollment/qr/json", fmt.Sprintf(`{
		"serverUrl":"%s",
		"enrollmentToken":"%s",
		"deviceAdminPackageDownloadLocation":"%s/launcher.apk",
		"deviceAdminPackageChecksum":"%s",
		"deviceIdentityPolicy":{
			"deviceId":"%s"
		},
		"bootstrapExtras":%s
	}`, baseURL, token, baseURL, launcherChecksum, deviceID, string(extrasJSON)))
	return encodeBootstrapURI(t, qrJSON)
}

// ── Content fixtures ─────────────────────────────────────────────────────────

// managedFileFixture holds the uploaded managed file's server-side identifiers.
type managedFileFixture struct {
	sourceID      string
	managedFileID string
	content       []byte
}

// certificateFixture holds the uploaded certificate source artifact's server-side identifiers.
type certificateFixture struct {
	sourceID      string
	certificateID string
	content       []byte
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
	managedFileID, _ := mf["id"].(string)
	if managedFileID == "" {
		t.Fatalf("managed file create returned empty id: %#v", mf)
	}
	if mf["path"] != "adb-managed-file.txt" {
		t.Fatalf("managed file create returned unexpected path: %v", mf["path"])
	}
	if mf["replaceVariables"] != true {
		t.Fatalf("managed file create returned unexpected replaceVariables: %v", mf["replaceVariables"])
	}

	return managedFileFixture{sourceID: sourceID, managedFileID: managedFileID, content: content}
}

// mustUploadCertificate uploads a certificate source artifact and registers it as an active certificate.
func mustUploadCertificate(t *testing.T, client *http.Client, baseURL string, artifactStore interface {
	Delete(context.Context, string) error
}) certificateFixture {
	t.Helper()

	content := testCertificatePEM()
	cs := checksum.SHA256Base64URL(content)
	storageKey := "artifacts/content-e2e/" + uuid.NewString() + "/wifi-root-ca.pem"

	fileResp := postMultipartFile(t, client, baseURL+"/api/v1/certificates", map[string]string{
		"name":       "wifi-root-ca",
		"storageKey": storageKey,
		"checksum":   cs,
		"sizeBytes":  fmt.Sprintf("%d", len(content)),
		"mimeType":   "application/x-pem-file",
	}, "file", "wifi-root-ca.pem", content)

	certificateID, _ := fileResp["id"].(string)
	if certificateID == "" {
		t.Fatalf("certificate create returned empty id: %#v", fileResp)
	}
	t.Cleanup(func() { _ = artifactStore.Delete(context.Background(), storageKey) })

	return certificateFixture{
		sourceID:      certificateID,
		certificateID: certificateID,
		content:       content,
	}
}

// mustRegisterChromeApp uploads the Chrome APK artifact and publishes it as a managed app version.
func mustRegisterChromeApp(t *testing.T, pool *pgxpool.Pool, client *http.Client, baseURL string, artifactStore interface {
	Delete(context.Context, string) error
}) string {
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

	appStore := appspg.New(pool)
	appRec, err := appStore.CreateApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{
		PackageName: chromePackage,
		Name:        chromeName,
	})
	if err != nil {
		t.Fatalf("create chrome app: %v", err)
	}
	appID := appRec.ID

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
	return appID
}

// mustRegisterLauncherUpdate publishes a new managed-app version for the seeded
// launcher app package and returns the app ID.
func mustRegisterLauncherUpdate(t *testing.T, pool *pgxpool.Pool, client *http.Client, baseURL string, artifactStore interface {
	Delete(context.Context, string) error
}, versionCode int64, versionName string) string {
	t.Helper()

	apk := mustReadLauncherAPKFixture(t, versionCode, versionName)
	checksumValue := checksum.SHA256Base64URL(apk)
	storageKey := "artifacts/content-e2e/" + uuid.NewString() + "/launcher.apk"
	fileName := fmt.Sprintf("launcher-%s.apk", strings.ReplaceAll(versionName, "/", "-"))
	uploadClient := newHTTPClient(t)
	login(uploadClient, t, baseURL, adminUsername, adminPassword)

	fileResp := postMultipartFile(t, uploadClient, baseURL+"/api/v1/files", map[string]string{
		"name":       fileName,
		"storageKey": storageKey,
		"checksum":   checksumValue,
		"sizeBytes":  fmt.Sprintf("%d", len(apk)),
		"mimeType":   "application/vnd.android.package-archive",
	}, "file", fileName, apk)

	artifact, _ := fileResp["artifact"].(map[string]any)
	artifactID, _ := artifact["id"].(string)
	if artifactID == "" {
		t.Fatalf("launcher file create returned empty artifact id: %#v", fileResp)
	}
	t.Cleanup(func() { _ = artifactStore.Delete(context.Background(), storageKey) })

	appRec, err := appspg.New(pool).GetAppByPackageName(context.Background(), bootstrap.SeedTenantID, bootstrap.SeedAgentAppPackage)
	if err != nil {
		t.Fatalf("get seeded launcher app: %v", err)
	}
	versionResp := postJSON(t, uploadClient, baseURL+"/api/v1/apps/"+appRec.ID+"/versions", fmt.Sprintf(`{
		"versionName":"%s",
		"versionCode":%d,
		"artifactId":"%s",
		"checksum":"%s",
		"publish":true
	}`, versionName, versionCode, artifactID, checksumValue))
	if versionResp["status"] != "published" {
		t.Fatalf("launcher version create returned unexpected status: %v", versionResp["status"])
	}
	return appRec.ID
}

// mustSeedLauncherApp ensures the seeded tenant has the system-owned launcher
// app row required for publishing a launcher update in e2e tests.
func mustSeedLauncherApp(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := appspg.New(pool).UpsertSystemOwnedApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{
		PackageName: bootstrap.SeedAgentAppPackage,
		Name:        bootstrap.SeedAgentAppName,
	}); err != nil {
		t.Fatalf("seed launcher app: %v", err)
	}
}

func testCertificatePEM() []byte {
	return []byte(`-----BEGIN CERTIFICATE-----
MIIDDzCCAfegAwIBAgIUFDeUC/bAkdG0MBP+Ux9kGcOSetwwDQYJKoZIhvcNAQEL
BQAwFzEVMBMGA1UEAwwMWE1ETSBUZXN0IENBMB4XDTI2MDUwMTA2MzYyNloXDTI3
MDUwMTA2MzYyNlowFzEVMBMGA1UEAwwMWE1ETSBUZXN0IENBMIIBIjANBgkqhkiG
9w0BAQEFAAOCAQ8AMIIBCgKCAQEAmzdNICgl2bmng2Ut3wWsYcjGZi2VVfMXq1Ca
M09+q5ZicuA2JfB9iHyAORrQ6lNItD6BlHONYNIOD2LyZlJhI/4/FuXAII4UXYRj
cuzZ1U/JBG6yWMGPYxtHPjny6Oy+UCroEBlXNGC/8LR3xzRr1xzluwnOL4j/BonV
h7O1Hb0iUYreMOGkzb/oizt+x0dxoGafq9Dkujz76z+HrrJXN4ybkYQgDZ3wK9TR
nmDqP2Dbz6Dej9mxx25cPqVFyaqArJKWK519Iow17et0SK+ELl7BLowEirI5VtU+
ZIEY0yDV9aJJT9+61xRZYs7D353CanF1ajiAjnWngxYAsw3NoQIDAQABo1MwUTAd
BgNVHQ4EFgQU4mGy0/yFDzK3zw/jCTtxxfNaTMIwHwYDVR0jBBgwFoAU4mGy0/yF
DzK3zw/jCTtxxfNaTMIwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOC
AQEAZatbIgJ9JNXRGvTPO1iOwNWxJQEWPGZb9pa0XQDN9VIGzMpLUjHBQCoMjWI8
qHmyH8813RSCwCXEBiKsUQFIexPYjSrGnNYaRnj50J7gjuczshR92npd8+3SwZWZ
mhI0Lwa6a9/QXqMrJ+FjuGTaOa7xHXPuLTCma6GdjfKLoIM/CtTZMZ9ZhpiZ4mdh
yDmhSLPQhKhlnLOldRuJb56ETMnPQhNeHi590pUa0E0RRvrxPdUDB0Nw1pq38Mqg
79ZGzWpdcg0DunOW0bYR3ihja1nMDjIpqaRb9BxV1e/PIwyb/pH2zhlke5G2npge
xonutDHiHIj+oco8QNeISHkh3w==
-----END CERTIFICATE-----`)
}

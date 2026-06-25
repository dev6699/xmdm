package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	v1 "xmdm/server/internal/api/v1"
	appspg "xmdm/server/internal/apps/postgres"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/bootstrap"
	certificatesspg "xmdm/server/internal/certificates/postgres"
	commandspg "xmdm/server/internal/commands/postgres"
	devicepg "xmdm/server/internal/device/postgres"
	deviceinfopg "xmdm/server/internal/deviceinfo/postgres"
	"xmdm/server/internal/enrollment"
	enrollmentpg "xmdm/server/internal/enrollment/postgres"
	filespg "xmdm/server/internal/files/postgres"
	grouppg "xmdm/server/internal/group/postgres"
	logspg "xmdm/server/internal/logs/postgres"
	managedfilespg "xmdm/server/internal/managedfiles/postgres"
	"xmdm/server/internal/mqttdynsec"
	"xmdm/server/internal/plugins"
	policypg "xmdm/server/internal/policy/postgres"
	"xmdm/server/internal/push"
	rolespg "xmdm/server/internal/roles/postgres"
	telemetrypg "xmdm/server/internal/telemetry/postgres"
	userspg "xmdm/server/internal/users/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── HTTP helpers ────────────────────────────────────────────────────────────

// newHTTPClient returns an *http.Client with a cookie jar that does not follow redirects.
func newHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ── Snapshot helpers ────────────────────────────────────────────────────────

// adbContentSnapshot returns a compact diagnostic string covering Chrome install state,
// managed-file content, and recent launcher log lines.
func adbContentSnapshot(t *testing.T, serial string) string {
	t.Helper()
	var parts []string
	parts = append(parts, chromePkgSnapshotPart(serial))
	parts = append(parts, managedFileSnapshotPart(serial))
	parts = append(parts, launcherLogSnapshotPart(serial))
	return strings.Join(parts, " | ")
}

// adbChromeSnapshot returns a compact diagnostic string covering Chrome install state
// and recent launcher log lines.
func adbChromeSnapshot(t *testing.T, serial string) string {
	t.Helper()
	return strings.Join([]string{
		chromePkgSnapshotPart(serial),
		launcherLogSnapshotPart(serial),
	}, " | ")
}

func chromePkgSnapshotPart(serial string) string {
	if out, err := adbShellOutput(serial, "pm", "list", "packages", "com.android.chrome"); err == nil {
		return "chrome=" + strings.TrimSpace(out)
	} else {
		return "chrome_err=" + strings.TrimSpace(err.Error())
	}
}

func managedFileSnapshotPart(serial string) string {
	if out, err := adbShellOutput(serial, "run-as", "com.xmdm.launcher", "cat", "files/managed-files/adb-managed-file.txt"); err == nil {
		return "managed_file=" + strings.TrimSpace(out)
	} else {
		return "managed_file_err=" + strings.TrimSpace(err.Error())
	}
}

func launcherLogSnapshotPart(serial string) string {
	if out, err := adb(serial, "shell", "logcat", "-d", "-v", "time", "XmdmLauncher:W", "*:S"); err == nil {
		return "launcher_logs=" + tailLines(strings.TrimSpace(out), 8)
	} else {
		return "launcher_logs_err=" + strings.TrimSpace(err.Error())
	}
}

// commandStatusSnapshot returns a compact diagnostic string for the given command's DB row.
func commandStatusSnapshot(t *testing.T, pool *pgxpool.Pool, commandID string) string {
	t.Helper()
	var status string
	var resultJSON []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT status, result_json FROM commands WHERE id = $1`, commandID,
	).Scan(&status, &resultJSON); err != nil {
		return "command_err=" + strings.TrimSpace(err.Error())
	}
	parts := []string{"status=" + status}
	if len(resultJSON) > 0 {
		parts = append(parts, "result="+strings.TrimSpace(string(resultJSON)))
	}
	return strings.Join(parts, " | ")
}

// deviceStatusSnapshot returns a compact diagnostic string for the given device row from the DB.
func deviceStatusSnapshot(t *testing.T, pool *pgxpool.Pool, deviceID string) string {
	t.Helper()
	rec, err := devicepg.New(pool).GetDevice(context.Background(), bootstrap.SeedTenantID, deviceID)
	if err != nil {
		return "device_err=" + strings.TrimSpace(err.Error())
	}
	return "status=" + rec.Status
}

// waitForDeviceEnrollment waits until the device row reaches enrolled status via the DB.
func waitForDeviceEnrollment(t *testing.T, pool *pgxpool.Pool, deviceID string) {
	t.Helper()
	waitForCondition(t, time.Minute, "device enrollment to complete",
		func() string { return deviceStatusSnapshot(t, pool, deviceID) },
		func() (bool, error) {
			var status string
			if err := pool.QueryRow(context.Background(),
				`SELECT status FROM devices WHERE id = $1`, deviceID,
			).Scan(&status); err != nil {
				return false, nil
			}
			return status == "enrolled" || status == "active", nil
		},
	)
}

// waitForDeviceEnrollmentInDB waits until the device row reaches enrolled status
// using the test database directly.
func waitForDeviceEnrollmentInDB(t *testing.T, pool *pgxpool.Pool, deviceID string) {
	t.Helper()
	waitForCondition(t, time.Minute, "device enrollment to complete in DB",
		func() string { return deviceStatusSnapshotInDB(t, pool, deviceID) },
		func() (bool, error) {
			var status string
			if err := pool.QueryRow(context.Background(),
				`SELECT status FROM devices WHERE id = $1`, deviceID,
			).Scan(&status); err != nil {
				return false, nil
			}
			return status == "enrolled" || status == "active", nil
		},
	)
}

func deviceStatusSnapshotInDB(t *testing.T, pool *pgxpool.Pool, deviceID string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM devices WHERE id = $1`, deviceID,
	).Scan(&status); err != nil {
		return "device_err=" + strings.TrimSpace(err.Error())
	}
	return "status=" + status
}

// tailLines returns the last maxLines of newline-separated text joined with " | ".
func tailLines(raw string, maxLines int) string {
	if maxLines <= 0 || raw == "" {
		return raw
	}
	lines := strings.Split(raw, "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, " | ")
	}
	return strings.Join(lines[len(lines)-maxLines:], " | ")
}

// ── URL / encoding utilities ─────────────────────────────────────────────────

// serverPortFromURL parses the port from rawURL, failing the test on error.
func serverPortFromURL(t *testing.T, rawURL string) string {
	t.Helper()
	port, err := serverPortFromURLNoTest(rawURL)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	return port
}

// serverPortFromURLNoTest parses the port from rawURL without access to *testing.T.
func serverPortFromURLNoTest(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if port := parsed.Port(); port != "" {
		return port, nil
	}
	return "", fmt.Errorf("server URL missing port: %s", rawURL)
}

// encodeBootstrapURI base64url-encodes the given QR JSON payload into a bootstrap URI.
func encodeBootstrapURI(t *testing.T, qrJSON map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(qrJSON)
	if err != nil {
		t.Fatalf("marshal QR json: %v", err)
	}
	return "base64url:" + strings.TrimRight(base64.URLEncoding.EncodeToString(raw), "=")
}

func login(client *http.Client, t *testing.T, baseURL, username, password string) {
	t.Helper()
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/admin/login", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected login redirect, got %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	if client.Jar != nil {
		for _, cookie := range res.Cookies() {
			client.Jar.SetCookies(req.URL, []*http.Cookie{
				{
					Name:  cookie.Name,
					Value: cookie.Value,
					Path:  "/",
				},
			})
		}
	}
}

func newE2EClient(t *testing.T, handler http.Handler) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{
		Jar:       jar,
		Transport: handlerTransport{handler: handler},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type handlerTransport struct {
	handler http.Handler
}

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.handler.ServeHTTP(rec, req)
	res := rec.Result()
	res.Request = req
	if res.Body == nil {
		res.Body = io.NopCloser(strings.NewReader(""))
	}
	return res, nil
}

func assertStatus(t *testing.T, client *http.Client, method, url, body string, want int) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	if res.StatusCode != want {
		t.Fatalf("expected %d, got %d for %s %s", want, res.StatusCode, method, url)
	}
}

func postJSON(t *testing.T, client *http.Client, url, body string) map[string]any {
	t.Helper()
	return doJSON(t, client, http.MethodPost, url, body, http.StatusOK)
}

func patchJSON(t *testing.T, client *http.Client, url, body string) map[string]any {
	t.Helper()
	return doJSON(t, client, http.MethodPatch, url, body, http.StatusOK)
}

func deleteJSON(t *testing.T, client *http.Client, url string) map[string]any {
	t.Helper()
	return doJSON(t, client, http.MethodDelete, url, "", http.StatusOK)
}

func getJSONList(t *testing.T, client *http.Client, url string) []map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build list request: %v", err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("list request got %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return listed
}

func doJSON(t *testing.T, client *http.Client, method, url, body string, want int) map[string]any {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != want {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected %d, got %d for %s %s: %s", want, res.StatusCode, method, url, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode json response: %v", err)
	}
	return payload
}

func testDeps(t *testing.T, pool *pgxpool.Pool, auditStore audit.Store, pluginManager *plugins.Manager, artifactStore artifacts.Store, enableMQTT bool) v1.Dependencies {
	devicesStore := devicepg.New(pool)
	enrollmentStore := enrollmentpg.New(pool)
	commandStore := commandspg.New(pool)
	deps := v1.Dependencies{
		Users:         userspg.New(pool),
		Roles:         rolespg.New(pool),
		Apps:          appspg.New(pool),
		Files:         filespg.New(pool),
		ManagedFiles:  managedfilespg.New(pool),
		Logs:          logspg.New(pool),
		Commands:      commandStore,
		DeviceInfo:    deviceinfopg.New(pool),
		Certificates:  certificatesspg.New(pool),
		Groups:        grouppg.New(pool),
		Policies:      policypg.New(pool),
		Devices:       devicesStore,
		Enrollment:    enrollmentStore,
		Telemetry:     telemetrypg.New(pool),
		Audit:         auditStore,
		PluginManager: pluginManager,
		Artifacts:     artifactStore,
		Runtime: enrollment.RuntimeSnapshot{
			MqttAddress: func() string {
				if enableMQTT {
					return "127.0.0.1:1883"
				}
				return ""
			}(),
			CommandPollIntervalMs: 1000,
			ConfigSyncIntervalMs:  1000,
		},
		TenantID: bootstrap.SeedTenantID,
	}
	if enableMQTT {
		if pub, err := push.NewMQTTPublisher(push.MQTTConfig{
			Address:  "127.0.0.1:1883",
			ClientID: "xmdm-server",
			Username: "xmdm-server",
			Password: "xmdm-server-secret",
		}); err == nil {
			commandStore.SetPublisher(pub)
		}
		if provisioner, err := mqttdynsec.New(mqttdynsec.Config{
			Address:  "127.0.0.1:1883",
			ClientID: "xmdm-dynsec",
			Username: "admin",
			Password: "xmdm-admin",
		}); err == nil {
			devicesStore.SetProvisioner(provisioner)
			enrollmentStore.SetProvisioner(provisioner)
			ensureTestServerPublisher(t, provisioner)
		}
	}
	return deps
}

func ensureTestServerPublisher(t *testing.T, provisioner mqttdynsec.Provisioner) {
	t.Helper()
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if err := provisioner.EnsureServerPublisher(context.Background(), "xmdm-server", "xmdm-server-secret"); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
			continue
		}
		return
	}
	t.Fatalf("ensure mqtt server publisher: %v", lastErr)
}

func postMultipartFile(t *testing.T, client *http.Client, url string, fields map[string]string, fileField, fileName string, content []byte) map[string]any {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write form field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatalf("create multipart file part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatalf("build multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("multipart upload request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(res.Body)
		t.Fatalf("expected %d, got %d for multipart upload: %s", http.StatusOK, res.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode multipart response: %v", err)
	}
	return payload
}

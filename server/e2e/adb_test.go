package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestADBFlow(t *testing.T) {
	serial := strings.TrimSpace(os.Getenv("XMDM_ADB_SERIAL"))
	serverURL := strings.TrimSpace(firstNonEmptyEnv("XMDM_ADB_SERVER_URL", "XMDM_SERVER_URL"))
	bootstrapURI := strings.TrimSpace(firstNonEmptyEnv("XMDM_ADB_BOOTSTRAP_URI", "XMDM_CONTENT_BOOTSTRAP_URI"))
	deviceID := strings.TrimSpace(firstNonEmptyEnv("XMDM_ADB_DEVICE_ID", "XMDM_CONTENT_DEVICE_ID"))
	adminUsername := strings.TrimSpace(firstNonEmptyEnv("XMDM_ADB_ADMIN_USERNAME", "XMDM_ADMIN_USERNAME"))
	adminPassword := strings.TrimSpace(firstNonEmptyEnv("XMDM_ADB_ADMIN_PASSWORD", "XMDM_ADMIN_PASSWORD"))
	if adminUsername == "" {
		adminUsername = "admin"
	}
	if adminPassword == "" {
		adminPassword = "admin"
	}
	launcherAPKPath := strings.TrimSpace(firstNonEmptyEnv("XMDM_ADB_LAUNCHER_APK", "XMDM_LAUNCHER_APK"))
	if launcherAPKPath == "" {
		launcherAPKPath = filepath.Join("..", "..", "app", "build", "outputs", "apk", "debug", "xmdm-agent-debug.apk")
	}
	if _, err := os.Stat(launcherAPKPath); err != nil {
		t.Skipf("launcher apk not available at %s: %v", launcherAPKPath, err)
	}
	if serial == "" || bootstrapURI == "" || deviceID == "" {
		t.Skip("set XMDM_ADB_SERIAL, XMDM_ADB_BOOTSTRAP_URI (or XMDM_CONTENT_BOOTSTRAP_URI), and XMDM_ADB_DEVICE_ID (or XMDM_CONTENT_DEVICE_ID) to run the adb troubleshooting helper")
	}
	if _, err := exec.LookPath("adb"); err != nil {
		t.Skipf("adb not available: %v", err)
	}
	runADBFlow(t, serial, serverURL, bootstrapURI, launcherAPKPath, deviceID, adminUsername, adminPassword)
}

func runADBFlow(t *testing.T, serial, serverURL, bootstrapURI, launcherAPKPath, deviceID, adminUsername, adminPassword string) {
	t.Helper()

	effectiveServerURL := strings.TrimSpace(serverURL)
	if effectiveServerURL == "" {
		var err error
		effectiveServerURL, err = serverURLFromBootstrapURI(bootstrapURI)
		if err != nil {
			t.Fatalf("decode bootstrap server url: %v", err)
		}
	}

	t.Logf("adb flow start serial=%s server=%s device=%s", serial, effectiveServerURL, deviceID)
	cleanupServerDeviceState(t, effectiveServerURL, bootstrapURI, adminUsername, adminPassword, serial, deviceID)
	resetADBLauncherState(t, serial, launcherAPKPath)
	reverseADBPort(t, serial, effectiveServerURL)
	t.Cleanup(func() {
		_ = removeADBPortReverse(serial, effectiveServerURL)
	})

	if _, err := adb(serial, "shell", "am", "start", "-S", "-n", "com.xmdm.launcher/.MainActivity", "--ez", "com.xmdm.launcher.EXTRA_RESET_STATE", "true", "-d", bootstrapURI); err != nil {
		t.Fatalf("start launcher on device: %v", err)
	}
	failIfRecoveryScreen(t, serial)

	waitForADBCondition(t, 12*time.Minute, "managed file to render on device", func() string {
		return adbContentSnapshot(t, serial)
	}, func() (bool, error) {
		failIfRecoveryScreen(t, serial)
		out, err := adbShellOutput(serial, "run-as", "com.xmdm.launcher", "cat", "files/managed-files/adb-managed-file.txt")
		if err != nil {
			return false, nil
		}
		want := "managed-file-on-device " + deviceID + " Acme"
		return strings.TrimSpace(out) == want, nil
	})

	waitForADBCondition(t, 12*time.Minute, "Chrome to be installed on device", func() string {
		return adbChromeSnapshot(t, serial)
	}, func() (bool, error) {
		failIfRecoveryScreen(t, serial)
		out, err := adbShellOutput(serial, "pm", "list", "packages", "com.android.chrome")
		if err != nil {
			return false, nil
		}
		return strings.Contains(out, "com.android.chrome"), nil
	})

	out, err := adbShellOutput(serial, "run-as", "com.xmdm.launcher", "cat", "files/managed-files/adb-managed-file.txt")
	if err != nil {
		t.Fatalf("read managed file after wait: %v", err)
	}
	want := "managed-file-on-device " + deviceID + " Acme"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected managed file content: got %q want %q", strings.TrimSpace(out), want)
	}

	out, err = adbShellOutput(serial, "pm", "list", "packages", "com.android.chrome")
	if err != nil {
		t.Fatalf("check chrome package: %v", err)
	}
	if !strings.Contains(out, "com.android.chrome") {
		t.Fatalf("expected Chrome package to be installed, got %q", strings.TrimSpace(out))
	}
}

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

func reverseADBPort(t *testing.T, serial, baseURL string) {
	t.Helper()
	port := serverPortFromURL(t, baseURL)
	if _, err := adb(serial, "reverse", "tcp:"+port, "tcp:"+port); err != nil {
		t.Fatalf("adb reverse: %v", err)
	}
}

func removeADBPortReverse(serial, baseURL string) error {
	port, err := serverPortFromURLNoTest(baseURL)
	if err != nil {
		return err
	}
	_, err = adb(serial, "reverse", "--remove", "tcp:"+port, "tcp:"+port)
	return err
}

func serverPortFromURL(t *testing.T, rawURL string) string {
	t.Helper()
	port, err := serverPortFromURLNoTest(rawURL)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	return port
}

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

func resetADBLauncherState(t *testing.T, serial, launcherAPKPath string) {
	t.Helper()
	if _, err := adb(serial, "shell", "am", "force-stop", "com.xmdm.launcher"); err != nil {
		t.Fatalf("force-stop launcher: %v", err)
	}
	_, _ = adb(serial, "shell", "dpm", "remove-active-admin", "com.xmdm.launcher/.AdminReceiver")
	_, _ = adb(serial, "shell", "dpm", "remove-active-admin", "--user", "0", "com.xmdm.launcher/.AdminReceiver")
	_, _ = adb(serial, "shell", "pm", "uninstall", "--user", "0", "com.xmdm.launcher")
	_, _ = adb(serial, "shell", "pm", "clear", "com.xmdm.launcher")
	_, _ = adb(serial, "shell", "pm", "uninstall", "--user", "0", "com.android.chrome")
	if strings.TrimSpace(launcherAPKPath) != "" {
		if _, err := adb(serial, "install", "-r", launcherAPKPath); err != nil {
			t.Fatalf("install launcher apk: %v", err)
		}
		if _, err := adb(serial, "shell", "run-as", "com.xmdm.launcher", "rm", "-rf", "files", "cache", "shared_prefs", "databases"); err != nil {
			t.Fatalf("wipe launcher app data: %v", err)
		}
	}
}

func cleanupServerDeviceState(t *testing.T, serverURL, bootstrapURI, adminUsername, adminPassword string, deviceIDs ...string) {
	t.Helper()
	if len(deviceIDs) == 0 {
		return
	}
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		var err error
		serverURL, err = serverURLFromBootstrapURI(bootstrapURI)
		if err != nil {
			t.Fatalf("decode bootstrap server url: %v", err)
		}
	}
	client := newHTTPClient(t)
	login(client, t, serverURL, adminUsername, adminPassword)
	devices := getJSONList(t, client, serverURL+"/api/v1/devices")
	for _, rec := range devices {
		name, _ := rec["name"].(string)
		matched := false
		for _, deviceID := range deviceIDs {
			if strings.TrimSpace(name) == strings.TrimSpace(deviceID) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		id, _ := rec["id"].(string)
		if id == "" {
			continue
		}
		_ = deleteJSON(t, client, serverURL+"/api/v1/devices/"+id)
	}
}

func failIfRecoveryScreen(t *testing.T, serial string) {
	t.Helper()
	if _, err := adb(serial, "shell", "uiautomator", "dump", "/sdcard/mdm-ui.xml"); err != nil {
		return
	}
	rawXML, err := adb(serial, "shell", "cat", "/sdcard/mdm-ui.xml")
	if err != nil {
		return
	}
	if strings.Contains(rawXML, "recoveryDetails") || strings.Contains(rawXML, "device already enrolled") {
		t.Fatalf("launcher is in recovery: device already enrolled")
	}
}

func waitForADBCondition(t *testing.T, timeout time.Duration, description string, snapshot func() string, fn func() (bool, error)) {
	t.Helper()
	start := time.Now()
	deadline := start.Add(timeout)
	var lastErr error
	lastLogAt := time.Time{}
	attempts := 0
	for time.Now().Before(deadline) {
		attempts++
		ok, err := fn()
		if err == nil && ok {
			t.Logf("wait succeeded: %s attempts=%d elapsed=%s", description, attempts, time.Since(start).Truncate(time.Second))
			return
		}
		if err != nil {
			lastErr = err
		}
		if lastLogAt.IsZero() || time.Since(lastLogAt) >= 30*time.Second {
			if snapshot != nil {
				t.Logf("waiting for %s attempts=%d elapsed=%s state=%s", description, attempts, time.Since(start).Truncate(time.Second), snapshot())
			} else {
				t.Logf("waiting for %s attempts=%d elapsed=%s", description, attempts, time.Since(start).Truncate(time.Second))
			}
			lastLogAt = time.Now()
		}
		time.Sleep(5 * time.Second)
	}
	if snapshot != nil {
		t.Logf("wait timed out for %s state=%s", description, snapshot())
	}
	if lastErr != nil {
		t.Fatalf("timeout waiting for %s: %v", description, lastErr)
	}
	t.Fatalf("timeout waiting for %s", description)
}

func adbContentSnapshot(t *testing.T, serial string) string {
	t.Helper()
	parts := make([]string, 0, 3)
	if out, err := adbShellOutput(serial, "pm", "list", "packages", "com.android.chrome"); err == nil {
		parts = append(parts, "chrome="+strings.TrimSpace(out))
	} else {
		parts = append(parts, "chrome_err="+strings.TrimSpace(err.Error()))
	}
	if out, err := adbShellOutput(serial, "run-as", "com.xmdm.launcher", "cat", "files/managed-files/adb-managed-file.txt"); err == nil {
		parts = append(parts, "managed_file="+strings.TrimSpace(out))
	} else {
		parts = append(parts, "managed_file_err="+strings.TrimSpace(err.Error()))
	}
	if out, err := adb(serial, "shell", "logcat", "-d", "-v", "time", "XmdmLauncher:W", "*:S"); err == nil {
		parts = append(parts, "launcher_logs="+tailLines(strings.TrimSpace(out), 8))
	} else {
		parts = append(parts, "launcher_logs_err="+strings.TrimSpace(err.Error()))
	}
	return strings.Join(parts, " | ")
}

func adbChromeSnapshot(t *testing.T, serial string) string {
	t.Helper()
	parts := make([]string, 0, 2)
	if out, err := adbShellOutput(serial, "pm", "list", "packages", "com.android.chrome"); err == nil {
		parts = append(parts, "chrome="+strings.TrimSpace(out))
	} else {
		parts = append(parts, "chrome_err="+strings.TrimSpace(err.Error()))
	}
	if out, err := adb(serial, "shell", "logcat", "-d", "-v", "time", "XmdmLauncher:W", "*:S"); err == nil {
		parts = append(parts, "launcher_logs="+tailLines(strings.TrimSpace(out), 8))
	} else {
		parts = append(parts, "launcher_logs_err="+strings.TrimSpace(err.Error()))
	}
	return strings.Join(parts, " | ")
}

func commandStatusSnapshot(t *testing.T, pool *pgxpool.Pool, commandID string) string {
	t.Helper()
	var status string
	var resultJSON []byte
	if err := pool.QueryRow(context.Background(), `SELECT status, result_json FROM commands WHERE id = $1`, commandID).Scan(&status, &resultJSON); err != nil {
		return "command_err=" + strings.TrimSpace(err.Error())
	}
	parts := []string{"status=" + status}
	if len(resultJSON) > 0 {
		parts = append(parts, "result="+strings.TrimSpace(string(resultJSON)))
	}
	return strings.Join(parts, " | ")
}

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

func adb(serial string, args ...string) (string, error) {
	cmdArgs := append([]string{"-s", serial}, args...)
	cmd := exec.Command("adb", cmdArgs...)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func adbShellOutput(serial string, args ...string) (string, error) {
	return adb(serial, append([]string{"shell"}, args...)...)
}

func encodeBootstrapURI(t *testing.T, qrJSON map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(qrJSON)
	if err != nil {
		t.Fatalf("marshal QR json: %v", err)
	}
	return "base64url:" + strings.TrimRight(base64.URLEncoding.EncodeToString(raw), "=")
}

func serverURLFromBootstrapURI(bootstrapURI string) (string, error) {
	raw := strings.TrimSpace(bootstrapURI)
	raw = strings.TrimPrefix(raw, "base64url:")
	decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(raw)
	if err != nil {
		return "", err
	}
	var payload struct {
		ServerURL string         `json:"serverUrl"`
		Extras    map[string]any `json:"android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.ServerURL) == "" {
		if extras, ok := payload.Extras["com.xmdm.BASE_URL"].(string); ok && strings.TrimSpace(extras) != "" {
			payload.ServerURL = extras
		} else if extras, ok := payload.Extras["BASE_URL"].(string); ok && strings.TrimSpace(extras) != "" {
			payload.ServerURL = extras
		}
	}
	if strings.TrimSpace(payload.ServerURL) == "" {
		return "", fmt.Errorf("bootstrap payload missing serverUrl")
	}
	return payload.ServerURL, nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

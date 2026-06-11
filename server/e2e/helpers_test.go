package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

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

// deviceStatusSnapshot returns a compact diagnostic string for the given device row from the API.
func deviceStatusSnapshot(t *testing.T, client *http.Client, baseURL, deviceID string) string {
	t.Helper()
	for _, rec := range getJSONList(t, client, baseURL+"/api/v1/devices") {
		name, _ := rec["name"].(string)
		if strings.TrimSpace(name) != strings.TrimSpace(deviceID) {
			continue
		}
		status, _ := rec["status"].(string)
		if status == "" {
			return "device_status_missing=" + fmt.Sprint(rec)
		}
		return "status=" + status
	}
	return "device_err=not found"
}

// waitForDeviceEnrollment waits until the device row reaches enrolled status via the API.
func waitForDeviceEnrollment(t *testing.T, client *http.Client, baseURL, deviceID string) {
	t.Helper()
	waitForCondition(t, time.Minute, "device enrollment to complete",
		func() string { return deviceStatusSnapshot(t, client, baseURL, deviceID) },
		func() (bool, error) {
			for _, rec := range getJSONList(t, client, baseURL+"/api/v1/devices") {
				name, _ := rec["name"].(string)
				if strings.TrimSpace(name) != strings.TrimSpace(deviceID) {
					continue
				}
				status, _ := rec["status"].(string)
				return status == "enrolled" || status == "active", nil
			}
			return false, nil
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
}

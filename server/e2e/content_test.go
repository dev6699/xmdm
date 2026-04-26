package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	v1 "xmdm/server/internal/api/v1"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/plugins"
)

func TestContentE2E(t *testing.T) {
	serial := strings.TrimSpace(os.Getenv("XMDM_ADB_SERIAL"))
	if serial == "" {
		t.Skip("XMDM_ADB_SERIAL must be set for the adb-backed content e2e")
	}
	if _, err := exec.LookPath("adb"); err != nil {
		t.Skipf("adb not available: %v", err)
	}

	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	auditStore := auditpg.NewDBStore(pool)
	artifactStore := newTestArtifactStore(t)
	handler := v1.NewMux(svc, testDeps(pool, auditStore, plugins.Disabled(), artifactStore))

	launcherAPKPath := filepath.Join("..", "..", "app", "build", "outputs", "apk", "debug", "xmdm-agent-debug.apk")
	launcherAPK, err := os.ReadFile(launcherAPKPath)
	if err != nil {
		t.Fatalf("read launcher apk: %v", err)
	}
	launcherChecksum := checksum.SHA256Base64URL(launcherAPK)

	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/launcher.apk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		http.ServeFile(w, r, launcherAPKPath)
	})
	serverMux.Handle("/", handler)

	server := httptest.NewServer(serverMux)
	defer server.Close()

	client := newHTTPClient(t)
	baseURL := server.URL
	login(client, t, baseURL, "admin", "secret")

	reverseADBPort(t, serial, baseURL)
	t.Cleanup(func() {
		_ = removeADBPortReverse(serial, baseURL)
	})
	resetADBLauncherState(t, serial, launcherAPKPath)

	managedFileSourceContent := []byte("managed-file-on-device DEVICE_NUMBER CUSTOMER")
	managedFileSourceChecksum := checksum.SHA256Base64URL(managedFileSourceContent)
	managedFileStorageKey := "artifacts/content-e2e/" + uuid.NewString() + "/managed-file.txt"
	managedFileSource := postMultipartFile(t, client, baseURL+"/api/v1/files", map[string]string{
		"name":       "managed-file.txt",
		"storageKey": managedFileStorageKey,
		"checksum":   managedFileSourceChecksum,
		"sizeBytes":  fmt.Sprintf("%d", len(managedFileSourceContent)),
		"mimeType":   "text/plain",
	}, "file", "managed-file.txt", managedFileSourceContent)
	managedFileSourceID, _ := managedFileSource["id"].(string)
	if managedFileSourceID == "" {
		t.Fatalf("managed file source create returned empty id: %#v", managedFileSource)
	}
	t.Cleanup(func() {
		_ = artifactStore.Delete(context.Background(), managedFileStorageKey)
	})

	managedFile := postJSON(t, client, baseURL+"/api/v1/managed-files", `{
		"fileId":"`+managedFileSourceID+`",
		"path":"adb-managed-file.txt",
		"replaceVariables":true
	}`)
	if managedFile["path"] != "adb-managed-file.txt" {
		t.Fatalf("managed file create returned path %v", managedFile["path"])
	}
	if managedFile["replaceVariables"] != true {
		t.Fatalf("managed file create returned replaceVariables %v", managedFile["replaceVariables"])
	}

	chromeAPKPath := filepath.Join("..", "..", "artifacts", "chrome.apk")
	chromeAPK, err := os.ReadFile(chromeAPKPath)
	if err != nil {
		t.Fatalf("read chrome apk: %v", err)
	}
	chromeChecksum := checksum.SHA256Base64URL(chromeAPK)
	chromeStorageKey := "artifacts/content-e2e/" + uuid.NewString() + "/chrome.apk"
	chromeFile := postMultipartFile(t, client, baseURL+"/api/v1/files", map[string]string{
		"name":       "chrome.apk",
		"storageKey": chromeStorageKey,
		"checksum":   chromeChecksum,
		"sizeBytes":  fmt.Sprintf("%d", len(chromeAPK)),
		"mimeType":   "application/vnd.android.package-archive",
	}, "file", "chrome.apk", chromeAPK)
	chromeArtifact, _ := chromeFile["artifact"].(map[string]any)
	chromeArtifactID, _ := chromeArtifact["id"].(string)
	if chromeArtifactID == "" {
		t.Fatalf("chrome file create returned empty artifact id: %#v", chromeFile)
	}
	t.Cleanup(func() {
		_ = artifactStore.Delete(context.Background(), chromeStorageKey)
	})

	chromeApp := postJSON(t, client, baseURL+"/api/v1/apps", `{
		"packageName":"com.android.chrome",
		"name":"Chrome"
	}`)
	chromeAppID, _ := chromeApp["id"].(string)
	if chromeAppID == "" {
		t.Fatalf("chrome app create returned empty id: %#v", chromeApp)
	}

	chromeVersion := postJSON(t, client, baseURL+"/api/v1/apps/"+chromeAppID+"/versions", `{
		"versionName":"138.0.7204.179",
		"versionCode":720417920,
		"artifactId":"`+chromeArtifactID+`",
		"checksum":"`+chromeChecksum+`",
		"publish":true
	}`)
	if chromeVersion["status"] != "published" {
		t.Fatalf("chrome version create returned status %v", chromeVersion["status"])
	}

	if _, err := pool.Exec(context.Background(), `DELETE FROM device_telemetry; DELETE FROM enrollment_tokens; DELETE FROM devices;`); err != nil {
		t.Fatalf("reset enrollment state: %v", err)
	}
	var deviceCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM devices`).Scan(&deviceCount); err != nil {
		t.Fatalf("count devices after reset: %v", err)
	}
	if deviceCount != 0 {
		t.Fatalf("expected 0 devices after reset, got %d", deviceCount)
	}

	deviceID := "content-e2e-" + uuid.NewString()

	runCommandScenario := func(t *testing.T, name string, mqttAddress string) {
		t.Helper()
		if mqttAddress != "" {
			if _, err := adb(serial, "reverse", "tcp:1883", "tcp:1883"); err != nil {
				t.Fatalf("adb reverse mqtt: %v", err)
			}
			t.Cleanup(func() {
				_, _ = adb(serial, "reverse", "--remove", "tcp:1883", "tcp:1883")
			})
		}

		enrollmentToken := postJSON(t, client, baseURL+"/api/v1/enrollment/tokens", `{"ttlSeconds":3600}`)
		token, _ := enrollmentToken["token"].(string)
		if token == "" {
			t.Fatalf("enrollment token response did not include token: %#v", enrollmentToken)
		}

		extras := `{"CUSTOMER":"Acme"}`
		if mqttAddress != "" {
			extras = `{"CUSTOMER":"Acme","MQTT_ADDRESS":"` + mqttAddress + `"}`
		}
		qrJSON := postJSON(t, client, baseURL+"/api/v1/enrollment/qr/json", `{
			"serverUrl":"`+baseURL+`",
			"serverProject":"rest",
			"enrollmentToken":"`+token+`",
			"deviceAdminPackageDownloadLocation":"`+baseURL+`/launcher.apk",
			"deviceAdminPackageChecksum":"`+launcherChecksum+`",
			"deviceIdentityPolicy":{
				"deviceId":"`+deviceID+`",
				"deviceIdUse":"serial"
			},
			"bootstrapExtras":`+extras+`
		}`)
		transportBootstrapURI := encodeBootstrapURI(t, qrJSON)

		runADBFlow(t, serial, baseURL, transportBootstrapURI, launcherAPKPath, deviceID, "admin", "secret")

		command := postJSON(t, client, baseURL+"/api/v1/admin/commands", `{
			"type":"ping",
			"target":{
				"type":"device",
				"deviceId":"`+deviceID+`"
			}
		}`)
		createdCommands, _ := command["commands"].([]any)
		if len(createdCommands) != 1 {
			t.Fatalf("%s: expected one command row, got %#v", name, command["commands"])
		}
		createdCommand, _ := createdCommands[0].(map[string]any)
		commandID, _ := createdCommand["id"].(string)
		if commandID == "" {
			t.Fatalf("%s: command response did not include an id: %#v", name, createdCommand)
		}
		if createdCommand["type"] != "ping" {
			t.Fatalf("%s: unexpected command type: %#v", name, createdCommand["type"])
		}
		if createdCommand["deviceId"] != deviceID {
			t.Fatalf("%s: unexpected command device: %#v", name, createdCommand["deviceId"])
		}

		waitForADBCondition(t, 4*time.Minute, name+" command ack to reach the server", func() string {
			return commandStatusSnapshot(t, pool, commandID)
		}, func() (bool, error) {
			var status string
			var resultJSON []byte
			if err := pool.QueryRow(context.Background(), `SELECT status, result_json FROM commands WHERE id = $1`, commandID).Scan(&status, &resultJSON); err != nil {
				return false, nil
			}
			return status == "acked" && strings.Contains(string(resultJSON), `"message":"pong"`), nil
		})

		if mqttAddress == "" {
			if _, err := adb(serial, "shell", "am", "force-stop", "com.xmdm.launcher"); err != nil {
				t.Fatalf("%s: force-stop launcher before expiry test: %v", name, err)
			}

			expiryAt := time.Now().Add(2 * time.Second).UTC().Format(time.RFC3339Nano)
			expiryCommand := postJSON(t, client, baseURL+"/api/v1/admin/commands", `{
				"type":"ping",
				"expiresAt":"`+expiryAt+`",
				"target":{
					"type":"device",
					"deviceId":"`+deviceID+`"
				}
			}`)
			expiryCommands, _ := expiryCommand["commands"].([]any)
			if len(expiryCommands) != 1 {
				t.Fatalf("%s: expected one expiring command row, got %#v", name, expiryCommand["commands"])
			}
			expiryRow, _ := expiryCommands[0].(map[string]any)
			expiryCommandID, _ := expiryRow["id"].(string)
			if expiryCommandID == "" {
				t.Fatalf("%s: expiring command response did not include an id: %#v", name, expiryRow)
			}

			time.Sleep(3 * time.Second)
			if _, err := adb(serial, "shell", "am", "start", "-S", "-n", "com.xmdm.launcher/.MainActivity", "-d", transportBootstrapURI); err != nil {
				t.Fatalf("%s: restart launcher for expiry test: %v", name, err)
			}

			waitForADBCondition(t, 2*time.Minute, name+" command to expire on the server", func() string {
				return commandStatusSnapshot(t, pool, expiryCommandID)
			}, func() (bool, error) {
				var status string
				if err := pool.QueryRow(context.Background(), `SELECT status FROM commands WHERE id = $1`, expiryCommandID).Scan(&status); err != nil {
					return false, nil
				}
				return status == "expired", nil
			})
		}
	}

	t.Run("mqtt", func(t *testing.T) {
		runCommandScenario(t, "mqtt", "127.0.0.1:1883")
	})
	t.Run("polling", func(t *testing.T) {
		runCommandScenario(t, "polling", "")
	})
}

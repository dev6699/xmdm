package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEnrollmentCommandsAgainstLiveServer(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	t.Setenv("XMDM_SESSION_FILE", sessionPath)

	loginLiveAdmin(t)

	issueTTL := "2h"
	validateIssued := issueEnrollmentToken(t, issueTTL)
	t.Cleanup(func() {
		deleteEnrollmentTokenByID(t, validateIssued.ID)
	})
	if validateIssued.ID == "" || validateIssued.Secret == "" {
		t.Fatalf("unexpected issue response: %#v", validateIssued)
	}
	if validateIssued.Status != "issued" {
		t.Fatalf("unexpected issue status: %q", validateIssued.Status)
	}

	validateOut := runCLI(t, []string{"--config", "../../config.yaml", "enrollment", "tokens", "validate", validateIssued.Secret}, "1.2.3").stdout
	validated := decodeEnrollmentTokenRecord(t, validateOut)
	if validated.ID != validateIssued.ID {
		t.Fatalf("validate id mismatch: got %q want %q", validated.ID, validateIssued.ID)
	}
	if validated.Status != "issued" {
		t.Fatalf("unexpected validate status: %q", validated.Status)
	}

	revokeOut := runCLI(t, []string{"--config", "../../config.yaml", "enrollment", "tokens", "revoke", validateIssued.ID}, "1.2.3").stdout
	revoked := decodeEnrollmentTokenRecord(t, revokeOut)
	if revoked.ID != validateIssued.ID {
		t.Fatalf("revoke id mismatch: got %q want %q", revoked.ID, validateIssued.ID)
	}
	if revoked.Status != "revoked" {
		t.Fatalf("unexpected revoke status: %q", revoked.Status)
	}

	consumeIssued := issueEnrollmentToken(t, issueTTL)
	t.Cleanup(func() {
		deleteEnrollmentTokenByID(t, consumeIssued.ID)
	})
	consumeOut := runCLI(t, []string{"--config", "../../config.yaml", "enrollment", "tokens", "consume", consumeIssued.Secret}, "1.2.3").stdout
	consumed := decodeEnrollmentTokenRecord(t, consumeOut)
	if consumed.ID != consumeIssued.ID {
		t.Fatalf("consume id mismatch: got %q want %q", consumed.ID, consumeIssued.ID)
	}
	if consumed.Status != "consumed" {
		t.Fatalf("unexpected consume status: %q", consumed.Status)
	}

	qrIssued := issueEnrollmentToken(t, issueTTL)
	t.Cleanup(func() {
		deleteEnrollmentTokenByID(t, qrIssued.ID)
	})

	qrJSONOut := runCLI(t, []string{
		"--config", "../../config.yaml",
		"enrollment", "qr", "json",
		"--server-project", "rest",
		"--enrollment-token", qrIssued.Secret,
		"--package-url", "https://cdn.example/launcher.apk",
		"--package-checksum", "abc123",
		"--device-id", "device-123",
		"--bootstrap-extras", `{"customer":"Acme","groups":["field"]}`,
	}, "1.2.3").stdout
	qrPayload := decodeJSONMap(t, qrJSONOut)
	if got := qrPayload["android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME"]; got != "com.xmdm.launcher/.AdminReceiver" {
		t.Fatalf("unexpected component: %#v", got)
	}
	if got := qrPayload["android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION"]; got != "https://cdn.example/launcher.apk" {
		t.Fatalf("unexpected download location: %#v", got)
	}
	if got := qrPayload["android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM"]; got != "abc123" {
		t.Fatalf("unexpected checksum: %#v", got)
	}
	extras := mustNestedMap(t, qrPayload["android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE"])
	if got := extras["com.xmdm.BASE_URL"]; got != "http://127.0.0.1:8080/api/v1" {
		t.Fatalf("unexpected base url: %#v", got)
	}
	if got := extras["com.xmdm.SERVER_PROJECT"]; got != "rest" {
		t.Fatalf("unexpected server project: %#v", got)
	}
	if got := extras["com.xmdm.ENROLLMENT_TOKEN"]; got != qrIssued.Secret {
		t.Fatalf("unexpected enrollment token: %#v", got)
	}
	if got := extras["com.xmdm.DEVICE_ID"]; got != "device-123" {
		t.Fatalf("unexpected device id: %#v", got)
	}
	if got := extras["com.xmdm.DEVICE_ID_USE"]; got != "serial" {
		t.Fatalf("unexpected device id use: %#v", got)
	}
	if got := extras["com.xmdm.CUSTOMER"]; got != "Acme" {
		t.Fatalf("unexpected customer: %#v", got)
	}
	if got := extras["com.xmdm.GROUP"]; got != "field" {
		t.Fatalf("unexpected group: %#v", got)
	}

	qrPNGPath := filepath.Join(t.TempDir(), "enrollment.png")
	_ = runCLI(t, []string{
		"--config", "../../config.yaml",
		"enrollment", "qr", "png",
		"--output", qrPNGPath,
		"--server-project", "rest",
		"--enrollment-token", qrIssued.Secret,
		"--package-url", "https://cdn.example/launcher.apk",
		"--package-checksum", "abc123",
		"--device-id", "device-123",
		"--bootstrap-extras", `{"customer":"Acme","groups":["field"]}`,
	}, "1.2.3")
	qrPNGBytes, err := os.ReadFile(qrPNGPath)
	if err != nil {
		t.Fatalf("read qr png: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(qrPNGBytes)); err != nil {
		t.Fatalf("decode qr png: %v", err)
	}
}

func loginLiveAdmin(t *testing.T) {
	t.Helper()
	out := runCLI(t, []string{"--config", "../../config.yaml", "auth", "login", "--username", "admin", "--password", "admin"}, "1.2.3").stdout
	if !strings.Contains(out, "logged in as admin") {
		t.Fatalf("unexpected login output: %s", out)
	}
}

type enrollmentIssuedToken struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Secret string `json:"token"`
	Tenant string `json:"tenantId"`
}

type enrollmentTokenRecord struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func issueEnrollmentToken(t *testing.T, ttl string) enrollmentIssuedToken {
	t.Helper()
	out := runCLI(t, []string{"--config", "../../config.yaml", "enrollment", "tokens", "issue", "--ttl", ttl}, "1.2.3").stdout
	var token enrollmentIssuedToken
	if err := json.Unmarshal([]byte(out), &token); err != nil {
		t.Fatalf("decode issue response: %v\noutput=%s", err, out)
	}
	return token
}

func decodeEnrollmentTokenRecord(t *testing.T, out string) enrollmentTokenRecord {
	t.Helper()
	var token enrollmentTokenRecord
	if err := json.Unmarshal([]byte(out), &token); err != nil {
		t.Fatalf("decode token response: %v\noutput=%s", err, out)
	}
	return token
}

func decodeJSONMap(t *testing.T, out string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode json response: %v\noutput=%s", err, out)
	}
	return payload
}

func mustNestedMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", value)
	}
	return m
}

func deleteEnrollmentTokenByID(t *testing.T, id string) {
	t.Helper()
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	sql := fmt.Sprintf("DELETE FROM enrollment_tokens WHERE id = '%s';", id)
	cmd := exec.Command("docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "xmdm", "-d", "xmdm", "-v", "ON_ERROR_STOP=1", "-c", sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup enrollment token: %v\n%s", err, out)
	}
}

package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/session"
)

func TestRunResourceListAndShowAgainstLiveServer(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	t.Setenv("XMDM_SESSION_FILE", sessionPath)

	loginOut := runCLI(t, []string{"--config", "../../config.yaml", "auth", "login", "--username", "admin", "--password", "admin"}, "1.2.3")
	if !strings.Contains(loginOut.stdout, "logged in as admin") {
		t.Fatalf("unexpected login output: %s", loginOut.stdout)
	}

	resolved, err := config.Resolve(config.Options{ConfigPath: "../../config.yaml"})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	state, err := session.Load(sessionPath)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if !state.IsValid() {
		t.Fatalf("expected valid session state: %#v", state)
	}
	if strings.TrimSpace(state.BaseURL) != strings.TrimSpace(resolved.BaseURL) {
		t.Fatalf("session base url mismatch: %q vs %q", state.BaseURL, resolved.BaseURL)
	}

	seed := seedLiveResources(t)

	cases := []struct {
		name   string
		args   []string
		want   string
		itemID string
	}{
		{name: "users", args: []string{"users", "list"}, want: seed.userEmail, itemID: seed.userID},
		{name: "roles", args: []string{"roles", "list"}, want: seed.roleName, itemID: seed.roleID},
		{name: "groups", args: []string{"groups", "list"}, want: seed.groupName, itemID: seed.groupID},
		{name: "policies", args: []string{"policies", "list"}, want: seed.policyName, itemID: seed.policyID},
		{name: "apps", args: []string{"apps", "list"}, want: seed.appName, itemID: seed.appID},
		{name: "files", args: []string{"files", "list"}, want: seed.fileName, itemID: seed.fileID},
		{name: "managed-files", args: []string{"managed-files", "list"}, want: seed.managedFilePath, itemID: seed.managedFileID},
		{name: "certificates", args: []string{"certificates", "list"}, want: seed.certificateName, itemID: seed.certificateID},
		{name: "devices", args: []string{"devices", "list"}, want: seed.deviceName, itemID: seed.deviceID},
		{name: "commands", args: []string{"commands", "list"}, want: seed.commandMarker, itemID: seed.commandID},
		{name: "logs", args: []string{"logs", "list"}, want: seed.logMessage, itemID: seed.logID},
		{name: "device-info", args: []string{"device-info", "list"}, want: seed.deviceInfoModel, itemID: seed.deviceInfoID},
		{name: "audit", args: []string{"audit", "list"}, want: seed.auditEventID, itemID: seed.auditEventID},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := runCLI(t, append([]string{"--config", "../../config.yaml"}, tc.args...), "1.2.3").stdout
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("list output missing %q:\n%s", tc.want, stdout)
			}

			listID := extractIDFromListOutput(t, stdout, tc.want)
			if tc.itemID != "" && listID != tc.itemID {
				t.Fatalf("unexpected list id: got %q want %q", listID, tc.itemID)
			}

			showOut := runCLI(t, []string{"--config", "../../config.yaml", tc.name, "show", listID}, "1.2.3").stdout
			if !strings.Contains(showOut, listID) {
				t.Fatalf("show output missing id %q:\n%s", listID, showOut)
			}
			if !strings.Contains(showOut, tc.want) {
				t.Fatalf("show output missing %q:\n%s", tc.want, showOut)
			}
		})
	}
}

type cliRunResult struct {
	stdout string
	stderr string
	code   int
}

func runCLI(t *testing.T, args []string, version string) cliRunResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr, version)
	if code != 0 {
		t.Fatalf("cli failed: code=%d stderr=%s", code, stderr.String())
	}
	return cliRunResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

type seededResource struct {
	userID             string
	userEmail          string
	roleID             string
	roleName           string
	groupID            string
	groupName          string
	policyID           string
	policyName         string
	appID              string
	appName            string
	fileID             string
	fileName           string
	managedFileID      string
	managedFilePath    string
	certificateID      string
	certificateName    string
	deviceID           string
	deviceName         string
	deviceSecret       string
	commandID          string
	commandMarker      string
	logID              string
	logMessage         string
	deviceInfoID       string
	deviceInfoModel    string
	auditEventID       string
	deviceAuditEventID string
}

func seedLiveResources(t *testing.T) seededResource {
	t.Helper()

	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	const (
		tenantID           = "00000000-0000-0000-0000-000000000000"
		roleID             = "22222222-2222-2222-2222-222222222222"
		userID             = "33333333-3333-3333-3333-333333333333"
		groupID            = "44444444-4444-4444-4444-444444444444"
		policyID           = "55555555-5555-5555-5555-555555555555"
		appID              = "66666666-6666-6666-6666-666666666666"
		artifactID         = "77777777-7777-7777-7777-777777777777"
		fileID             = "88888888-8888-8888-8888-888888888888"
		managedFileID      = "99999999-9999-9999-9999-999999999999"
		certID             = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		certArtifact       = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		deviceID           = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		commandID          = "dddddddd-dddd-dddd-dddd-dddddddddddd"
		logID              = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
		deviceInfoID       = "ffffffff-ffff-ffff-ffff-ffffffffffff"
		auditEventID       = "12121212-1212-1212-1212-121212121212"
		deviceAuditEventID = "34343434-3434-3434-3434-343434343434"
	)

	deviceSecret := "secret-" + nonce
	roleName := "cli-role-" + nonce
	userEmail := "cli-user-" + nonce + "@example.com"
	groupName := "cli-group-" + nonce
	policyName := "cli-policy-" + nonce
	appName := "cli-app-" + nonce
	fileName := "cli-file-" + nonce + ".txt"
	certificateName := "cli-cert-" + nonce + ".pem"
	deviceName := "cli-device-" + nonce
	commandMarker := nonce
	logMessage := "cli-log-" + nonce
	deviceInfoModel := "cli-model-" + nonce
	managedFilePath := "/data/cli-file-" + nonce + ".txt"

	sql := fmt.Sprintf(`
DELETE FROM device_info WHERE id = '%[27]s';
DELETE FROM device_logs WHERE id = '%[25]s';
DELETE FROM commands WHERE id = '%[24]s';
DELETE FROM managed_files WHERE id = '%[16]s';
DELETE FROM certificates WHERE id = '%[20]s';
DELETE FROM files WHERE id = '%[15]s';
DELETE FROM artifacts WHERE id IN ('%[13]s', '%[18]s');
DELETE FROM users WHERE id = '%[4]s';
DELETE FROM devices WHERE id = '%[21]s';
DELETE FROM groups WHERE id = '%[7]s';
DELETE FROM policies WHERE id = '%[9]s';
DELETE FROM apps WHERE id = '%[11]s';
DELETE FROM roles WHERE id = '%[2]s';
DELETE FROM audit_events WHERE id = '%[29]s';
DELETE FROM audit_events WHERE id = '%[30]s';

INSERT INTO tenants (id, name, status, created_at, updated_at)
VALUES ('%[1]s', 'Default tenant', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO roles (id, tenant_id, name, permissions, status, created_at, updated_at)
VALUES ('%[2]s', '%[1]s', '%[3]s', '["admin.read","admin.write","devices.read","devices.write"]'::jsonb, 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    permissions = EXCLUDED.permissions,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO users (id, tenant_id, email, password_hash, role_id, status, created_at, updated_at)
VALUES ('%[4]s', '%[1]s', '%[5]s', 'hash-%[6]s', '%[2]s', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    email = EXCLUDED.email,
    password_hash = EXCLUDED.password_hash,
    role_id = EXCLUDED.role_id,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO groups (id, tenant_id, name, status, created_at, updated_at)
VALUES ('%[7]s', '%[1]s', '%[8]s', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO policies (id, tenant_id, name, version, kiosk_mode, restrictions_json, status, created_at, updated_at)
VALUES ('%[9]s', '%[1]s', '%[10]s', 1, false, '{}'::jsonb, 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    version = EXCLUDED.version,
    kiosk_mode = EXCLUDED.kiosk_mode,
    restrictions_json = EXCLUDED.restrictions_json,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO apps (id, tenant_id, package_name, name, status, created_at, updated_at)
VALUES ('%[11]s', '%[1]s', 'com.example.cli.%[6]s', '%[12]s', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    package_name = EXCLUDED.package_name,
    name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO artifacts (id, tenant_id, storage_key, checksum, size_bytes, mime_type, status, created_at, updated_at)
VALUES ('%[13]s', '%[1]s', 'artifacts/%[14]s', 'checksum-file-%[6]s', 12, 'text/plain', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    storage_key = EXCLUDED.storage_key,
    checksum = EXCLUDED.checksum,
    size_bytes = EXCLUDED.size_bytes,
    mime_type = EXCLUDED.mime_type,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO files (id, tenant_id, name, artifact_id, checksum, mime_type, status, created_at, updated_at)
VALUES ('%[15]s', '%[1]s', '%[14]s', '%[13]s', 'checksum-file-%[6]s', 'text/plain', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    artifact_id = EXCLUDED.artifact_id,
    checksum = EXCLUDED.checksum,
    mime_type = EXCLUDED.mime_type,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO managed_files (id, tenant_id, file_id, path, replace_variables, status, created_at, updated_at)
VALUES ('%[16]s', '%[1]s', '%[15]s', '%[17]s', true, 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    file_id = EXCLUDED.file_id,
    path = EXCLUDED.path,
    replace_variables = EXCLUDED.replace_variables,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO artifacts (id, tenant_id, storage_key, checksum, size_bytes, mime_type, status, created_at, updated_at)
VALUES ('%[18]s', '%[1]s', 'artifacts/%[19]s', 'checksum-cert-%[6]s', 16, 'application/x-pem-file', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    storage_key = EXCLUDED.storage_key,
    checksum = EXCLUDED.checksum,
    size_bytes = EXCLUDED.size_bytes,
    mime_type = EXCLUDED.mime_type,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO certificates (id, tenant_id, name, artifact_id, checksum, status, created_at, updated_at)
VALUES ('%[20]s', '%[1]s', '%[19]s', '%[18]s', 'checksum-cert-%[6]s', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    artifact_id = EXCLUDED.artifact_id,
    checksum = EXCLUDED.checksum,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO devices (id, tenant_id, device_id, secret_hash, status, policy_id, created_at, updated_at)
VALUES ('%[21]s', '%[1]s', '%[22]s', '%[23]s', 'active', '%[9]s', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    device_id = EXCLUDED.device_id,
    secret_hash = EXCLUDED.secret_hash,
    status = EXCLUDED.status,
    policy_id = EXCLUDED.policy_id,
    updated_at = now();

INSERT INTO commands (id, tenant_id, device_id, type, payload_json, status, created_at, updated_at)
VALUES ('%[24]s', '%[1]s', '%[21]s', 'ping', '{"marker":"%[6]s"}'::jsonb, 'queued', now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    device_id = EXCLUDED.device_id,
    type = EXCLUDED.type,
    payload_json = EXCLUDED.payload_json,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO device_logs (id, tenant_id, device_id, observed_at, source, level, message, payload_json, created_at, updated_at)
VALUES ('%[25]s', '%[1]s', '%[21]s', now(), 'cli-test', 'info', '%[26]s', '{}'::jsonb, now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    device_id = EXCLUDED.device_id,
    observed_at = EXCLUDED.observed_at,
    source = EXCLUDED.source,
    level = EXCLUDED.level,
    message = EXCLUDED.message,
    payload_json = EXCLUDED.payload_json,
    updated_at = now();

INSERT INTO device_info (id, tenant_id, device_id, observed_at, payload_json, created_at, updated_at)
VALUES ('%[27]s', '%[1]s', '%[21]s', now(), '{"model":"%[28]s"}'::jsonb, now(), now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    device_id = EXCLUDED.device_id,
    observed_at = EXCLUDED.observed_at,
    payload_json = EXCLUDED.payload_json,
    updated_at = now();

INSERT INTO audit_events (id, tenant_id, actor, action, resource_type, resource_id, details, created_at)
VALUES ('%[29]s', '%[1]s', 'admin', 'create', 'users', '%[4]s', '{"email":"%[5]s"}'::jsonb, now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    actor = EXCLUDED.actor,
    action = EXCLUDED.action,
    resource_type = EXCLUDED.resource_type,
    resource_id = EXCLUDED.resource_id,
    details = EXCLUDED.details,
    created_at = now();

INSERT INTO audit_events (id, tenant_id, actor, action, resource_type, resource_id, details, created_at)
VALUES ('%[30]s', '%[1]s', 'admin', 'update', 'devices', '%[21]s', '{"name":"%[22]s"}'::jsonb, now())
ON CONFLICT (id) DO UPDATE
SET tenant_id = EXCLUDED.tenant_id,
    actor = EXCLUDED.actor,
    action = EXCLUDED.action,
    resource_type = EXCLUDED.resource_type,
    resource_id = EXCLUDED.resource_id,
    details = EXCLUDED.details,
    created_at = now();
`, tenantID, roleID, roleName, userID, userEmail, nonce, groupID, groupName, policyID, policyName, appID, appName, artifactID, fileName, fileID, managedFileID, managedFilePath, certArtifact, certificateName, certID, deviceID, deviceName, hashToken(deviceSecret), commandID, logID, logMessage, deviceInfoID, deviceInfoModel, auditEventID, deviceAuditEventID)

	cmd := exec.Command("docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "xmdm", "-d", "xmdm", "-v", "ON_ERROR_STOP=1", "-c", sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed live database: %v\n%s", err, out)
	}

	return seededResource{
		userID:             userID,
		userEmail:          userEmail,
		roleID:             roleID,
		roleName:           roleName,
		groupID:            groupID,
		groupName:          groupName,
		policyID:           policyID,
		policyName:         policyName,
		appID:              appID,
		appName:            appName,
		fileID:             fileID,
		fileName:           fileName,
		managedFileID:      managedFileID,
		managedFilePath:    managedFilePath,
		certificateID:      certID,
		certificateName:    certificateName,
		deviceID:           deviceID,
		deviceName:         deviceName,
		deviceSecret:       deviceSecret,
		commandID:          commandID,
		commandMarker:      commandMarker,
		logID:              logID,
		logMessage:         logMessage,
		deviceInfoID:       deviceInfoID,
		deviceInfoModel:    deviceInfoModel,
		auditEventID:       auditEventID,
		deviceAuditEventID: deviceAuditEventID,
	}
}

func extractIDFromListOutput(t *testing.T, stdout, want string) string {
	t.Helper()
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode list output: %v\n%s", err, stdout)
	}
	for _, raw := range envelope.Items {
		if strings.Contains(string(raw), want) {
			var item map[string]any
			if err := json.Unmarshal(raw, &item); err != nil {
				t.Fatalf("decode matched item: %v", err)
			}
			if id, ok := item["id"].(string); ok {
				return id
			}
		}
	}
	t.Fatalf("could not find item containing %q in %s", want, stdout)
	return ""
}

func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

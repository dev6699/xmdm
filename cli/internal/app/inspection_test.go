package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRunDeviceInspectAgainstLiveServer(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	t.Setenv("XMDM_SESSION_FILE", sessionPath)

	loginLiveAdmin(t)
	seed := seedLiveResources(t)

	out := runCLI(t, []string{
		"--config", "../../config.yaml",
		"devices", "inspect", seed.deviceID,
		"--device-secret", seed.deviceSecret,
		"--limit", "3",
	}, "1.2.3").stdout

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode inspect output: %v\noutput=%s", err, out)
	}

	device := decodeRawMap(t, payload["device"])
	if got := device["name"]; got != seed.deviceName {
		t.Fatalf("unexpected device name: %#v", got)
	}
	if got := device["status"]; got != "active" {
		t.Fatalf("unexpected device status: %#v", got)
	}

	if raw, ok := payload["config"]; ok {
		config := decodeRawMap(t, raw)
		deviceConfig := decodeAnyMap(t, config["device"])
		if got := deviceConfig["deviceId"]; got != seed.deviceName {
			t.Fatalf("unexpected config device id: %#v", got)
		}
	}
	if raw, ok := payload["configError"]; ok && len(raw) > 0 {
		t.Logf("config snapshot unavailable: %s", string(raw))
	}

	logs := decodeRawSlice(t, payload["logs"])
	if len(logs) == 0 {
		t.Fatalf("expected logs section")
	}
	if got := decodeRawMap(t, logs[0])["message"]; got != seed.logMessage {
		t.Fatalf("unexpected log message: %#v", got)
	}

	info := decodeRawSlice(t, payload["deviceInfo"])
	if len(info) == 0 {
		t.Fatalf("expected device-info section")
	}
	info0 := decodeRawMap(t, info[0])
	if got := decodeAnyMap(t, info0["payload"])["model"]; got != seed.deviceInfoModel {
		t.Fatalf("unexpected device-info model: %#v", got)
	}

	commands := decodeRawSlice(t, payload["commands"])
	if len(commands) == 0 {
		t.Fatalf("expected commands section")
	}
	command0 := decodeRawMap(t, commands[0])
	if got := decodeAnyMap(t, command0["payload"])["marker"]; got != seed.commandMarker {
		t.Fatalf("unexpected command payload: %#v", command0["payload"])
	}

	audit := decodeRawSlice(t, payload["audit"])
	if len(audit) == 0 {
		t.Fatalf("expected audit section")
	}
	if got := decodeRawMap(t, audit[0])["resourceId"]; got != seed.deviceID {
		t.Fatalf("unexpected audit resource id: %#v", got)
	}
	if got := decodeRawMap(t, audit[0])["id"]; got != seed.deviceAuditEventID {
		t.Fatalf("unexpected audit id: %#v", got)
	}
}

func decodeRawMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode object: %v\nraw=%s", err, string(raw))
	}
	return payload
}

func decodeRawSlice(t *testing.T, raw json.RawMessage) []json.RawMessage {
	t.Helper()
	var payload []json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode array: %v\nraw=%s", err, string(raw))
	}
	return payload
}

func decodeAnyMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode object: %v\nraw=%s", err, string(raw))
	}
	return payload
}

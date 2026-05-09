package app

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCommandOperationsAgainstLiveServer(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	t.Setenv("XMDM_SESSION_FILE", sessionPath)

	loginLiveAdmin(t)
	seed := seedLiveResources(t)

	sendOut := runCLI(t, []string{
		"--config", "../../config.yaml",
		"commands", "send",
		"--json", `{"type":"reboot","payload":{"force":true},"target":{"type":"device","deviceId":"` + seed.deviceName + `"}}`,
	}, "1.2.3").stdout
	commandID := decodeSentCommandID(t, sendOut)
	if commandID == "" {
		t.Fatalf("missing command id in send output: %s", sendOut)
	}
	t.Cleanup(func() {
		deleteCommandByID(t, commandID)
	})

	showOut := runCLI(t, []string{"--config", "../../config.yaml", "commands", "show", commandID}, "1.2.3").stdout
	showBefore := decodeCommandEnvelope(t, showOut)
	if got := showBefore["id"]; got != commandID {
		t.Fatalf("unexpected command id before ack: %#v", got)
	}
	if got := showBefore["type"]; got != "reboot" {
		t.Fatalf("unexpected command type before ack: %#v", got)
	}

	ackOut := runCLI(t, []string{
		"--config", "../../config.yaml",
		"commands", "ack", seed.deviceName, commandID,
		"--device-secret", seed.deviceSecret,
		"--status", "acked",
		"--message", "done",
		"--details", `{"transport":"polling"}`,
	}, "1.2.3").stdout
	acked := decodeCommandEnvelope(t, ackOut)
	if got := acked["id"]; got != commandID {
		t.Fatalf("unexpected acked command id: %#v", got)
	}
	if got := acked["status"]; got != "acked" {
		t.Fatalf("unexpected ack status: %#v", got)
	}
	if got := acked["ackedAt"]; got == nil {
		t.Fatalf("expected ackedAt in acked response: %#v", acked)
	}

	ackedShowOut := runCLI(t, []string{"--config", "../../config.yaml", "commands", "show", commandID}, "1.2.3").stdout
	ackedShow := decodeCommandEnvelope(t, ackedShowOut)
	if got := ackedShow["status"]; got != "acked" {
		t.Fatalf("unexpected command status after ack: %#v", got)
	}
	if got := ackedShow["ackedAt"]; got == nil {
		t.Fatalf("expected ackedAt after ack: %#v", ackedShow)
	}
	if got := decodeAnyMap(t, ackedShow["result"])["status"]; got != "acked" {
		t.Fatalf("unexpected result status: %#v", got)
	}
	if got := decodeAnyMap(t, ackedShow["result"])["message"]; got != "done" {
		t.Fatalf("unexpected result message: %#v", got)
	}

	listOut := runCLI(t, []string{"--config", "../../config.yaml", "commands", "list", "--status", "acked", "--type", "reboot", "--limit", "5"}, "1.2.3").stdout
	if !strings.Contains(listOut, commandID) {
		t.Fatalf("list output missing command id:\n%s", listOut)
	}
}

func decodeSentCommandID(t *testing.T, out string) string {
	t.Helper()
	var envelope struct {
		Data struct {
			Item struct {
				Commands []map[string]any `json:"commands"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode send output: %v\noutput=%s", err, out)
	}
	if len(envelope.Data.Item.Commands) == 0 {
		t.Fatalf("expected at least one command in send output: %s", out)
	}
	id, _ := envelope.Data.Item.Commands[0]["id"].(string)
	return id
}

func decodeCommandEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope struct {
		Data struct {
			Item json.RawMessage `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode command envelope: %v\noutput=%s", err, out)
	}
	var payload map[string]any
	if err := json.Unmarshal(envelope.Data.Item, &payload); err != nil {
		t.Fatalf("decode command item: %v\noutput=%s", err, out)
	}
	return payload
}

func deleteCommandByID(t *testing.T, id string) {
	t.Helper()
	if strings.TrimSpace(id) == "" {
		return
	}
	sql := fmt.Sprintf("DELETE FROM commands WHERE id = '%s';", id)
	cmd := exec.Command("docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "xmdm", "-d", "xmdm", "-v", "ON_ERROR_STOP=1", "-c", sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup command: %v\n%s", err, out)
	}
}

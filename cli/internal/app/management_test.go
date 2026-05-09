package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRunCoreResourceManagementAgainstLiveServer(t *testing.T) {
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())

	roleName := "cli-role-" + nonce
	userEmail := "cli-user-" + nonce + "@example.com"
	groupName := "cli-group-" + nonce
	policyName := "cli-policy-" + nonce
	deviceName := "cli-device-" + nonce

	roleID, _ := createManagedResource(t, "roles", `{"name":"`+roleName+`","permissions":["admin.read","devices.read"]}`)
	t.Cleanup(func() {
		retireManagedResource(t, "roles", roleID)
	})
	roleOut := updateManagedResource(t, "roles", roleID, `{"name":"`+roleName+`-updated","permissions":["admin.read","admin.write","devices.read"]}`)
	assertEnvelopeFieldContains(t, roleOut, "name", roleName+"-updated")
	roleOut = updateManagedResource(t, "roles", roleID, `{"name":"`+roleName+`-updated2","permissions":["admin.read","devices.read"]}`)
	assertEnvelopeFieldContains(t, roleOut, "name", roleName+"-updated2")

	userCreate := `{"email":"` + userEmail + `","passwordHash":"hash-` + nonce + `","roleId":"` + roleID + `"}`
	userID, _ := createManagedResource(t, "users", userCreate)
	updateManagedResource(t, "users", userID, `{"email":"`+strings.ReplaceAll(userEmail, "@", "+updated@")+`","passwordHash":"hash-`+nonce+`-updated","roleId":"`+roleID+`"}`)
	retireManagedResource(t, "users", userID)

	groupID, _ := createManagedResource(t, "groups", `{"name":"`+groupName+`"}`)
	groupOut := updateManagedResource(t, "groups", groupID, `{"name":"`+groupName+`-updated"}`)
	assertEnvelopeFieldContains(t, groupOut, "name", groupName+"-updated")
	retireManagedResource(t, "groups", groupID)

	policyID, _ := createManagedResource(t, "policies", `{"name":"`+policyName+`","version":1,"kioskMode":false,"restrictions":null}`)
	policyOut := updateManagedResource(t, "policies", policyID, `{"name":"`+policyName+`-updated","version":2,"kioskMode":false,"restrictions":null}`)
	assertEnvelopeFieldContains(t, policyOut, "name", policyName+"-updated")
	retireManagedResource(t, "policies", policyID)

	deviceID, _ := createManagedResource(t, "devices", `{"name":"`+deviceName+`","secretHash":"secret-`+nonce+`"}`)
	deviceOut := updateManagedResource(t, "devices", deviceID, `{"name":"`+deviceName+`-updated","secretHash":"secret-`+nonce+`-updated"}`)
	assertEnvelopeFieldContains(t, deviceOut, "name", deviceName+"-updated")
	retireManagedResource(t, "devices", deviceID)
}

func createManagedResource(t *testing.T, resource, body string) (string, string) {
	t.Helper()
	out := runCLI(t, []string{"--config", "../../config.yaml", resource, "create", "--json", body}, "1.2.3").stdout
	return envelopeID(t, out), out
}

func updateManagedResource(t *testing.T, resource, id, body string) string {
	t.Helper()
	return runCLI(t, []string{"--config", "../../config.yaml", resource, "update", id, "--json", body}, "1.2.3").stdout
}

func retireManagedResource(t *testing.T, resource, id string) string {
	t.Helper()
	out := runCLI(t, []string{"--config", "../../config.yaml", resource, "retire", id}, "1.2.3").stdout
	return envelopeID(t, out)
}

func envelopeID(t *testing.T, out string) string {
	t.Helper()
	var envelope struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v\noutput=%s", err, out)
	}
	var payload struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(envelope.Item, &payload); err != nil {
		t.Fatalf("unmarshal item: %v\noutput=%s", err, out)
	}
	if payload.ID == "" {
		t.Fatalf("missing id in output: %s", out)
	}
	return payload.ID
}

func assertEnvelopeFieldContains(t *testing.T, out, field, want string) {
	t.Helper()
	var envelope struct {
		Item map[string]any `json:"item"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v\noutput=%s", err, out)
	}
	raw, ok := envelope.Item[field]
	if !ok {
		t.Fatalf("missing %q in output: %s", field, out)
	}
	if got := fmt.Sprint(raw); !strings.Contains(got, want) {
		t.Fatalf("unexpected %s: got %q want contains %q", field, got, want)
	}
}

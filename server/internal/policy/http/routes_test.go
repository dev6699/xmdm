package policyhttp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"xmdm/server/internal/httpx"
)

func TestDecodePolicyRequestRequiresKioskExitPasscodeForKioskMode(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/policies", strings.NewReader(`{"name":"policy","kioskMode":true,"restrictions":{}}`))
	req.Header.Set("Content-Type", "application/json")

	if _, err := decodePolicyRequest(req); err != httpx.ErrInvalidInput {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestDecodePolicyRequestAllowsKioskModeWithPasscode(t *testing.T) {
	req := httptest.NewRequest(
		"POST",
		"/api/v1/policies",
		strings.NewReader(`{"name":"policy","kioskMode":true,"restrictions":{"kioskExitPasscode":"1234"}}`),
	)
	req.Header.Set("Content-Type", "application/json")

	payload, err := decodePolicyRequest(req)
	if err != nil {
		t.Fatalf("decode policy request: %v", err)
	}
	if !payload.KioskMode {
		t.Fatalf("expected kiosk mode to remain enabled")
	}
}

func TestDecodePolicyRequestRejectsHashOnlyKioskPasscode(t *testing.T) {
	req := httptest.NewRequest(
		"POST",
		"/api/v1/policies",
		strings.NewReader(`{"name":"policy","kioskMode":true,"restrictions":{"kioskExitPasscodeHash":"abcd"}}`),
	)
	req.Header.Set("Content-Type", "application/json")

	if _, err := decodePolicyRequest(req); err != httpx.ErrInvalidInput {
		t.Fatalf("expected invalid input for hash-only kiosk passcode, got %v", err)
	}
}

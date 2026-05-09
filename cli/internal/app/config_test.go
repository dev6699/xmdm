package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunConfigValidateRequiresTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"config", "validate"}, &stdout, &stderr, "1.2.3")
	if code == 0 {
		t.Fatalf("expected failure")
	}
	if !strings.Contains(stderr.String(), "profile or --base-url") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

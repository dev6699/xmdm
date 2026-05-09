package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsVersionedHelpTree(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{}, &stdout, &stderr, "1.2.3")
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}

	out := stdout.String()
	for _, want := range []string{"xmdm 1.2.3", "config", "validate", "--base-url"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

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

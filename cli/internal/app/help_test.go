package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsVersionedHelpTree(t *testing.T) {
	var liveStdout, liveStderr bytes.Buffer
	code := Run([]string{"--config", "../../config.yaml", "config", "validate"}, &liveStdout, &liveStderr, "1.2.3")
	if code != 0 {
		t.Fatalf("live preflight failed: code=%d stderr=%s", code, liveStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{}, &stdout, &stderr, "1.2.3")
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}

	out := stdout.String()
	for _, want := range []string{"xmdm 1.2.3", "auth", "login", "whoami", "logout", "config", "users", "create", "update", "retire", "managed-files", "commands", "device-info", "audit", "--base-url"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

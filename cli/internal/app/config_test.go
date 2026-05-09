package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunConfigValidateAgainstLiveServer(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--config", "../../config.yaml", "config", "validate"}, &stdout, &stderr, "1.2.3")
	if code != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "target reachable:") {
		t.Fatalf("unexpected output: %s", out)
	}
}

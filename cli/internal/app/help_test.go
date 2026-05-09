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
	for _, want := range []string{"xmdm 1.2.3", "auth", "login", "whoami", "logout", "config", "--base-url"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

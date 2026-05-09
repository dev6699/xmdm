package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAuthLoginWhoamiLogout(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	t.Setenv("XMDM_SESSION_FILE", sessionPath)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--config", "../../config.yaml", "auth", "login", "--username", "admin", "--password", "admin"}, &stdout, &stderr, "1.2.3")
	if code != 0 {
		t.Fatalf("login exit code: %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged in as admin") {
		t.Fatalf("unexpected login output: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"auth", "whoami"}, &stdout, &stderr, "1.2.3")
	if code != 0 {
		t.Fatalf("whoami exit code: %d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "session user: admin") {
		t.Fatalf("unexpected whoami output: %s", out)
	}
	if !strings.Contains(out, "http://127.0.0.1:8080/api/v1") {
		t.Fatalf("whoami output missing target: %s", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"auth", "logout"}, &stdout, &stderr, "1.2.3")
	if code != 0 {
		t.Fatalf("logout exit code: %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged out") {
		t.Fatalf("unexpected logout output: %s", stdout.String())
	}
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("expected session file removed, stat err=%v", err)
	}
}

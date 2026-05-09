package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunContentManagementAgainstLiveServer(t *testing.T) {
	seed := seedLiveResources(t)
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	sourcePath := filepath.Join(t.TempDir(), "artifact.bin")
	sourceContent := []byte("cli-content-" + fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.WriteFile(sourcePath, sourceContent, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	fileCreateOut := runCLI(t, []string{
		"--config", "../../config.yaml",
		"files", "create",
		"--name", "cli-file-" + nonce + "-" + filepath.Base(sourcePath),
		"--storage-key", "cli/files/" + nonce + "-" + filepath.Base(sourcePath),
		"--source", sourcePath,
		"--mime-type", "application/octet-stream",
	}, "1.2.3").stdout
	fileRec := decodeContentItem[struct {
		ID         string `json:"id"`
		ArtifactID string `json:"artifactId"`
		Checksum   string `json:"checksum"`
	}](t, fileCreateOut)
	if fileRec.ID == "" || fileRec.ArtifactID == "" || fileRec.Checksum == "" {
		t.Fatalf("unexpected file create output: %s", fileCreateOut)
	}
	t.Cleanup(func() {
		_ = runCLI(t, []string{"--config", "../../config.yaml", "files", "retire", fileRec.ID}, "1.2.3")
	})

	certCreateOut := runCLI(t, []string{
		"--config", "../../config.yaml",
		"certificates", "create",
		"--name", "cli-cert-" + nonce + "-" + filepath.Base(sourcePath),
		"--storage-key", "cli/certs/" + nonce + "-" + filepath.Base(sourcePath),
		"--source", sourcePath,
		"--mime-type", "application/x-pem-file",
	}, "1.2.3").stdout
	certRec := decodeContentItem[struct {
		ID       string `json:"id"`
		Checksum string `json:"checksum"`
	}](t, certCreateOut)
	if certRec.ID == "" || certRec.Checksum == "" {
		t.Fatalf("unexpected certificate create output: %s", certCreateOut)
	}
	t.Cleanup(func() {
		_ = runCLI(t, []string{"--config", "../../config.yaml", "certificates", "retire", certRec.ID}, "1.2.3")
	})

	managedOut := runCLI(t, []string{
		"--config", "../../config.yaml",
		"managed-files", "create",
		"--file-id", fileRec.ID,
		"--path", "/etc/cli/" + nonce + "-" + filepath.Base(sourcePath),
	}, "1.2.3").stdout
	managedRec := decodeContentItem[struct {
		ID     string `json:"id"`
		FileID string `json:"fileId"`
		Path   string `json:"path"`
	}](t, managedOut)
	if managedRec.ID == "" || managedRec.FileID == "" || managedRec.Path == "" {
		t.Fatalf("unexpected managed file output: %s", managedOut)
	}
	t.Cleanup(func() {
		_ = runCLI(t, []string{"--config", "../../config.yaml", "managed-files", "retire", managedRec.ID}, "1.2.3")
	})

	versionOut := runCLI(t, []string{
		"--config", "../../config.yaml",
		"apps", "versions", "publish", seed.appID,
		"--version-name", "1.0.0-" + nonce,
		"--version-code", "100",
		"--artifact-id", fileRec.ArtifactID,
		"--checksum", fileRec.Checksum,
	}, "1.2.3").stdout
	versionRec := decodeContentItem[struct {
		ID         string  `json:"id"`
		ArtifactID *string `json:"artifactId"`
		Checksum   string  `json:"checksum"`
		Status     string  `json:"status"`
	}](t, versionOut)
	if versionRec.ID == "" || versionRec.ArtifactID == nil || versionRec.Checksum == "" {
		t.Fatalf("unexpected app version output: %s", versionOut)
	}
	if versionRec.Status != "published" {
		t.Fatalf("expected published version, got %q", versionRec.Status)
	}
	t.Cleanup(func() {
		deleteAppVersionByID(t, versionRec.ID)
	})
}

func decodeContentItem[T any](t *testing.T, out string) T {
	t.Helper()
	var envelope struct {
		Item json.RawMessage `json:"item"`
	}
	var zero T
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v\noutput=%s", err, out)
	}
	if err := json.Unmarshal(envelope.Item, &zero); err != nil {
		t.Fatalf("unmarshal item: %v\noutput=%s", err, out)
	}
	return zero
}

func deleteAppVersionByID(t *testing.T, id string) {
	t.Helper()
	if strings.TrimSpace(id) == "" {
		return
	}
	sql := fmt.Sprintf("DELETE FROM app_versions WHERE id = '%s';", id)
	cmd := exec.Command("docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "xmdm", "-d", "xmdm", "-v", "ON_ERROR_STOP=1", "-c", sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup app version: %v\n%s", err, out)
	}
}

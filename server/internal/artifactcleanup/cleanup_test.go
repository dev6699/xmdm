package artifactcleanup

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	files "xmdm/server/internal/files"
)

func TestRunDryRunListsCandidatesWithoutMutating(t *testing.T) {
	repo := &fakeRepository{
		candidates: []files.Artifact{artifact("a-1", "artifacts/a-1.apk", files.StatusActive)},
	}
	store := &fakeObjectStore{}

	result, err := Run(context.Background(), repo, store, "tenant-1", false)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(result.Candidates))
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("expected no deletions in dry run, got %d", len(result.Deleted))
	}
	if repo.retireCalls != 0 || repo.deleteCalls != 0 || store.deleteCalls != 0 {
		t.Fatalf("expected no mutations in dry run: repo=%d/%d store=%d", repo.retireCalls, repo.deleteCalls, store.deleteCalls)
	}
}

func TestRunAppliesCleanupForActiveOrphan(t *testing.T) {
	repo := &fakeRepository{
		candidates: []files.Artifact{artifact("a-1", "artifacts/a-1.apk", files.StatusActive)},
		retired:    artifact("a-1", "artifacts/a-1.apk", files.StatusRetired),
	}
	store := &fakeObjectStore{}

	result, err := Run(context.Background(), repo, store, "tenant-1", true)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Retired) != 1 {
		t.Fatalf("expected 1 retired artifact, got %d", len(result.Retired))
	}
	if len(result.Deleted) != 1 {
		t.Fatalf("expected 1 deleted artifact, got %d", len(result.Deleted))
	}
	if repo.retireCalls != 1 || repo.deleteCalls != 1 {
		t.Fatalf("expected retire and delete calls, got retire=%d delete=%d", repo.retireCalls, repo.deleteCalls)
	}
	if store.deleteCalls != 1 || store.deletedKey != "artifacts/a-1.apk" {
		t.Fatalf("unexpected object store calls: calls=%d key=%q", store.deleteCalls, store.deletedKey)
	}
}

func TestRunAppliesCleanupForRetiredOrphan(t *testing.T) {
	repo := &fakeRepository{
		candidates: []files.Artifact{artifact("a-2", "artifacts/a-2.apk", files.StatusRetired)},
	}
	store := &fakeObjectStore{}

	result, err := Run(context.Background(), repo, store, "tenant-1", true)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Retired) != 0 {
		t.Fatalf("expected no retire call for already retired artifact, got %d", len(result.Retired))
	}
	if len(result.Deleted) != 1 {
		t.Fatalf("expected 1 deleted artifact, got %d", len(result.Deleted))
	}
	if repo.retireCalls != 0 || repo.deleteCalls != 1 {
		t.Fatalf("unexpected repo calls: retire=%d delete=%d", repo.retireCalls, repo.deleteCalls)
	}
}

type fakeRepository struct {
	candidates []files.Artifact
	retired    files.Artifact
	retireErr  error
	deleteErr  error

	retireCalls int
	deleteCalls int
}

func (r *fakeRepository) ListOrphanArtifacts(context.Context, string) ([]files.Artifact, error) {
	return r.candidates, nil
}

func (r *fakeRepository) RetireArtifact(context.Context, string, string) (files.Artifact, error) {
	r.retireCalls++
	if r.retireErr != nil {
		return files.Artifact{}, r.retireErr
	}
	return r.retired, nil
}

func (r *fakeRepository) DeleteArtifact(context.Context, string, string) error {
	r.deleteCalls++
	return r.deleteErr
}

type fakeObjectStore struct {
	deleteCalls int
	deletedKey  string
}

func (s *fakeObjectStore) Put(context.Context, string, io.Reader, string, int64) error {
	return nil
}

func (s *fakeObjectStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (s *fakeObjectStore) Delete(_ context.Context, key string) error {
	s.deleteCalls++
	s.deletedKey = key
	return nil
}

func artifact(id, key, status string) files.Artifact {
	now := time.Unix(1, 0)
	return files.Artifact{
		RecordBase: files.RecordBase{
			ID:        id,
			TenantID:  "tenant-1",
			Status:    status,
			UpdatedAt: now,
		},
		StorageKey: key,
		Checksum:   "checksum-" + id,
		SizeBytes:  123,
		MimeType:   "application/octet-stream",
	}
}

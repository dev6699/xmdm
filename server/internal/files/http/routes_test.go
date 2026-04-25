package filehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
)

func TestRegisterDeviceFileArtifactRouteRemoved(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeFileStore{}, nil, nil, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/files/file-1/artifact", nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected not found, got %d", res.Code)
	}
}

type fakeFileStore struct {
	file files.File
}

func (s *fakeFileStore) ListFiles(context.Context, string) ([]files.File, error) {
	return []files.File{s.file}, nil
}

func (s *fakeFileStore) GetFile(context.Context, string, string) (files.File, error) {
	return s.file, nil
}

func (s *fakeFileStore) CreateFile(context.Context, string, files.FileUpsert) (files.File, error) {
	return files.File{}, nil
}

func (s *fakeFileStore) RetireFile(context.Context, string, string) (files.File, error) {
	return files.File{}, nil
}

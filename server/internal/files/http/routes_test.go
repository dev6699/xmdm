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
	"xmdm/server/internal/pagination"
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

func TestRegisterFilesCollectionUsesQueryPagination(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	store := &fakeFileStore{}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, store, nil, nil, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?page=3&limit=7", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", res.Code, res.Body.String())
	}
	if store.lastParams.Limit != 7 {
		t.Fatalf("unexpected limit: %+v", store.lastParams)
	}
	if store.lastParams.Offset != 14 {
		t.Fatalf("unexpected offset: %+v", store.lastParams)
	}
}

type fakeFileStore struct {
	file       files.File
	lastParams pagination.Params
}

func (s *fakeFileStore) ListFiles(_ context.Context, _ string, params pagination.Params) ([]files.File, error) {
	s.lastParams = params
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

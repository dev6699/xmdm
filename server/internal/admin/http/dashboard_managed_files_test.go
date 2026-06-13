package adminhttp

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/files"
	"xmdm/server/internal/managedfiles"
	"xmdm/server/internal/pagination"
)

func TestManagedFileMutationsRecordAudit(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
	artifactStore := &recordingArtifactStore{}
	fileStore := &recordingFileStore{
		file: files.File{
			RecordBase: files.RecordBase{ID: "file-1", Status: files.StatusActive},
			Name:       "device-config.txt",
			ArtifactID: "artifact-1",
			Checksum:   checksum.SHA256Base64URL([]byte("managed-content")),
			MimeType:   "text/plain",
		},
	}
	managedStore := &recordingManagedFilesStore{
		managedFile: managedfiles.ManagedFile{
			RecordBase: managedfiles.RecordBase{ID: "managed-file-1", TenantID: "tenant-1", Status: managedfiles.StatusActive},
			FileID:     "file-1",
			Path:       "/sdcard/device-config.txt",
		},
	}
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Files:        fileStore,
		ManagedFiles: managedStore,
		Artifacts:    artifactStore,
		Audit:        auditStore,
		TenantID:     "tenant-1",
	})

	postMultipart := func(path string, fields map[string]string, fileField, fileName string, content []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for key, value := range fields {
			_ = writer.WriteField(key, value)
		}
		part, err := writer.CreateFormFile(fileField, fileName)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("write file content: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart writer: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, path, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	t.Run("create managed file", func(t *testing.T) {
		auditStore.records = nil
		rr := postMultipart("/admin/managed-files/create", map[string]string{
			"path":             "/sdcard/device-config.txt",
			"replaceVariables": "on",
			"csrfToken":        "token",
		}, "file", "device-config.txt", []byte("managed-content"))
		assertRedirect(t, rr, "/admin/managed-files?ok=managed+file+uploaded")
		assertAuditRecord(t, auditStore, "create", "managed_files", "managed-file-1")
	})

	t.Run("retire managed file", func(t *testing.T) {
		auditStore.records = nil
		req := httptest.NewRequest(http.MethodPost, "/admin/managed-files/managed-file-1/retire", strings.NewReader("csrfToken=token"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		assertRedirect(t, rr, "/admin/managed-files?ok=managed+file+retired")
		assertAuditRecord(t, auditStore, "retire", "managed_files", "managed-file-1")
	})
}

type recordingManagedFilesStore struct {
	managedFile managedfiles.ManagedFile
}

func (s *recordingManagedFilesStore) ListManagedFiles(context.Context, string, pagination.Params) ([]managedfiles.ManagedFile, error) {
	return []managedfiles.ManagedFile{s.managedFile}, nil
}

func (s *recordingManagedFilesStore) GetOverviewStats(context.Context, string) (managedfiles.OverviewStats, error) {
	return managedfiles.OverviewStats{}, nil
}

func (s *recordingManagedFilesStore) GetManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return s.managedFile, nil
}

func (s *recordingManagedFilesStore) CreateManagedFile(context.Context, string, managedfiles.ManagedFileUpsert) (managedfiles.ManagedFile, error) {
	return s.managedFile, nil
}

func (s *recordingManagedFilesStore) RetireManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return s.managedFile, nil
}

type recordingFileStore struct {
	file files.File
}

func (s *recordingFileStore) ListFiles(context.Context, string, pagination.Params) ([]files.File, error) {
	return []files.File{s.file}, nil
}

func (s *recordingFileStore) GetFile(context.Context, string, string) (files.File, error) {
	return s.file, nil
}

func (s *recordingFileStore) CreateFile(context.Context, string, files.FileUpsert) (files.File, error) {
	return s.file, nil
}

func (s *recordingFileStore) RetireFile(context.Context, string, string) (files.File, error) {
	return s.file, nil
}

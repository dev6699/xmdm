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
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/pagination"
)

func TestCertificateMutationsRecordAudit(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
	artifactStore := &recordingArtifactStore{}
	certStore := &recordingCertificateStore{
		certificate: certificates.Certificate{
			RecordBase: certificates.RecordBase{ID: "cert-1", TenantID: "tenant-1", Status: certificates.StatusActive},
			Name:       "root-ca",
			ArtifactID: "artifact-2",
			Checksum:   checksum.SHA256Base64URL([]byte("certificate-content")),
		},
	}
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Certificates: certStore,
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

	t.Run("create certificate", func(t *testing.T) {
		auditStore.records = nil
		rr := postMultipart("/admin/certificates/create", map[string]string{
			"name":      "root-ca",
			"csrfToken": "token",
		}, "file", "root-ca.pem", []byte("certificate-content"))
		assertRedirect(t, rr, "/admin/certificates?ok=certificate+uploaded")
		assertAuditRecord(t, auditStore, "create", "certificates", "cert-1")
	})

	t.Run("retire certificate", func(t *testing.T) {
		auditStore.records = nil
		req := httptest.NewRequest(http.MethodPost, "/admin/certificates/cert-1/retire", strings.NewReader("csrfToken=token"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		assertRedirect(t, rr, "/admin/certificates?ok=certificate+retired")
		assertAuditRecord(t, auditStore, "retire", "certificates", "cert-1")
	})
}

type recordingCertificateStore struct {
	certificate certificates.Certificate
}

func (s *recordingCertificateStore) ListCertificates(context.Context, string, pagination.Params) ([]certificates.Certificate, error) {
	return []certificates.Certificate{s.certificate}, nil
}

func (s *recordingCertificateStore) ListActiveCertificates(context.Context, string, pagination.Params) ([]certificates.Certificate, error) {
	return []certificates.Certificate{s.certificate}, nil
}

func (s *recordingCertificateStore) GetOverviewStats(context.Context, string) (certificates.OverviewStats, error) {
	return certificates.OverviewStats{}, nil
}

func (s *recordingCertificateStore) GetCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return s.certificate, nil
}

func (s *recordingCertificateStore) CreateCertificate(context.Context, string, certificates.CertificateUpsert) (certificates.Certificate, error) {
	return s.certificate, nil
}

func (s *recordingCertificateStore) RetireCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return s.certificate, nil
}

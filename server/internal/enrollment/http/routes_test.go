package enrollmenthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
)

func TestRegisterQRPng(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, "tenant-1")

	body := `{
		"serverUrl":"https://mdm.example/base/",
		"serverProject":"rest",
		"enrollmentToken":"token-123",
		"deviceAdminPackageDownloadLocation":"https://cdn.example/launcher.apk",
		"deviceAdminPackageChecksum":"abc123",
		"deviceIdentityPolicy":{"deviceIdUse":"serial"},
		"bootstrapExtras":{"customer":"Acme","groups":["field"]}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment/qr", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if _, err := png.Decode(bytes.NewReader(res.Body.Bytes())); err != nil {
		t.Fatalf("decode png: %v", err)
	}
}

func TestRegisterQRJSONPayload(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, "tenant-1")

	body := `{
		"serverUrl":"https://mdm.example/base/",
		"serverProject":"rest",
		"enrollmentToken":"token-123",
		"deviceAdminPackageDownloadLocation":"https://cdn.example/launcher.apk",
		"deviceAdminPackageChecksum":"abc123",
		"deviceIdentityPolicy":{"deviceIdUse":"serial"},
		"bootstrapExtras":{"customer":"Acme","groups":["field"]}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment/qr/json", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME"] != "com.xmdm.launcher/.AdminReceiver" {
		t.Fatalf("unexpected component: %#v", payload["android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME"])
	}
	if payload["android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION"] != "https://cdn.example/launcher.apk" {
		t.Fatalf("unexpected package url: %#v", payload["android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION"])
	}
	if payload["android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM"] != "abc123" {
		t.Fatalf("unexpected checksum: %#v", payload["android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM"])
	}

	extras, ok := payload["android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE"].(map[string]any)
	if !ok {
		t.Fatalf("extras bundle has wrong type: %T", payload["android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE"])
	}
	if extras["com.xmdm.BASE_URL"] != "https://mdm.example/base" {
		t.Fatalf("unexpected base url: %#v", extras["com.xmdm.BASE_URL"])
	}
	if extras["com.xmdm.SERVER_PROJECT"] != "rest" {
		t.Fatalf("unexpected server project: %#v", extras["com.xmdm.SERVER_PROJECT"])
	}
	if extras["com.xmdm.ENROLLMENT_TOKEN"] != "token-123" {
		t.Fatalf("unexpected enrollment token: %#v", extras["com.xmdm.ENROLLMENT_TOKEN"])
	}
	if extras["com.xmdm.DEVICE_ID_USE"] != "serial" {
		t.Fatalf("unexpected device id use: %#v", extras["com.xmdm.DEVICE_ID_USE"])
	}
	if extras["com.xmdm.CUSTOMER"] != "Acme" {
		t.Fatalf("unexpected customer: %#v", extras["com.xmdm.CUSTOMER"])
	}
	if extras["com.xmdm.GROUP"] != "field" {
		t.Fatalf("unexpected group: %#v", extras["com.xmdm.GROUP"])
	}
}

func TestRegisterQRValidationAndPermissions(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment/qr/json", bytes.NewBufferString(`{"serverUrl":"not-a-url","deviceAdminPackageDownloadLocation":"https://cdn.example/launcher.apk","deviceAdminPackageChecksum":"abc123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d", res.Code)
	}

	svc = auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	session, err = svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/enrollment/qr/json", bytes.NewBufferString(`{"serverUrl":"not-a-url","deviceAdminPackageDownloadLocation":"https://cdn.example/launcher.apk","deviceAdminPackageChecksum":"abc123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res = httptest.NewRecorder()
	mux = http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, "tenant-1")
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", res.Code)
	}
}

func TestRegisterTokenLifecycleRoutes(t *testing.T) {
	store := &fakeEnrollmentStore{
		issued: enrollment.IssuedToken{
			Token: enrollment.Token{
				ID:       "token-id",
				TenantID: "tenant-1",
				Status:   enrollment.TokenStatusIssued,
			},
			Secret: "secret-token",
		},
		validated: enrollment.Token{
			ID:       "token-id",
			TenantID: "tenant-1",
			Status:   enrollment.TokenStatusIssued,
		},
		consumed: enrollment.Token{
			ID:       "token-id",
			TenantID: "tenant-1",
			Status:   enrollment.TokenStatusConsumed,
		},
		revoked: enrollment.Token{
			ID:       "token-id",
			TenantID: "tenant-1",
			Status:   enrollment.TokenStatusRevoked,
		},
	}

	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, store, nil, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment/tokens", bytes.NewBufferString(`{"ttlSeconds":3600}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected issue ok, got %d", res.Code)
	}
	var issued map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}
	if issued["token"] != "secret-token" {
		t.Fatalf("unexpected token secret: %#v", issued["token"])
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/enrollment/tokens/validate", bytes.NewBufferString(`{"token":"secret-token"}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected validate ok, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/enrollment/tokens/consume", bytes.NewBufferString(`{"token":"secret-token"}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected consume ok, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/enrollment/tokens/token-id", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected revoke ok, got %d", res.Code)
	}
}

func TestRegisterEnrollmentBindRoute(t *testing.T) {
	store := &fakeEnrollmentStore{
		bound: enrollment.BoundDevice{
			DeviceID:     "device-123",
			DeviceSecret: "device-secret",
			Status:       device.StatusEnrolled,
		},
	}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, store, &fakeCertificateStore{
		active: []certificates.Certificate{
			{
				RecordBase: certificates.RecordBase{
					ID:       "cert-1",
					TenantID: "tenant-1",
					Status:   certificates.StatusActive,
				},
				Name:       "wifi-root-ca",
				ArtifactID: "artifact-1",
				Checksum:   "sha256-cert-abc",
			},
		},
	}, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment", bytes.NewBufferString(`{
		"enrollmentToken":"secret-token",
		"deviceIdentityPolicy":{"deviceId":"device-123","deviceIdUse":"serial"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected bind ok, got %d", res.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode bind response: %v", err)
	}
	if payload["deviceId"] != "device-123" {
		t.Fatalf("unexpected device id: %#v", payload["deviceId"])
	}
	if payload["deviceSecret"] != "device-secret" {
		t.Fatalf("unexpected device secret: %#v", payload["deviceSecret"])
	}
	rawConfig, ok := payload["config"].(map[string]any)
	if !ok {
		t.Fatalf("config has wrong type: %T", payload["config"])
	}
	rawConfigJSON, err := json.Marshal(rawConfig)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var typedConfig struct {
		Version      string                     `json:"version"`
		Device       map[string]any             `json:"device"`
		Policy       map[string]any             `json:"policy"`
		Apps         []any                      `json:"apps"`
		Files        []any                      `json:"files"`
		Certificates []certificates.Certificate `json:"certificates"`
		Commands     []any                      `json:"commands"`
		Signature    string                     `json:"signature"`
	}
	if err := json.Unmarshal(rawConfigJSON, &typedConfig); err != nil {
		t.Fatalf("decode config snapshot: %v", err)
	}
	config := enrollment.ConfigSnapshot{
		Version:   typedConfig.Version,
		Device:    typedConfig.Device,
		Policy:    typedConfig.Policy,
		Apps:      typedConfig.Apps,
		Files:     typedConfig.Files,
		Commands:  typedConfig.Commands,
		Signature: typedConfig.Signature,
	}
	for _, cert := range typedConfig.Certificates {
		config.Certificates = append(config.Certificates, cert)
	}
	if config.Version != "1" {
		t.Fatalf("unexpected config version: %q", config.Version)
	}
	if err := enrollment.VerifyConfigSnapshot(config, "device-secret"); err != nil {
		t.Fatalf("verify config snapshot: %v", err)
	}
	if len(config.Certificates) != 1 {
		t.Fatalf("expected one certificate in config, got %d", len(config.Certificates))
	}
}

type fakeEnrollmentStore struct {
	issued    enrollment.IssuedToken
	validated enrollment.Token
	consumed  enrollment.Token
	revoked   enrollment.Token
	bound     enrollment.BoundDevice

	issueTenant    string
	issueExpiresAt time.Time
	validateTenant string
	validateToken  string
	consumeTenant  string
	consumeToken   string
	revokeTenant   string
	revokeID       string
	bindTenant     string
	bindToken      string
	bindDeviceID   string
}

type fakeCertificateStore struct {
	active []certificates.Certificate
}

func (s *fakeEnrollmentStore) IssueToken(ctx context.Context, tenantID string, expiresAt time.Time) (enrollment.IssuedToken, error) {
	s.issueTenant = tenantID
	s.issueExpiresAt = expiresAt
	return s.issued, nil
}

func (s *fakeEnrollmentStore) ValidateToken(_ context.Context, tenantID, token string) (enrollment.Token, error) {
	s.validateTenant = tenantID
	s.validateToken = token
	return s.validated, nil
}

func (s *fakeEnrollmentStore) ConsumeToken(_ context.Context, tenantID, token string) (enrollment.Token, error) {
	s.consumeTenant = tenantID
	s.consumeToken = token
	return s.consumed, nil
}

func (s *fakeEnrollmentStore) BindDevice(_ context.Context, tenantID, token, deviceID string) (enrollment.BoundDevice, error) {
	s.bindTenant = tenantID
	s.bindToken = token
	s.bindDeviceID = deviceID
	return s.bound, nil
}

func (s *fakeEnrollmentStore) RevokeToken(_ context.Context, tenantID, id string) (enrollment.Token, error) {
	s.revokeTenant = tenantID
	s.revokeID = id
	return s.revoked, nil
}

func (s *fakeEnrollmentStore) ExpireTokens(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (s *fakeCertificateStore) ListCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return append([]certificates.Certificate(nil), s.active...), nil
}

func (s *fakeCertificateStore) ListActiveCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return append([]certificates.Certificate(nil), s.active...), nil
}

func (s *fakeCertificateStore) CreateCertificate(context.Context, string, certificates.CertificateUpsert) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

func (s *fakeCertificateStore) RetireCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

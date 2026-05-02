package enrollmenthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/managedfiles"
	"xmdm/server/internal/policy"
)

func TestRegisterQRPng(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, nil, nil, nil, nil, nil, enrollment.RuntimeSnapshot{}, "tenant-1")

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
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, nil, nil, nil, nil, nil, enrollment.RuntimeSnapshot{}, "tenant-1")

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

func TestRegisterEnrollmentBindRouteReturnsIdentityOnly(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	store := &fakeEnrollmentStore{
		bound: enrollment.BoundDevice{
			DeviceID:     "device-123",
			DeviceSecret: "device-secret",
			Status:       device.StatusEnrolled,
		},
	}
	policyStore := &fakePolicyStore{
		policies: []policy.Policy{
			{
				RecordBase: policy.RecordBase{
					ID:       "policy-old",
					TenantID: "tenant-1",
					Status:   "active",
				},
				Name:         "old",
				Version:      1,
				KioskMode:    false,
				Restrictions: json.RawMessage(`{"blockPackages":["com.example.old"]}`),
			},
			{
				RecordBase: policy.RecordBase{
					ID:       "policy-new",
					TenantID: "tenant-1",
					Status:   "active",
				},
				Name:         "new",
				Version:      2,
				KioskMode:    true,
				Restrictions: json.RawMessage(`{"blockPackages":["com.example.bad"],"allowPackages":["com.example.good"]}`),
			},
		},
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, store, nil, nil, nil, nil, policyStore, enrollment.RuntimeSnapshot{}, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment", bytes.NewBufferString(`{
		"enrollmentToken":"secret-token",
		"deviceIdentityPolicy":{"deviceId":"device-123","deviceIdUse":"serial"},
		"bootstrapExtras":{"customer":"Acme"}
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
	if payload["status"] != device.StatusEnrolled {
		t.Fatalf("unexpected status: %#v", payload["status"])
	}
	if _, ok := payload["config"]; ok {
		t.Fatalf("enrollment response should not include config: %#v", payload["config"])
	}
}

func TestRegisterQRValidationAndPermissions(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, nil, nil, nil, nil, nil, enrollment.RuntimeSnapshot{}, "tenant-1")

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
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, nil, nil, nil, nil, nil, enrollment.RuntimeSnapshot{}, "tenant-1")
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
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, store, nil, nil, nil, nil, nil, enrollment.RuntimeSnapshot{}, "tenant-1")

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
	deviceStore := &fakeDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{
				ID:       "device-record-1",
				TenantID: "tenant-1",
				Status:   device.StatusEnrolled,
			},
			Name:            "device-123",
			BootstrapExtras: map[string]any{"customer": "Acme"},
		},
	}
	appStore := &fakeAppStore{
		apps: []apps.App{
			{
				RecordBase: apps.RecordBase{
					ID:       "app-1",
					TenantID: "tenant-1",
					Status:   apps.StatusActive,
				},
				PackageName: "com.example.app",
				Name:        "Example App",
			},
		},
		versions: map[string][]apps.Version{
			"app-1": {
				{
					ID:          "version-1",
					TenantID:    "tenant-1",
					AppID:       "app-1",
					Status:      apps.VersionStatusPublished,
					VersionName: "1.0.0",
					VersionCode: 100,
					ArtifactID:  strPtr("artifact-1"),
					Checksum:    "sha256-app-abc",
				},
			},
		},
	}
	fileStore := &fakeFileStore{
		items: []managedfiles.ManagedFile{
			{
				RecordBase: managedfiles.RecordBase{
					ID:       "managed-file-1",
					TenantID: "tenant-1",
					Status:   managedfiles.StatusActive,
				},
				FileID:           "file-1",
				Path:             "device-config.txt",
				ReplaceVariables: true,
				File: &files.File{
					RecordBase: files.RecordBase{
						ID:       "file-1",
						TenantID: "tenant-1",
						Status:   files.StatusActive,
					},
					Name:       "device-config.txt",
					ArtifactID: "artifact-2",
					Checksum:   "sha256-file-abc",
					MimeType:   "text/plain",
					Artifact: &files.Artifact{
						RecordBase: files.RecordBase{
							ID:       "artifact-2",
							TenantID: "tenant-1",
							Status:   files.StatusActive,
						},
						StorageKey: "artifacts/device-config.txt",
						Checksum:   "sha256-file-abc",
						SizeBytes:  42,
						MimeType:   "text/plain",
					},
				},
			},
		},
	}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	artifactStore := &fakeArtifactStore{content: []byte("managed-file-on-device DEVICE_NUMBER CUSTOMER")}
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, deviceStore, store, appStore, fileStore, artifactStore, &fakeCertificateStore{
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
	}, nil, enrollment.RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000}, "tenant-1")

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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/config", nil)
	req.Header.Set("X-XMDM-Device-Secret", "device-secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected config ok, got %d", res.Code)
	}
	var typedConfig struct {
		Version      string                           `json:"version"`
		Runtime      enrollment.RuntimeSnapshot       `json:"runtime"`
		Device       enrollment.DeviceSnapshot        `json:"device"`
		Policy       enrollment.PolicySnapshot        `json:"policy"`
		Apps         []enrollment.AppSnapshot         `json:"apps"`
		Files        []enrollment.ManagedFileSnapshot `json:"files"`
		Certificates []enrollment.CertificateSnapshot `json:"certificates"`
		Signature    string                           `json:"signature"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &typedConfig); err != nil {
		t.Fatalf("decode config snapshot: %v", err)
	}
	config := enrollment.ConfigSnapshot{
		Version:      typedConfig.Version,
		Runtime:      typedConfig.Runtime,
		Device:       typedConfig.Device,
		Policy:       typedConfig.Policy,
		Apps:         typedConfig.Apps,
		Files:        typedConfig.Files,
		Certificates: typedConfig.Certificates,
		Signature:    typedConfig.Signature,
	}
	if config.Version == "" {
		t.Fatalf("expected non-empty config version")
	}
	if err := enrollment.VerifyConfigSnapshot(config, "device-secret"); err != nil {
		t.Fatalf("verify config snapshot: %v", err)
	}
	if len(config.Certificates) != 1 {
		t.Fatalf("expected one certificate in config, got %d", len(config.Certificates))
	}
	if config.Certificates[0].DownloadPath != "/api/v1/devices/device-123/certificates/cert-1/artifact" {
		t.Fatalf("unexpected certificate download path: %#v", config.Certificates[0].DownloadPath)
	}
	if config.Runtime.MqttAddress != "127.0.0.1:1883" || config.Runtime.CommandPollIntervalMs != 1000 || config.Runtime.ConfigSyncIntervalMs != 1000 {
		t.Fatalf("unexpected runtime config: %#v", config.Runtime)
	}
	if len(config.Apps) != 1 {
		t.Fatalf("expected one app in config, got %d", len(config.Apps))
	}
	if len(config.Files) != 1 {
		t.Fatalf("expected one file in config, got %d", len(config.Files))
	}
	if got := config.Files[0].Checksum; got != checksum.SHA256Base64URL([]byte("managed-file-on-device device-123 Acme")) {
		t.Fatalf("unexpected rendered file checksum: %#v", got)
	}
	if config.Apps[0].DownloadPath != "/api/v1/devices/device-123/apps/app-1/versions/version-1/artifact" {
		t.Fatalf("unexpected download path: %#v", config.Apps[0].DownloadPath)
	}
	fileEntry := config.Files[0]
	if fileEntry.Path != "device-config.txt" {
		t.Fatalf("unexpected file path: %#v", fileEntry.Path)
	}
	if fileEntry.DownloadPath != "/api/v1/devices/device-123/managed-files/managed-file-1/artifact" {
		t.Fatalf("unexpected file download path: %#v", fileEntry.DownloadPath)
	}
	if !fileEntry.ReplaceVariables {
		t.Fatalf("expected replace variables to be enabled")
	}
}

func TestRegisterEnrollmentBindRouteUsesLatestPublishedVersion(t *testing.T) {
	store := &fakeEnrollmentStore{
		bound: enrollment.BoundDevice{
			DeviceID:     "device-123",
			DeviceSecret: "device-secret",
			Status:       device.StatusEnrolled,
		},
	}
	deviceStore := &fakeDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{
				ID:       "device-record-1",
				TenantID: "tenant-1",
				Status:   device.StatusEnrolled,
			},
			Name:            "device-123",
			BootstrapExtras: map[string]any{"customer": "Acme"},
		},
	}
	appStore := &fakeAppStore{
		apps: []apps.App{
			{
				RecordBase: apps.RecordBase{
					ID:       "app-1",
					TenantID: "tenant-1",
					Status:   apps.StatusActive,
				},
				PackageName: "com.example.app",
				Name:        "Example App",
			},
		},
		versions: map[string][]apps.Version{
			"app-1": {
				{
					ID:          "version-1",
					TenantID:    "tenant-1",
					AppID:       "app-1",
					Status:      apps.VersionStatusPublished,
					VersionName: "1.0.0",
					VersionCode: 100,
					ArtifactID:  strPtr("artifact-1"),
					Checksum:    "sha256-app-abc",
				},
				{
					ID:          "version-2",
					TenantID:    "tenant-1",
					AppID:       "app-1",
					Status:      apps.VersionStatusPublished,
					VersionName: "1.1.0",
					VersionCode: 110,
					ArtifactID:  strPtr("artifact-2"),
					Checksum:    "sha256-app-def",
				},
			},
		},
	}
	fileStore := &fakeFileStore{
		items: []managedfiles.ManagedFile{
			{
				RecordBase: managedfiles.RecordBase{
					ID:       "managed-file-1",
					TenantID: "tenant-1",
					Status:   managedfiles.StatusActive,
				},
				FileID:           "file-1",
				Path:             "device-config.txt",
				ReplaceVariables: true,
				File: &files.File{
					RecordBase: files.RecordBase{
						ID:       "file-1",
						TenantID: "tenant-1",
						Status:   files.StatusActive,
					},
					Name:       "device-config.txt",
					ArtifactID: "artifact-2",
					Checksum:   "sha256-file-abc",
					MimeType:   "text/plain",
					Artifact: &files.Artifact{
						RecordBase: files.RecordBase{
							ID:       "artifact-2",
							TenantID: "tenant-1",
							Status:   files.StatusActive,
						},
						StorageKey: "artifacts/device-config.txt",
						Checksum:   "sha256-file-abc",
						SizeBytes:  42,
						MimeType:   "text/plain",
					},
				},
			},
		},
	}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	artifactStore := &fakeArtifactStore{content: []byte("managed-file-on-device DEVICE_NUMBER CUSTOMER")}
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, deviceStore, store, appStore, fileStore, artifactStore, nil, nil, enrollment.RuntimeSnapshot{}, "tenant-1")

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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/config", nil)
	req.Header.Set("X-XMDM-Device-Secret", "device-secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected config ok, got %d", res.Code)
	}
	var typedConfig struct {
		Apps  []enrollment.AppSnapshot         `json:"apps"`
		Files []enrollment.ManagedFileSnapshot `json:"files"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &typedConfig); err != nil {
		t.Fatalf("decode config snapshot: %v", err)
	}
	if len(typedConfig.Apps) != 1 {
		t.Fatalf("expected one app in config, got %d", len(typedConfig.Apps))
	}
	if typedConfig.Apps[0].VersionID != "version-2" {
		t.Fatalf("expected latest version, got %#v", typedConfig.Apps[0].VersionID)
	}
	if typedConfig.Apps[0].DownloadPath != "/api/v1/devices/device-123/apps/app-1/versions/version-2/artifact" {
		t.Fatalf("unexpected download path: %#v", typedConfig.Apps[0].DownloadPath)
	}
	if len(typedConfig.Files) != 1 {
		t.Fatalf("expected one file in config, got %d", len(typedConfig.Files))
	}
	if got := typedConfig.Files[0].Checksum; got != checksum.SHA256Base64URL([]byte("managed-file-on-device device-123 Acme")) {
		t.Fatalf("unexpected rendered file checksum: %#v", got)
	}
}

func TestRegisterDeviceConfigSyncRouteReturnsLatestSnapshot(t *testing.T) {
	deviceStore := &fakeDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{
				ID:       "device-record-1",
				TenantID: "tenant-1",
				Status:   device.StatusEnrolled,
			},
			Name:            "device-123",
			BootstrapExtras: map[string]any{"CUSTOMER": "Acme"},
		},
	}
	appStore := &fakeAppStore{
		apps: []apps.App{
			{
				RecordBase: apps.RecordBase{
					ID:       "app-1",
					TenantID: "tenant-1",
					Status:   apps.StatusActive,
				},
				PackageName: "com.example.app",
				Name:        "Example App",
			},
		},
		versions: map[string][]apps.Version{
			"app-1": {
				{
					ID:          "version-1",
					TenantID:    "tenant-1",
					AppID:       "app-1",
					Status:      apps.VersionStatusPublished,
					VersionName: "1.0.0",
					VersionCode: 100,
					ArtifactID:  strPtr("artifact-1"),
					Checksum:    "sha256-app-abc",
				},
			},
		},
	}
	fileStore := &fakeFileStore{
		items: []managedfiles.ManagedFile{
			{
				RecordBase: managedfiles.RecordBase{
					ID:       "managed-file-1",
					TenantID: "tenant-1",
					Status:   managedfiles.StatusActive,
				},
				FileID:           "file-1",
				Path:             "device-config.txt",
				ReplaceVariables: true,
				File: &files.File{
					RecordBase: files.RecordBase{
						ID:       "file-1",
						TenantID: "tenant-1",
						Status:   files.StatusActive,
					},
					Name:       "device-config.txt",
					ArtifactID: "artifact-2",
					Checksum:   "sha256-file-abc",
					MimeType:   "text/plain",
					Artifact: &files.Artifact{
						RecordBase: files.RecordBase{
							ID:       "artifact-2",
							TenantID: "tenant-1",
							Status:   files.StatusActive,
						},
						StorageKey: "artifacts/device-config.txt",
						Checksum:   "sha256-file-abc",
						SizeBytes:  42,
						MimeType:   "text/plain",
					},
				},
			},
		},
	}
	policyStore := &fakePolicyStore{
		policies: []policy.Policy{
			{
				RecordBase: policy.RecordBase{
					ID:       "policy-1",
					TenantID: "tenant-1",
					Status:   "active",
				},
				Name:            "policy-one",
				Version:         3,
				KioskMode:       true,
				KioskAppPackage: "com.example.kiosk",
				Restrictions: json.RawMessage(`{
					"blockPackages":["com.example.bad"],
					"allowPackages":["com.example.good"]
				}`),
			},
		},
	}

	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	artifactStore := &fakeArtifactStore{content: []byte("managed-file-on-device DEVICE_NUMBER CUSTOMER")}
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, deviceStore, nil, appStore, fileStore, artifactStore, &fakeCertificateStore{
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
	}, policyStore, enrollment.RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000}, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/config", nil)
	req.Header.Set("X-XMDM-Device-Secret", "device-secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}

	var typedConfig struct {
		Version      string                           `json:"version"`
		Runtime      enrollment.RuntimeSnapshot       `json:"runtime"`
		Device       enrollment.DeviceSnapshot        `json:"device"`
		Policy       enrollment.PolicySnapshot        `json:"policy"`
		Apps         []enrollment.AppSnapshot         `json:"apps"`
		Files        []enrollment.ManagedFileSnapshot `json:"files"`
		Certificates []enrollment.CertificateSnapshot `json:"certificates"`
		Signature    string                           `json:"signature"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &typedConfig); err != nil {
		t.Fatalf("decode config snapshot: %v", err)
	}
	config := enrollment.ConfigSnapshot{
		Version:      typedConfig.Version,
		Runtime:      typedConfig.Runtime,
		Device:       typedConfig.Device,
		Policy:       typedConfig.Policy,
		Apps:         typedConfig.Apps,
		Files:        typedConfig.Files,
		Certificates: typedConfig.Certificates,
		Signature:    typedConfig.Signature,
	}
	if err := enrollment.VerifyConfigSnapshot(config, "device-secret"); err != nil {
		t.Fatalf("verify config snapshot: %v", err)
	}
	if config.Device.DeviceID != "device-123" {
		t.Fatalf("unexpected device id: %#v", config.Device.DeviceID)
	}
	if config.Runtime.MqttAddress != "127.0.0.1:1883" || config.Runtime.CommandPollIntervalMs != 1000 || config.Runtime.ConfigSyncIntervalMs != 1000 {
		t.Fatalf("unexpected runtime config: %#v", config.Runtime)
	}
	if config.Policy.Name != "policy-one" || !config.Policy.KioskMode || config.Policy.KioskAppPackage != "com.example.kiosk" {
		t.Fatalf("unexpected policy: %#v", config.Policy)
	}
	if config.Policy.Restrictions.KioskKeepScreenOn || config.Policy.Restrictions.KioskStayAwakeWhilePluggedIn || config.Policy.Restrictions.KioskUnlockOnBoot {
		t.Fatalf("unexpected kiosk screen defaults: %#v", config.Policy.Restrictions)
	}
	if len(config.Apps) != 1 || len(config.Files) != 1 || len(config.Certificates) != 1 {
		t.Fatalf("unexpected snapshot contents: apps=%d files=%d certs=%d", len(config.Apps), len(config.Files), len(config.Certificates))
	}
	if config.Certificates[0].DownloadPath != "/api/v1/devices/device-123/certificates/cert-1/artifact" {
		t.Fatalf("unexpected certificate download path: %#v", config.Certificates[0].DownloadPath)
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

type fakeFileStore struct {
	items []managedfiles.ManagedFile
}

type fakeAppStore struct {
	apps     []apps.App
	versions map[string][]apps.Version
}

type fakePolicyStore struct {
	policies []policy.Policy
}

type fakeDeviceStore struct {
	device device.Device
}

type fakeArtifactStore struct {
	content []byte
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

func (s *fakeEnrollmentStore) BindDevice(_ context.Context, tenantID, token, deviceID string, _ map[string]any) (enrollment.BoundDevice, error) {
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

func (s *fakeCertificateStore) GetCertificate(context.Context, string, string) (certificates.Certificate, error) {
	if len(s.active) == 0 {
		return certificates.Certificate{}, httpx.ErrNotFound
	}
	return s.active[0], nil
}

func (s *fakeCertificateStore) CreateCertificate(context.Context, string, certificates.CertificateUpsert) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

func (s *fakeCertificateStore) RetireCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

func (s *fakeFileStore) ListManagedFiles(context.Context, string) ([]managedfiles.ManagedFile, error) {
	return append([]managedfiles.ManagedFile(nil), s.items...), nil
}

func (s *fakeFileStore) GetManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}

func (s *fakeFileStore) CreateManagedFile(context.Context, string, managedfiles.ManagedFileUpsert) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}

func (s *fakeFileStore) RetireManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}

func (s *fakeAppStore) ListApps(context.Context, string) ([]apps.App, error) {
	return append([]apps.App(nil), s.apps...), nil
}

func (s *fakeAppStore) CreateApp(context.Context, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}

func (s *fakeAppStore) UpdateApp(context.Context, string, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}

func (s *fakeAppStore) RetireApp(context.Context, string, string) (apps.App, error) {
	return apps.App{}, nil
}

func (s *fakeAppStore) ListVersions(_ context.Context, _ string, appID string) ([]apps.Version, error) {
	return append([]apps.Version(nil), s.versions[appID]...), nil
}

func (s *fakeAppStore) GetVersion(context.Context, string, string, string) (apps.Version, error) {
	return apps.Version{}, nil
}

func (s *fakeAppStore) CreateVersion(context.Context, string, string, apps.VersionUpsert) (apps.Version, error) {
	return apps.Version{}, nil
}

func (s *fakePolicyStore) ListPolicies(context.Context, string) ([]policy.Policy, error) {
	return append([]policy.Policy(nil), s.policies...), nil
}

func (s *fakePolicyStore) CreatePolicy(context.Context, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *fakePolicyStore) UpdatePolicy(context.Context, string, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *fakePolicyStore) RetirePolicy(context.Context, string, string) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *fakeDeviceStore) ListDevices(context.Context, string) ([]device.Device, error) {
	return []device.Device{s.device}, nil
}

func (s *fakeDeviceStore) CreateDevice(context.Context, string, device.DeviceUpsert) (device.Device, error) {
	return s.device, nil
}

func (s *fakeDeviceStore) UpdateDevice(context.Context, string, string, device.DeviceUpsert) (device.Device, error) {
	return s.device, nil
}

func (s *fakeDeviceStore) RetireDevice(context.Context, string, string) (device.Device, error) {
	return s.device, nil
}

func (s *fakeDeviceStore) Authenticate(context.Context, string, string, string) (device.Device, error) {
	return s.device, nil
}

func (s *fakeArtifactStore) Put(context.Context, string, io.Reader, string, int64) error {
	return nil
}

func (s *fakeArtifactStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

func (s *fakeArtifactStore) Delete(context.Context, string) error {
	return nil
}

func strPtr(value string) *string {
	return &value
}

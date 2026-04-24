package enrollmenthttp

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
)

func TestRegisterQRPng(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1/enrollment"), svc)

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
	Register(httpx.WithPrefix(mux, "/api/v1/enrollment"), svc)

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
	Register(httpx.WithPrefix(mux, "/api/v1/enrollment"), svc)

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
	Register(httpx.WithPrefix(mux, "/api/v1/enrollment"), svc)
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", res.Code)
	}
}

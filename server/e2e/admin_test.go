package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	v1 "xmdm/server/internal/api/v1"
	appspg "xmdm/server/internal/apps/postgres"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	certificatesspg "xmdm/server/internal/certificates/postgres"
	device "xmdm/server/internal/device"
	devicepg "xmdm/server/internal/device/postgres"
	enrollmentpg "xmdm/server/internal/enrollment/postgres"
	filespg "xmdm/server/internal/files/postgres"
	grouppg "xmdm/server/internal/group/postgres"
	identitypg "xmdm/server/internal/identity/postgres"
	"xmdm/server/internal/plugins"
	policypg "xmdm/server/internal/policy/postgres"
	telemetrypg "xmdm/server/internal/telemetry/postgres"
)

func TestAdminE2E(t *testing.T) {
	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	auditStore := auditpg.NewDBStore(pool)
	artifactStore := newTestArtifactStore(t)
	defer func() { _ = artifactStore.Delete(context.Background(), "artifacts/launcher.apk") }()
	handler := v1.NewMux(svc, testDeps(pool, auditStore, plugins.Disabled(), artifactStore))
	client := newE2EClient(t, handler)
	baseURL := "http://xmdm.local"

	login(client, t, baseURL, "admin", "secret")
	assertStatus(t, client, http.MethodGet, baseURL+"/api/v1/admin/me", "", http.StatusOK)

	for _, kind := range []string{"users", "roles", "apps", "groups", "policies", "devices"} {
		created := postJSON(t, client, baseURL+"/api/v1/"+kind, crudCreateBody(kind))
		id, _ := created["id"].(string)
		if id == "" {
			t.Fatalf("%s create returned empty id", kind)
		}
		if created["id"] == "" {
			t.Fatalf("%s create returned id %v", kind, created["id"])
		}
		if kind == "devices" {
			if created["status"] != device.StatusPending {
				t.Fatalf("%s create returned status %v", kind, created["status"])
			}
		} else if created["status"] != device.StatusActive {
			t.Fatalf("%s create returned status %v", kind, created["status"])
		}

		listed := getJSONList(t, client, baseURL+"/api/v1/"+kind)
		found := false
		for _, item := range listed {
			if item["id"] == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s list did not include created item", kind)
		}

		updated := patchJSON(t, client, baseURL+"/api/v1/"+kind+"/"+id, crudUpdateBody(kind))
		if kind == "users" {
			if updated["email"] != "users-two@example.com" {
				t.Fatalf("%s update returned email %v", kind, updated["email"])
			}
		} else if updated["name"] != kind+"-two" {
			t.Fatalf("%s update returned name %v", kind, updated["name"])
		}

		if kind == "apps" {
			version := postJSON(t, client, baseURL+"/api/v1/apps/"+id+"/versions", `{
				"versionName":"1.0.0",
				"versionCode":100,
				"artifactId":"artifact-1",
				"checksum":"sha256-abc",
				"publish":true
			}`)
			if version["status"] != "published" {
				t.Fatalf("app version create returned status %v", version["status"])
			}
			if version["versionName"] != "1.0.0" {
				t.Fatalf("app version create returned versionName %v", version["versionName"])
			}
			versions := getJSONList(t, client, baseURL+"/api/v1/apps/"+id+"/versions")
			if len(versions) != 1 {
				t.Fatalf("expected one version, got %d", len(versions))
			}
			if versions[0]["status"] != "published" {
				t.Fatalf("app version list returned status %v", versions[0]["status"])
			}
		}

		retired := deleteJSON(t, client, baseURL+"/api/v1/"+kind+"/"+id)
		if retired["status"] != device.StatusRetired {
			t.Fatalf("%s retire returned status %v", kind, retired["status"])
		}
	}

	fileCreated := postMultipartFile(t, client, baseURL+"/api/v1/files", map[string]string{
		"name":       "launcher.apk",
		"storageKey": "artifacts/launcher.apk",
		"checksum":   "sha256-file-abc",
		"sizeBytes":  "1024",
		"mimeType":   "application/vnd.android.package-archive",
	}, "file", "launcher.apk", bytes.Repeat([]byte("x"), 1024))
	if fileCreated["name"] != "launcher.apk" {
		t.Fatalf("file create returned name %v", fileCreated["name"])
	}
	if fileCreated["artifact"] == nil {
		t.Fatalf("expected artifact details in file response: %#v", fileCreated)
	}
	files := getJSONList(t, client, baseURL+"/api/v1/files")
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	if files[0]["artifact"] == nil {
		t.Fatalf("expected artifact details in file list: %#v", files[0])
	}
	fileID, _ := fileCreated["id"].(string)
	if fileID == "" {
		t.Fatalf("file create returned empty id")
	}
	fileRetired := deleteJSON(t, client, baseURL+"/api/v1/files/"+fileID)
	if fileRetired["status"] != "retired" {
		t.Fatalf("file retire returned status %v", fileRetired["status"])
	}

	certCreated := postMultipartFile(t, client, baseURL+"/api/v1/certificates", map[string]string{
		"name":       "wifi-root-ca.pem",
		"storageKey": "artifacts/wifi-root-ca.pem",
		"checksum":   "sha256-cert-abc",
		"sizeBytes":  "512",
		"mimeType":   "application/x-pem-file",
	}, "file", "wifi-root-ca.pem", bytes.Repeat([]byte("c"), 512))
	if certCreated["name"] != "wifi-root-ca.pem" {
		t.Fatalf("certificate create returned name %v", certCreated["name"])
	}
	if certCreated["artifact"] == nil {
		t.Fatalf("expected artifact details in certificate response: %#v", certCreated)
	}
	defer func() { _ = artifactStore.Delete(context.Background(), "artifacts/wifi-root-ca.pem") }()
	certificates := getJSONList(t, client, baseURL+"/api/v1/certificates")
	if len(certificates) != 1 {
		t.Fatalf("expected one certificate, got %d", len(certificates))
	}
	if certificates[0]["artifact"] == nil {
		t.Fatalf("expected artifact details in certificate list: %#v", certificates[0])
	}
	certID, _ := certCreated["id"].(string)
	if certID == "" {
		t.Fatalf("certificate create returned empty id")
	}
	certRetired := deleteJSON(t, client, baseURL+"/api/v1/certificates/"+certID)
	if certRetired["status"] != "retired" {
		t.Fatalf("certificate retire returned status %v", certRetired["status"])
	}

	events, err := auditStore.List(context.Background(), bootstrap.SeedTenantID)
	if err != nil {
		t.Fatalf("audit list failed: %v", err)
	}
	if len(events) != 23 {
		t.Fatalf("expected 23 audit events, got %d", len(events))
	}
	if events[0].Action != "create" || events[len(events)-1].Action != "retire" {
		t.Fatalf("unexpected audit actions: first=%s last=%s", events[0].Action, events[len(events)-1].Action)
	}

	assertStatus(t, client, http.MethodPost, baseURL+"/api/v1/admin/logout", "", http.StatusNoContent)
	assertStatus(t, client, http.MethodGet, baseURL+"/api/v1/admin/me", "", http.StatusUnauthorized)
}

func crudCreateBody(kind string) string {
	switch kind {
	case "users":
		return `{"email":"users-one@example.com","passwordHash":"hash-users-one","roleId":"` + bootstrap.SeedAdminRoleID + `"}`
	case "roles":
		return `{"name":"roles-one","permissions":["admin.read","admin.write"]}`
	case "groups":
		return `{"name":"groups-one"}`
	case "apps":
		return `{"packageName":"com.example.app","name":"apps-one"}`
	case "policies":
		return `{"name":"policies-one","version":1,"kioskMode":false,"restrictions":{"camera":false}}`
	case "devices":
		return `{"name":"devices-one","secretHash":"hash-devices-one"}`
	default:
		return `{"name":"` + kind + `-one"}`
	}
}

func crudUpdateBody(kind string) string {
	switch kind {
	case "users":
		return `{"email":"users-two@example.com","passwordHash":"hash-users-two","roleId":"` + bootstrap.SeedAdminRoleID + `"}`
	case "roles":
		return `{"name":"roles-two","permissions":["admin.read"]}`
	case "groups":
		return `{"name":"groups-two"}`
	case "apps":
		return `{"packageName":"com.example.app.two","name":"apps-two"}`
	case "policies":
		return `{"name":"policies-two","version":2,"kioskMode":true,"restrictions":{"camera":true}}`
	case "devices":
		return `{"name":"devices-two","secretHash":"hash-devices-two"}`
	default:
		return `{"name":"` + kind + `-two"}`
	}
}

func newE2EClient(t *testing.T, handler http.Handler) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{
		Jar:       jar,
		Transport: handlerTransport{handler: handler},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type handlerTransport struct {
	handler http.Handler
}

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.handler.ServeHTTP(rec, req)
	res := rec.Result()
	res.Request = req
	if res.Body == nil {
		res.Body = io.NopCloser(strings.NewReader(""))
	}
	return res, nil
}

func login(client *http.Client, t *testing.T, baseURL, username, password string) {
	t.Helper()
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/admin/login", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected login redirect, got %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
}

func assertStatus(t *testing.T, client *http.Client, method, url, body string, want int) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	if res.StatusCode != want {
		t.Fatalf("expected %d, got %d for %s %s", want, res.StatusCode, method, url)
	}
}

func postJSON(t *testing.T, client *http.Client, url, body string) map[string]any {
	t.Helper()
	return doJSON(t, client, http.MethodPost, url, body, http.StatusOK)
}

func patchJSON(t *testing.T, client *http.Client, url, body string) map[string]any {
	t.Helper()
	return doJSON(t, client, http.MethodPatch, url, body, http.StatusOK)
}

func deleteJSON(t *testing.T, client *http.Client, url string) map[string]any {
	t.Helper()
	return doJSON(t, client, http.MethodDelete, url, "", http.StatusOK)
}

func getJSONList(t *testing.T, client *http.Client, url string) []map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build list request: %v", err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("list request got %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return listed
}

func doJSON(t *testing.T, client *http.Client, method, url, body string, want int) map[string]any {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != want {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected %d, got %d for %s %s: %s", want, res.StatusCode, method, url, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode json response: %v", err)
	}
	return payload
}

func testDeps(pool *pgxpool.Pool, auditStore audit.Store, pluginManager *plugins.Manager, artifactStore artifacts.Store) v1.Dependencies {
	return v1.Dependencies{
		Identity:      identitypg.New(pool),
		Apps:          appspg.New(pool),
		Files:         filespg.New(pool),
		Certificates:  certificatesspg.New(pool),
		Groups:        grouppg.New(pool),
		Policies:      policypg.New(pool),
		Devices:       devicepg.New(pool),
		Enrollment:    enrollmentpg.New(pool),
		Telemetry:     telemetrypg.New(pool),
		Audit:         auditStore,
		PluginManager: pluginManager,
		Artifacts:     artifactStore,
		TenantID:      bootstrap.SeedTenantID,
	}
}

func postMultipartFile(t *testing.T, client *http.Client, url string, fields map[string]string, fileField, fileName string, content []byte) map[string]any {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write form field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatalf("create multipart file part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatalf("build multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("multipart upload request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(res.Body)
		t.Fatalf("expected %d, got %d for multipart upload: %s", http.StatusOK, res.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode multipart response: %v", err)
	}
	return payload
}

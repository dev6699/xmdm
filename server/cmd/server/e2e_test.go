package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/admin"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	devicepg "xmdm/server/internal/device/postgres"
	grouppg "xmdm/server/internal/group/postgres"
	identitypg "xmdm/server/internal/identity/postgres"
	"xmdm/server/internal/plugins"
	policypg "xmdm/server/internal/policy/postgres"
)

func TestAdminE2E(t *testing.T) {
	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	store := admin.NewRepository(
		identitypg.New(pool),
		grouppg.New(pool),
		policypg.New(pool),
		devicepg.New(pool),
	)
	auditStore := auditpg.NewDBStore(pool)
	handler := newMux(svc, store, auditStore, plugins.Disabled())
	client := newE2EClient(t, handler)
	baseURL := "http://xmdm.local"

	login(client, t, baseURL, "admin", "secret")
	assertStatus(t, client, http.MethodGet, baseURL+"/api/v1/admin/me", "", http.StatusOK)

	for _, kind := range []string{"users", "roles", "groups", "policies", "devices"} {
		created := postJSON(t, client, baseURL+"/api/v1/"+kind, crudCreateBody(kind))
		id, _ := created["id"].(string)
		if id == "" {
			t.Fatalf("%s create returned empty id", kind)
		}
		if created["id"] == "" {
			t.Fatalf("%s create returned id %v", kind, created["id"])
		}
		if kind == "devices" {
			if created["status"] != "pending" {
				t.Fatalf("%s create returned status %v", kind, created["status"])
			}
		} else if created["status"] != "active" {
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

		retired := deleteJSON(t, client, baseURL+"/api/v1/"+kind+"/"+id)
		if retired["status"] != "retired" {
			t.Fatalf("%s retire returned status %v", kind, retired["status"])
		}
	}

	events, err := auditStore.List(context.Background(), bootstrap.SeedTenantID)
	if err != nil {
		t.Fatalf("audit list failed: %v", err)
	}
	if len(events) != 15 {
		t.Fatalf("expected 15 audit events, got %d", len(events))
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
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload []map[string]any
	decodeBody(t, res.Body, &payload)
	return payload
}

func doJSON(t *testing.T, client *http.Client, method, url, body string, want int) map[string]any {
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
	if res.StatusCode != want {
		data, _ := io.ReadAll(res.Body)
		t.Fatalf("expected %d, got %d: %s", want, res.StatusCode, strings.TrimSpace(string(data)))
	}
	var payload map[string]any
	decodeBody(t, res.Body, &payload)
	return payload
}

func decodeBody(t *testing.T, r io.Reader, dst any) {
	t.Helper()
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("empty body")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}

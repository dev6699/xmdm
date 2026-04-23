package main

import (
	"bytes"
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
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
)

func TestAdminE2E(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	store := admin.NewStore()
	auditStore := audit.NewStore()
	handler := newMux(svc, store, auditStore)
	client := newE2EClient(t, handler)
	baseURL := "http://xmdm.local"

	login(client, t, baseURL, "admin", "secret")
	assertStatus(t, client, http.MethodGet, baseURL+"/admin/me", "", http.StatusOK)

	for _, kind := range []string{"users", "roles", "groups", "policies", "devices"} {
		created := postJSON(t, client, baseURL+"/admin/"+kind, `{"name":"`+kind+`-one","extra":{"kind":"`+kind+`"}}`)
		if created["id"] != kind+"-1" {
			t.Fatalf("%s create returned id %v", kind, created["id"])
		}
		if created["status"] != "active" {
			t.Fatalf("%s create returned status %v", kind, created["status"])
		}

		listed := getJSONList(t, client, baseURL+"/admin/"+kind)
		if len(listed) != 1 {
			t.Fatalf("%s list returned %d items", kind, len(listed))
		}

		updated := patchJSON(t, client, baseURL+"/admin/"+kind+"/"+kind+"-1", `{"name":"`+kind+`-two"}`)
		if updated["name"] != kind+"-two" {
			t.Fatalf("%s update returned name %v", kind, updated["name"])
		}

		retired := deleteJSON(t, client, baseURL+"/admin/"+kind+"/"+kind+"-1")
		if retired["status"] != "retired" {
			t.Fatalf("%s retire returned status %v", kind, retired["status"])
		}
	}

	events := auditStore.List("tenant-1")
	if len(events) != 15 {
		t.Fatalf("expected 15 audit events, got %d", len(events))
	}
	if events[0].Action != "create" || events[len(events)-1].Action != "retire" {
		t.Fatalf("unexpected audit actions: first=%s last=%s", events[0].Action, events[len(events)-1].Action)
	}

	assertStatus(t, client, http.MethodPost, baseURL+"/admin/logout", "", http.StatusNoContent)
	assertStatus(t, client, http.MethodGet, baseURL+"/admin/me", "", http.StatusUnauthorized)
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

	req, err := http.NewRequest(http.MethodPost, baseURL+"/admin/login", strings.NewReader(form.Encode()))
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

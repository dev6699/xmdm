package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/admin"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
)

func TestAdminDevicesRouteRequiresPermission(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	mux := newMux(svc, admin.NewStore(), audit.NewStore())

	req := httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without session, got %d", res.Code)
	}

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRes := httptest.NewRecorder()
	mux.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d", loginRes.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without devices.read, got %d", res.Code)
	}
}

func TestAdminDevicesRouteAllowsPermission(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := newMux(svc, admin.NewStore(), audit.NewStore())
	req := httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
}

func TestCoreCrudLifecycle(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })
	store := admin.NewStore()
	auditStore := audit.NewStore()
	mux := newMux(svc, store, auditStore)

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	for _, kind := range []string{"users", "roles", "groups", "policies", "devices"} {
		createBody := `{"name":"` + kind + `-one","extra":{"kind":"` + kind + `"}}`
		req := httptest.NewRequest(http.MethodPost, "/admin/"+kind, strings.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s create failed: %d", kind, res.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/admin/"+kind, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s list failed: %d", kind, res.Code)
		}

		req = httptest.NewRequest(http.MethodPatch, "/admin/"+kind+"/"+kind+"-1", strings.NewReader(`{"name":"`+kind+`-two"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s update failed: %d", kind, res.Code)
		}

		req = httptest.NewRequest(http.MethodDelete, "/admin/"+kind+"/"+kind+"-1", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s retire failed: %d", kind, res.Code)
		}
	}

	events := auditStore.List("tenant-1")
	if len(events) != 15 {
		t.Fatalf("expected 15 audit events, got %d", len(events))
	}
	if events[0].Action != "create" || events[len(events)-1].Action != "retire" {
		t.Fatalf("unexpected audit actions: first=%s last=%s", events[0].Action, events[len(events)-1].Action)
	}
}

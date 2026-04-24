package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	v1 "xmdm/server/internal/api/v1"
	"xmdm/server/internal/audit"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	devicepg "xmdm/server/internal/device/postgres"
	enrollmentpg "xmdm/server/internal/enrollment/postgres"
	grouppg "xmdm/server/internal/group/postgres"
	identitypg "xmdm/server/internal/identity/postgres"
	"xmdm/server/internal/plugins"
	policypg "xmdm/server/internal/policy/postgres"
	telemetrypg "xmdm/server/internal/telemetry/postgres"
)

func TestAdminDevicesRouteRequiresPermission(t *testing.T) {
	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	mux := newMux(svc, testDeps(pool, auditpg.NewDBStore(pool), plugins.Disabled()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
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
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRes := httptest.NewRecorder()
	mux.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d", loginRes.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without devices.read, got %d", res.Code)
	}
}

func TestAdminDevicesRouteAllowsPermission(t *testing.T) {
	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := newMux(svc, testDeps(pool, auditpg.NewDBStore(pool), plugins.Disabled()))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
}

func TestCoreCrudLifecycle(t *testing.T) {
	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })
	auditStore := auditpg.NewDBStore(pool)
	mux := newMux(svc, testDeps(pool, auditStore, plugins.Disabled()))

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	for _, kind := range []string{"users", "roles", "groups", "policies", "devices"} {
		createBody := crudCreateBody(kind)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/"+kind, strings.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s create failed: %d", kind, res.Code)
		}
		var created map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
			t.Fatalf("%s create decode failed: %v", kind, err)
		}
		id, _ := created["id"].(string)
		if id == "" {
			t.Fatalf("%s create returned empty id", kind)
		}

		req = httptest.NewRequest(http.MethodGet, "/api/v1/"+kind, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s list failed: %d", kind, res.Code)
		}
		var listed []map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
			t.Fatalf("%s list decode failed: %v", kind, err)
		}
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

		req = httptest.NewRequest(http.MethodPatch, "/api/v1/"+kind+"/"+id, strings.NewReader(crudUpdateBody(kind)))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s update failed: %d", kind, res.Code)
		}

		req = httptest.NewRequest(http.MethodDelete, "/api/v1/"+kind+"/"+id, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s retire failed: %d", kind, res.Code)
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
}

func TestPluginIsolationDoesNotExposeOptionalRoutes(t *testing.T) {
	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	mux := newMux(svc, testDeps(pool, auditpg.NewDBStore(pool), plugins.Disabled()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected optional plugin route to be absent, got %d", res.Code)
	}
}

func testDeps(pool *pgxpool.Pool, auditStore audit.Store, pluginManager *plugins.Manager) v1.Dependencies {
	return v1.Dependencies{
		Identity:      identitypg.New(pool),
		Groups:        grouppg.New(pool),
		Policies:      policypg.New(pool),
		Devices:       devicepg.New(pool),
		Enrollment:    enrollmentpg.New(pool),
		Telemetry:     telemetrypg.New(pool),
		Audit:         auditStore,
		PluginManager: pluginManager,
		TenantID:      bootstrap.SeedTenantID,
	}
}

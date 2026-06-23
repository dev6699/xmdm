package adminhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
)

func TestGroupsPageRender(t *testing.T) {
	perms := []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, perms)
	session := svc.IssueSession("admin", perms)
	store := &recordingGroupStore{
		groups: []group.Group{
			{
				RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: group.StatusActive},
				Name:       "Field Devices",
			},
		},
		group: group.Group{
			RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: group.StatusActive},
			Name:       "Field Devices",
		},
		devices: []device.Device{
			{
				RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Date(2026, 6, 23, 8, 0, 0, 0, time.UTC)},
				Name:       "warehouse-tablet-001",
				PolicyID:   func() *string { v := "policy-1"; return &v }(),
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Groups:   store,
		Audit:    &recordingAuditStore{},
		TenantID: "tenant-1",
	})

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	t.Run("groups page", func(t *testing.T) {
		rr := get("/admin/groups")
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected groups page status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Groups", "Define device cohorts and inspect the devices assigned to each cohort.", "Create group", "Field Devices"} {
			if !strings.Contains(body, want) {
				t.Fatalf("groups page missing %q: %s", want, body)
			}
		}
	})

	t.Run("group detail page", func(t *testing.T) {
		rr := get("/admin/groups/group-1")
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected group detail status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Group Detail", "Review the cohort, then update or retire it from this page.", "Update group", "Retire group", "Field Devices", "Battery", "Last online", "No telemetry", "warehouse-tablet-001"} {
			if !strings.Contains(body, want) {
				t.Fatalf("group detail page missing %q: %s", want, body)
			}
		}
	})
}

func TestGroupMutationsRecordAudit(t *testing.T) {
	perms := []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, perms)
	session := svc.IssueSession("admin", perms)
	store := &recordingGroupStore{
		groups: []group.Group{
			{
				RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: group.StatusActive},
				Name:       "Field Devices",
			},
		},
		group: group.Group{
			RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: group.StatusActive},
			Name:       "Field Devices",
		},
		createdGroup: group.Group{
			RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: group.StatusActive},
			Name:       "Field Devices",
		},
		updatedGroup: group.Group{
			RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: group.StatusActive},
			Name:       "Field Devices Updated",
		},
		retiredGroup: group.Group{
			RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: group.StatusRetired},
			Name:       "Field Devices",
		},
	}
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Groups:   store,
		Audit:    auditStore,
		TenantID: "tenant-1",
	})

	postForm := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	t.Run("create group", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/groups/create", "name=Field+Devices&csrfToken=token")
		assertRedirect(t, rr, "/admin/groups?ok=group+created")
		assertAuditRecord(t, auditStore, "create", "groups", "group-1")
	})

	t.Run("update group", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/groups/group-1/update", "name=Field+Devices+Updated&csrfToken=token")
		assertRedirect(t, rr, "/admin/groups/group-1?ok=group+updated")
		assertAuditRecord(t, auditStore, "update", "groups", "group-1")
	})

	t.Run("retire group", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/groups/group-1/retire", "csrfToken=token")
		assertRedirect(t, rr, "/admin/groups?ok=group+retired")
		assertAuditRecord(t, auditStore, "retire", "groups", "group-1")
	})
}

type recordingGroupStore struct {
	groups  []group.Group
	group   group.Group
	devices []device.Device

	createdGroup group.Group
	updatedGroup group.Group
	retiredGroup group.Group
}

func (s *recordingGroupStore) ListGroups(context.Context, string, pagination.Params) ([]group.Group, error) {
	return append([]group.Group(nil), s.groups...), nil
}

func (s *recordingGroupStore) ListActiveGroups(context.Context, string) ([]group.Group, error) {
	items := make([]group.Group, 0, len(s.groups))
	for _, item := range s.groups {
		if item.Status == group.StatusActive {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *recordingGroupStore) GetGroup(_ context.Context, _ string, id string) (group.Group, error) {
	if strings.TrimSpace(id) == strings.TrimSpace(s.group.ID) {
		return s.group, nil
	}
	return group.Group{}, httpx.ErrNotFound
}

func (s *recordingGroupStore) ListGroupDevices(context.Context, string, string, pagination.Params) ([]device.Device, error) {
	return append([]device.Device(nil), s.devices...), nil
}

func (s *recordingGroupStore) CreateGroup(context.Context, string, group.GroupUpsert) (group.Group, error) {
	if s.createdGroup.ID != "" {
		return s.createdGroup, nil
	}
	return s.group, nil
}

func (s *recordingGroupStore) UpdateGroup(context.Context, string, string, group.GroupUpsert) (group.Group, error) {
	if s.updatedGroup.ID != "" {
		return s.updatedGroup, nil
	}
	return s.group, nil
}

func (s *recordingGroupStore) RetireGroup(context.Context, string, string) (group.Group, error) {
	if s.retiredGroup.ID != "" {
		return s.retiredGroup, nil
	}
	return s.group, nil
}

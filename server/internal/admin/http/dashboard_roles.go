package adminhttp

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/roles"
)

func (d *dashboard) permissionCatalog() []auth.Permission {
	if d.deps.PluginManager != nil {
		if perms := d.deps.PluginManager.PermissionCatalog(); len(perms) > 0 {
			return perms
		}
	}
	return auth.AllPermissions()
}

func (d *dashboard) roles(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Roles")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Roles.ListRoles(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Roles", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	data := pageData{
		Title:    "Roles",
		Subtitle: "Define the permission bundles available to operators.",
		Items:    withPager(rolesTable(items), pagerHTML(r, page, limit, hasNext)),
	}
	if d.canWrite(session) {
		data.Forms = []formData{{
			Title: "Create role", Action: "/admin/roles/create",
			Fields: []fieldData{
				{Name: "name", Label: "Name", Type: "text", Placeholder: "operators", Required: true},
				{Name: "permissions", Label: "Permissions", Type: "multiselect", Options: allPermissionOptions(d.permissionCatalog())},
			},
			Submit: "Create role",
		}}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) roleDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Role Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, err := d.deps.Roles.GetRole(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Role Detail", err)
		return
	}
	id := found.ID
	catalog := d.permissionCatalog()
	data := pageData{
		Title:    "Role Detail",
		Subtitle: "Edit the permission bundle or retire it from active use.",
		Items:    panelSectionHTML("", "Current role", template.HTML(pre(found))),
	}
	if d.canWrite(session) {
		if found.Status == "active" {
			data.Forms = []formData{{
				Title:  "Update role",
				Action: "/admin/roles/" + id + "/update",
				Fields: []fieldData{
					{Name: "name", Label: "Name", Type: "text", Value: found.Name, Placeholder: "operators", Required: true},
					{Name: "permissions", Label: "Permissions", Type: "multiselect", Values: found.Permissions, Options: allPermissionOptions(catalog)},
				},
				Submit: "Update role",
			}, {
				Title:  "Retire role",
				Action: "/admin/roles/" + id + "/retire",
				Submit: "Retire role",
				Danger: true,
			}}
		}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) createRole(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Roles") {
		return
	}
	req, err := roleUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/roles", err.Error())
		return
	}
	rec, err := d.deps.Roles.CreateRole(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/roles", err.Error())
		return
	}
	d.recordAudit(r, "create", "roles", rec.ID, map[string]any{"name": rec.Name, "permissions": rec.Permissions})
	d.redirectOK(w, r, "/admin/roles", "role created")
}

func (d *dashboard) updateRole(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Roles") {
		return
	}
	req, err := roleUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), err.Error())
		return
	}
	rec, err := d.deps.Roles.UpdateRole(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), err.Error())
		return
	}
	if rec.Status != "active" {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), "conflict")
		return
	}
	d.recordAudit(r, "update", "roles", rec.ID, map[string]any{"name": rec.Name, "permissions": rec.Permissions})
	d.redirectOK(w, r, "/admin/roles/"+rec.ID, "role updated")
}

func (d *dashboard) retireRole(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Roles") {
		return
	}
	rec, err := d.deps.Roles.RetireRole(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "roles", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/roles", "role retired")
}

func roleUpsertFromForm(r *http.Request) (roles.RoleUpsert, error) {
	if err := r.ParseForm(); err != nil {
		return roles.RoleUpsert{}, err
	}
	var permissions []string
	if values := r.Form["permissions"]; len(values) > 1 {
		permissions = append(permissions, values...)
	} else {
		raw := strings.TrimSpace(r.FormValue("permissions"))
		if raw == "" {
			permissions = nil
		} else if strings.HasPrefix(raw, "[") {
			if err := json.Unmarshal([]byte(raw), &permissions); err != nil {
				return roles.RoleUpsert{}, fmt.Errorf("invalid permissions json")
			}
		} else {
			permissions = append(permissions, raw)
		}
	}
	return roles.RoleUpsert{Name: strings.TrimSpace(r.FormValue("name")), Permissions: permissions}, nil
}

func rolesTable(items []roles.Role) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		perms := strings.Join(item.Permissions, ", ")
		if perms == "" {
			perms = "—"
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(`<a href="/admin/roles/`+escAttr(item.ID)+`">`+esc(item.Name)+`</a>`),
			template.HTML(esc(perms)),
			template.HTML(statusBadge(item.Status)),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Name", "Permissions", "Status"}, rows.String())
}

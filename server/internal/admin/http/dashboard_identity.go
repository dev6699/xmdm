package adminhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/policy"
)

func (d *dashboard) users(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Users")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Identity.ListUsers(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Users", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	roles, err := d.loadRolesForUsers(r.Context(), items)
	if err != nil {
		d.renderPageError(w, r, session, "Users", err)
		return
	}
	roleOptions, err := d.loadRolesForSelect(r.Context(), "")
	if err != nil {
		d.renderPageError(w, r, session, "Users", err)
		return
	}
	data := pageData{
		Title:    "Users",
		Subtitle: "Manage operator accounts and role bindings.",
		Items:    withPager(usersTable(items, roleNameByID(roles)), pagerHTML(r, page, limit, hasNext)),
	}
	if d.canWrite(session) {
		data.Forms = []formData{{
			Title:  "Create user",
			Action: "/admin/users/create",
			Fields: []fieldData{
				{Name: "email", Label: "Email", Type: "email", Placeholder: "operator@example.com", Required: true},
				{Name: "password", Label: "Password", Type: "password", Placeholder: "password", Required: true},
				{Name: "roleId", Label: "Role", Type: "select", Placeholder: "Select a role", Options: allRoleOptions(roleOptions), Required: true},
			},
			Submit: "Create user",
		}}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) userDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "User Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, err := d.deps.Identity.GetUser(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "User Detail", err)
		return
	}
	id := found.ID
	roles, err := d.loadRolesForSelect(r.Context(), found.RoleID)
	if err != nil {
		d.renderPageError(w, r, session, "User Detail", err)
		return
	}
	data := pageData{
		Title:    "User Detail",
		Subtitle: "Edit the operator account or retire it from the active roster.",
		Items:    panelSectionHTML("", "Current user", template.HTML(pre(found))),
	}
	if d.canWrite(session) {
		if found.Status == "active" {
			data.Forms = []formData{{
				Title:  "Update user",
				Action: "/admin/users/" + id + "/update",
				Fields: []fieldData{
					{Name: "email", Label: "Email", Type: "email", Value: found.Email, Placeholder: "operator@example.com", Required: true},
					{Name: "password", Label: "Password", Type: "password", Placeholder: "leave blank to keep the current password"},
					{Name: "roleId", Label: "Role", Type: "select", Value: found.RoleID, Placeholder: "Select a role", Options: allRoleOptions(roles), Required: true},
				},
				Submit: "Update user",
			}, {
				Title:  "Retire user",
				Action: "/admin/users/" + id + "/retire",
				Submit: "Retire user",
				Danger: true,
			}}
		}
	}
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) createUser(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Users") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/users", "invalid form")
		return
	}
	passwordHash, err := identity.HashPassword(r.FormValue("password"))
	if err != nil {
		d.redirectError(w, r, "/admin/users", err.Error())
		return
	}
	rec, err := d.deps.Identity.CreateUser(r.Context(), d.deps.TenantID, identity.UserUpsert{Email: strings.TrimSpace(r.FormValue("email")), PasswordHash: passwordHash, RoleID: strings.TrimSpace(r.FormValue("roleId"))})
	if err != nil {
		d.redirectError(w, r, "/admin/users", err.Error())
		return
	}
	d.recordAudit(r, "create", "users", rec.ID, map[string]any{"email": rec.Email, "roleId": rec.RoleID})
	d.redirectOK(w, r, "/admin/users", "user created")
}

func (d *dashboard) updateUser(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Users") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), "invalid form")
		return
	}
	passwordHash, err := identity.HashPassword(r.FormValue("password"))
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	rec, err := d.deps.Identity.UpdateUser(r.Context(), d.deps.TenantID, r.PathValue("id"), identity.UserUpsert{Email: strings.TrimSpace(r.FormValue("email")), PasswordHash: passwordHash, RoleID: strings.TrimSpace(r.FormValue("roleId"))})
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	if rec.Status != "active" {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), "conflict")
		return
	}
	d.recordAudit(r, "update", "users", rec.ID, map[string]any{"email": rec.Email, "roleId": rec.RoleID})
	d.redirectOK(w, r, "/admin/users/"+rec.ID, "user updated")
}

func (d *dashboard) retireUser(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Users") {
		return
	}
	rec, err := d.deps.Identity.RetireUser(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "users", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/users", "user retired")
}

func (d *dashboard) roles(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Roles")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Identity.ListRoles(r.Context(), d.deps.TenantID, params)
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
	found, err := d.deps.Identity.GetRole(r.Context(), d.deps.TenantID, r.PathValue("id"))
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
	rec, err := d.deps.Identity.CreateRole(r.Context(), d.deps.TenantID, req)
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
	rec, err := d.deps.Identity.UpdateRole(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
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
	rec, err := d.deps.Identity.RetireRole(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/roles/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "roles", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/roles", "role retired")
}

func (d *dashboard) groups(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Groups")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Groups.ListGroups(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Groups", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Groups",
		Subtitle: "Define device cohorts and inspect the devices assigned to each cohort.",
		Forms:    []formData{{Title: "Create group", Action: "/admin/groups/create", Fields: []fieldData{{Name: "name", Label: "Name", Type: "text", Placeholder: "Field Devices", Required: true}}, Submit: "Create group"}},
		Items:    withPager(groupsTable(items), pagerHTML(r, page, limit, hasNext)),
	})
}

func (d *dashboard) createGroup(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Groups") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/groups", "invalid form")
		return
	}
	rec, err := d.deps.Groups.CreateGroup(r.Context(), d.deps.TenantID, group.GroupUpsert{Name: strings.TrimSpace(r.FormValue("name"))})
	if err != nil {
		d.redirectError(w, r, "/admin/groups", err.Error())
		return
	}
	d.recordAudit(r, "create", "groups", rec.ID, map[string]any{"name": rec.Name})
	d.redirectOK(w, r, "/admin/groups", "group created")
}

func (d *dashboard) groupDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Group Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, params := listPaginationParamsForKeys(r, "memberDevicesPage", "memberDevicesLimit", pagination.DefaultLimit)
	found, devices, policies, err := d.loadGroupDetail(r.Context(), r.PathValue("id"), params)
	if err != nil {
		d.renderPageError(w, r, session, "Group Detail", err)
		return
	}
	data := d.groupDetailPageData(r, session, found, devices, policies)
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) updateGroup(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Groups") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/groups/"+r.PathValue("id"), "invalid form")
		return
	}
	rec, err := d.deps.Groups.UpdateGroup(r.Context(), d.deps.TenantID, r.PathValue("id"), group.GroupUpsert{Name: strings.TrimSpace(r.FormValue("name"))})
	if err != nil {
		d.redirectError(w, r, "/admin/groups/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "groups", rec.ID, map[string]any{"name": rec.Name})
	d.redirectOK(w, r, "/admin/groups/"+rec.ID, "group updated")
}

func (d *dashboard) retireGroup(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Groups") {
		return
	}
	rec, err := d.deps.Groups.RetireGroup(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/groups", err.Error())
		return
	}
	d.recordAudit(r, "retire", "groups", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/groups", "group retired")
}

func (d *dashboard) permissionCatalog() []auth.Permission {
	if d.deps.PluginManager != nil {
		if perms := d.deps.PluginManager.PermissionCatalog(); len(perms) > 0 {
			return perms
		}
	}
	return auth.AllPermissions()
}

func (d *dashboard) loadRolesForSelect(ctx context.Context, selectedID string) ([]identity.Role, error) {
	items := []identity.Role{}
	if d.deps.Identity != nil {
		var err error
		items, err = d.deps.Identity.ListActiveRoles(ctx, d.deps.TenantID)
		if err != nil {
			return nil, err
		}
	}
	selectedID = strings.TrimSpace(selectedID)
	if selectedID == "" || containsRoleID(items, selectedID) {
		return items, nil
	}
	if d.deps.Identity == nil {
		return items, nil
	}
	found, err := d.deps.Identity.GetRole(ctx, d.deps.TenantID, selectedID)
	if err != nil {
		if err == httpx.ErrNotFound {
			return items, nil
		}
		return nil, err
	}
	return append([]identity.Role{found}, items...), nil
}

func (d *dashboard) loadRolesForUsers(ctx context.Context, items []identity.User) ([]identity.Role, error) {
	if d.deps.Identity == nil {
		return []identity.Role{}, nil
	}
	ids := roleIDsForUsers(items)
	roles := make([]identity.Role, 0, len(ids))
	for _, id := range ids {
		rec, err := d.deps.Identity.GetRole(ctx, d.deps.TenantID, id)
		if err != nil {
			if err == httpx.ErrNotFound {
				continue
			}
			return nil, err
		}
		roles = append(roles, rec)
	}
	return roles, nil
}

func (d *dashboard) loadGroupDetail(ctx context.Context, id string, page pagination.Params) (*group.Group, []device.Device, []policy.Policy, error) {
	found, err := d.deps.Groups.GetGroup(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, nil, nil, err
	}
	devices := []device.Device{}
	if d.deps.Groups != nil {
		devices, err = d.deps.Groups.ListGroupDevices(ctx, d.deps.TenantID, found.ID, page)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	policies := []policy.Policy{}
	if d.deps.Policies != nil {
		policies, err = d.loadPoliciesForDevices(ctx, devices)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return &found, devices, policies, nil
}

func (d *dashboard) groupDetailPageData(r *http.Request, session *auth.Session, found *group.Group, devices []device.Device, policies []policy.Policy) pageData {
	body := string(panelSectionHTML("", "Current group", template.HTML(pre(found))))
	page, params := listPaginationParamsForKeys(r, "memberDevicesPage", "memberDevicesLimit", pagination.DefaultLimit)
	limit := params.Limit - 1
	items, hasNext := paginateItems(devices, limit)
	if len(devices) > 0 {
		body += string(panelSectionHTML("", "Member devices", withPager(devicesTable(items, policies), pagerHTMLForKeys(r, "memberDevicesPage", "memberDevicesLimit", page, limit, hasNext))))
	} else {
		body += string(panelMessageHTML("Member devices", "No devices are linked to this group."))
	}
	data := pageData{
		Title:    "Group Detail",
		Subtitle: "Review the cohort, then update or retire it from this page.",
		Items:    template.HTML(body),
	}
	if d.canWrite(session) && found.Status != "retired" {
		data.Forms = []formData{{
			Title:  "Update group",
			Action: "/admin/groups/" + found.ID + "/update",
			Fields: []fieldData{{Name: "name", Label: "Name", Type: "text", Value: found.Name, Placeholder: "Field Devices", Required: true}},
			Submit: "Update group",
		}, {
			Title:  "Retire group",
			Action: "/admin/groups/" + found.ID + "/retire",
			Submit: "Retire group",
			Danger: true,
		}}
	}
	return data
}

func allRoleOptions(items []identity.Role) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		if item.Status != "active" {
			label += " (" + item.Status + ")"
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func containsString(items []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range items {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func containsRoleID(items []identity.Role, id string) bool {
	id = strings.TrimSpace(id)
	for _, item := range items {
		if strings.TrimSpace(item.ID) == id {
			return true
		}
	}
	return false
}

func roleNameByID(items []identity.Role) map[string]identity.Role {
	roles := make(map[string]identity.Role, len(items))
	for _, item := range items {
		roles[item.ID] = item
	}
	return roles
}

func roleIDsForUsers(items []identity.User) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.RoleID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func roleUpsertFromForm(r *http.Request) (identity.RoleUpsert, error) {
	if err := r.ParseForm(); err != nil {
		return identity.RoleUpsert{}, err
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
				return identity.RoleUpsert{}, fmt.Errorf("invalid permissions json")
			}
		} else {
			permissions = append(permissions, raw)
		}
	}
	return identity.RoleUpsert{Name: strings.TrimSpace(r.FormValue("name")), Permissions: permissions}, nil
}

func usersTable(items []identity.User, roles map[string]identity.Role) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		roleLabel := item.RoleID
		if rec, ok := roles[item.RoleID]; ok {
			roleLabel = rec.Name
			if strings.TrimSpace(roleLabel) == "" {
				roleLabel = rec.ID
			}
			if rec.Status != "active" {
				roleLabel += " (" + rec.Status + ")"
			}
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(`<a href="/admin/users/`+escAttr(item.ID)+`">`+esc(item.Email)+`</a>`),
			template.HTML(esc(roleLabel)),
			template.HTML(statusBadge(item.Status)),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Email", "Role", "Status"}, rows.String())
}

func rolesTable(items []identity.Role) template.HTML {
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

func groupsTable(items []group.Group) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(`<a href="/admin/groups/`+escAttr(item.ID)+`">`+esc(label)+`</a>`),
			template.HTML(statusBadge(item.Status)),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Name", "Status"}, rows.String())
}

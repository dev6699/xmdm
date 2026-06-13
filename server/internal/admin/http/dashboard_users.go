package adminhttp

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/roles"
	"xmdm/server/internal/users"
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
	items, err := d.deps.Users.ListUsers(r.Context(), d.deps.TenantID, params)
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
	found, err := d.deps.Users.GetUser(r.Context(), d.deps.TenantID, r.PathValue("id"))
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
	passwordHash, err := users.HashPassword(r.FormValue("password"))
	if err != nil {
		d.redirectError(w, r, "/admin/users", err.Error())
		return
	}
	rec, err := d.deps.Users.CreateUser(r.Context(), d.deps.TenantID, users.UserUpsert{Email: strings.TrimSpace(r.FormValue("email")), PasswordHash: passwordHash, RoleID: strings.TrimSpace(r.FormValue("roleId"))})
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
	passwordHash, err := users.HashPassword(r.FormValue("password"))
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	rec, err := d.deps.Users.UpdateUser(r.Context(), d.deps.TenantID, r.PathValue("id"), users.UserUpsert{Email: strings.TrimSpace(r.FormValue("email")), PasswordHash: passwordHash, RoleID: strings.TrimSpace(r.FormValue("roleId"))})
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
	rec, err := d.deps.Users.RetireUser(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/users/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "users", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/users", "user retired")
}

func (d *dashboard) loadRolesForSelect(ctx context.Context, selectedID string) ([]roles.Role, error) {
	items := []roles.Role{}
	if d.deps.Roles != nil {
		var err error
		items, err = d.deps.Roles.ListActiveRoles(ctx, d.deps.TenantID)
		if err != nil {
			return nil, err
		}
	}
	selectedID = strings.TrimSpace(selectedID)
	if selectedID == "" || containsRoleID(items, selectedID) {
		return items, nil
	}
	if d.deps.Roles == nil {
		return items, nil
	}
	found, err := d.deps.Roles.GetRole(ctx, d.deps.TenantID, selectedID)
	if err != nil {
		if err == httpx.ErrNotFound {
			return items, nil
		}
		return nil, err
	}
	return append([]roles.Role{found}, items...), nil
}

func (d *dashboard) loadRolesForUsers(ctx context.Context, items []users.User) ([]roles.Role, error) {
	if d.deps.Roles == nil {
		return []roles.Role{}, nil
	}
	ids := roleIDsForUsers(items)
	rolesOut := make([]roles.Role, 0, len(ids))
	for _, id := range ids {
		rec, err := d.deps.Roles.GetRole(ctx, d.deps.TenantID, id)
		if err != nil {
			if err == httpx.ErrNotFound {
				continue
			}
			return nil, err
		}
		rolesOut = append(rolesOut, rec)
	}
	return rolesOut, nil
}

func roleNameByID(items []roles.Role) map[string]roles.Role {
	roles := make(map[string]roles.Role, len(items))
	for _, item := range items {
		roles[item.ID] = item
	}
	return roles
}

func roleIDsForUsers(items []users.User) []string {
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

func containsString(items []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range items {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func containsRoleID(items []roles.Role, id string) bool {
	id = strings.TrimSpace(id)
	for _, item := range items {
		if strings.TrimSpace(item.ID) == id {
			return true
		}
	}
	return false
}

func allRoleOptions(items []roles.Role) []optionData {
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

func usersTable(items []users.User, roles map[string]roles.Role) template.HTML {
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

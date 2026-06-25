package adminhttp

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/group"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/policy"
)

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
		deviceInfoByDevice, err := d.latestDeviceInfoByDeviceIDs(r.Context(), items)
		if err != nil {
			return pageData{
				Title:    "Group Detail",
				Subtitle: "Review the cohort, then update or retire it from this page.",
				Error:    err.Error(),
			}
		}
		body += string(panelSectionHTML("", "Member devices", withPager(devicesTable(items, policies, deviceInfoByDevice, true), pagerHTMLForKeys(r, "memberDevicesPage", "memberDevicesLimit", page, limit, hasNext))))
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

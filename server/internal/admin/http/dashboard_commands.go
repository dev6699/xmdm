package adminhttp

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/plugins"
)

func (d *dashboard) commands(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Commands")
	if !ok {
		return
	}
	devices := []device.Device{}
	if d.deps.Devices != nil {
		var err error
		devices, err = d.deps.Devices.ListActiveDevices(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Commands", err)
			return
		}
	}
	deviceMap := make(map[string]device.Device, len(devices))
	for _, item := range devices {
		deviceMap[item.ID] = item
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		var err error
		groups, err = d.deps.Groups.ListActiveGroups(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Commands", err)
			return
		}
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Commands.ListRecent(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Commands", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Commands",
		Subtitle: "Send commands to individual devices or device groups. Expired commands are shown as terminal records and never replay as fresh work.",
		Forms:    []formData{{Title: "Send command", Action: "/admin/commands/create", Fields: commandFields(session, d.deps.PluginManager, devices, groups), Submit: "Send command"}},
		Items:    withPager(commandsTable(items, deviceMap), pagerHTML(r, page, limit, hasNext)),
	})
}

func (d *dashboard) commandDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Commands")
	if !ok {
		return
	}
	if d.deps.Commands == nil {
		d.renderError(w, http.StatusNotFound, "Commands", "not found")
		return
	}
	commandID := strings.TrimSpace(r.PathValue("id"))
	found, err := d.deps.Commands.Get(r.Context(), d.deps.TenantID, commandID)
	if err != nil {
		if err == httpx.ErrNotFound {
			d.renderError(w, http.StatusNotFound, "Commands", "not found")
			return
		}
		d.renderPageError(w, r, session, "Commands", err)
		return
	}
	deviceRec := device.Device{}
	if d.deps.Devices != nil {
		deviceRec, err = d.deps.Devices.GetDevice(r.Context(), d.deps.TenantID, found.DeviceID)
		if err != nil && err != httpx.ErrNotFound {
			d.renderPageError(w, r, session, "Commands", err)
			return
		}
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Command Detail",
		Subtitle: "Inspect the queued command row, payload, delivery state, and device acknowledgement result.",
		Items:    commandSummary(found, deviceRec),
	})
}

func (d *dashboard) createCommand(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Commands") {
		return
	}
	session, ok := sessionFromRequest(r, d.svc)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	req, err := commandUpsertFromForm(r, func(commandType string) bool {
		return isAllowedCommandType(session, d.deps.PluginManager, commandType)
	})
	if err != nil {
		d.redirectError(w, r, "/admin/commands", err.Error())
		return
	}
	created, err := d.deps.Commands.Enqueue(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/commands", err.Error())
		return
	}
	for _, rec := range created {
		d.recordAudit(r, "create", "commands", rec.ID, map[string]any{"type": rec.Type, "status": rec.Status, "deviceId": rec.DeviceID})
	}
	d.redirectOK(w, r, "/admin/commands", fmt.Sprintf("%d command(s) queued", len(created)))
}

func commandSummary(found commands.Command, deviceRec device.Device) template.HTML {
	deviceLabel := found.DeviceID
	if deviceRec.ID != "" {
		deviceLabel = deviceRec.ID
		if strings.TrimSpace(deviceRec.Name) != "" {
			deviceLabel = deviceRec.Name
		}
		deviceLabel = `<a href="/admin/devices/` + escAttr(deviceRec.ID) + `">` + esc(deviceLabel) + `</a>`
	}
	expires := "—"
	if found.ExpiresAt != nil {
		expires = formatDashboardTime(*found.ExpiresAt)
	}
	acked := "—"
	if found.AckedAt != nil {
		acked = formatDashboardTime(*found.AckedAt)
	}
	transportSource := commandTransportSource(found)
	var b strings.Builder
	b.WriteString(`<div class="policy-summary">`)
	b.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	b.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	b.WriteString(summaryTextItem("ID", found.ID))
	b.WriteString(summaryTextItem("Type", found.Type))
	b.WriteString(summaryHTMLItem("Device", template.HTML(deviceLabel)))
	b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	b.WriteString(summaryTextItem("Expires", expires))
	b.WriteString(summaryTextItem("Acked", acked))
	if transportSource != "" {
		b.WriteString(summaryTextItem("Transport", transportSource))
	}
	b.WriteString(summaryWideHTMLItem("Payload", preStructuredOnly(found.Payload), ""))
	if len(found.Result) > 0 {
		b.WriteString(summaryWideHTMLItem("Result", preStructuredOnly(found.Result), ""))
	} else {
		b.WriteString(summaryTextItem("Result", "—"))
	}
	b.WriteString(`</div>`)
	b.WriteString(string(rawDataDetails("Raw command data", found)))
	return panelSectionHTML("", "Current command", template.HTML(b.String()))
}

func commandTransportSource(found commands.Command) string {
	if len(found.Result) == 0 {
		return ""
	}
	if raw, ok := found.Result["transportSource"].(string); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	details, ok := found.Result["details"].(map[string]any)
	if !ok {
		return ""
	}
	if raw, ok := details["transportSource"].(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

func commandFields(session *auth.Session, pluginManager *plugins.Manager, devices []device.Device, groups []group.Group) []fieldData {
	return []fieldData{
		{Name: "type", Label: "Command", Type: "select", Value: "ping", Required: true, Options: commandTypeOptions(session, pluginManager)},
		{Name: "targetType", Label: "Target type", Type: "select", Value: commands.TargetDevice, Required: true, Options: []optionData{{Value: commands.TargetDevice, Label: "Device"}, {Value: commands.TargetGroup, Label: "Group"}}},
		{Name: "targetDeviceId", Label: "Device", Type: "select", Placeholder: "Select device", Options: commandDeviceOptions(devices)},
		{Name: "targetGroupId", Label: "Group", Type: "select", Placeholder: "Select group", Options: commandGroupOptions(groups)},
		{Name: "payload", Label: "Payload JSON", Type: "textarea", Value: "{}"},
		{Name: "expiresAt", Label: "Expires at", Type: "datetime-local"},
	}
}

func commandTypeOptions(session *auth.Session, pluginManager *plugins.Manager) []optionData {
	options := make([]optionData, 0, len(commands.BuiltinTypes())+4)
	seen := map[string]struct{}{}
	for _, spec := range commands.BuiltinTypes() {
		if _, ok := seen[spec.Type]; ok {
			continue
		}
		seen[spec.Type] = struct{}{}
		options = append(options, optionData{Value: spec.Type, Label: spec.Label})
	}
	if pluginManager != nil && session != nil {
		for _, spec := range pluginManager.CommandTypesFor(session) {
			if _, ok := seen[spec.Type]; ok {
				continue
			}
			seen[spec.Type] = struct{}{}
			label := spec.Label
			if strings.TrimSpace(label) == "" {
				label = spec.Type
			}
			options = append(options, optionData{Value: spec.Type, Label: label})
		}
	}
	return options
}

func commandDeviceOptions(items []device.Device) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		if item.Status != device.StatusActive && item.Status != device.StatusEnrolled {
			continue
		}
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func commandGroupOptions(items []group.Group) []optionData {
	options := make([]optionData, 0, len(items))
	for _, item := range items {
		if item.Status != "active" {
			continue
		}
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		options = append(options, optionData{Value: item.ID, Label: label})
	}
	return options
}

func commandUpsertFromForm(r *http.Request, allowType func(string) bool) (commands.Upsert, error) {
	if err := r.ParseForm(); err != nil {
		return commands.Upsert{}, err
	}
	commandType := strings.TrimSpace(r.FormValue("type"))
	if commandType == "" {
		return commands.Upsert{}, fmt.Errorf("command type is required")
	}
	if allowType != nil && !allowType(commandType) {
		return commands.Upsert{}, fmt.Errorf("unsupported command type")
	}
	var payload map[string]any
	rawPayload := strings.TrimSpace(r.FormValue("payload"))
	if rawPayload != "" {
		if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
			return commands.Upsert{}, fmt.Errorf("invalid payload json")
		}
	}
	var expiresAt *time.Time
	if raw := strings.TrimSpace(r.FormValue("expiresAt")); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02T15:04", raw, time.Local)
		if err != nil {
			parsed, err = time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local)
		}
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, raw)
		}
		if err != nil {
			return commands.Upsert{}, fmt.Errorf("invalid expiry")
		}
		expiresAt = &parsed
	}
	targetType := strings.TrimSpace(r.FormValue("targetType"))
	if targetType == "" {
		targetType = commands.TargetDevice
	}
	switch targetType {
	case commands.TargetDevice, commands.TargetGroup:
	default:
		return commands.Upsert{}, fmt.Errorf("broadcast commands are disabled")
	}
	return commands.Upsert{Type: commandType, Payload: payload, ExpiresAt: expiresAt, Target: commands.Target{Type: targetType, DeviceID: strings.TrimSpace(r.FormValue("targetDeviceId")), GroupID: strings.TrimSpace(r.FormValue("targetGroupId"))}}, nil
}

func commandsTable(items []commands.Command, devices map[string]device.Device) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		deviceLabel := item.DeviceID
		if rec, ok := devices[item.DeviceID]; ok {
			deviceLabel = rec.Name
			if strings.TrimSpace(deviceLabel) == "" {
				deviceLabel = rec.ID
			}
			deviceLabel = `<a href="/admin/devices/` + escAttr(rec.ID) + `">` + esc(deviceLabel) + `</a>`
		} else {
			deviceLabel = esc(deviceLabel)
		}
		commandLink := `<a href="/admin/commands/` + escAttr(item.ID) + `">` + esc(item.ID) + `</a>`
		expires := "—"
		if item.ExpiresAt != nil {
			expires = formatDashboardTime(*item.ExpiresAt)
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(commandLink),
			template.HTML(esc(item.Type)),
			template.HTML(deviceLabel),
			template.HTML(statusBadge(item.Status)),
			template.HTML(esc(expires)),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Type", "Device", "Status", "Expires"}, rows.String())
}

func isAllowedCommandType(session *auth.Session, pluginManager *plugins.Manager, commandType string) bool {
	commandType = strings.TrimSpace(commandType)
	if commandType == "" {
		return false
	}
	if commands.IsBuiltinType(commandType) {
		return true
	}
	if pluginManager == nil || session == nil {
		return false
	}
	_, ok := pluginManager.SupportsCommandType(session, commandType)
	return ok
}

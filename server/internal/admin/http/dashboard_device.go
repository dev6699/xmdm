package adminhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	apps "xmdm/server/internal/apps"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/enrollment"
	enrollmenthttp "xmdm/server/internal/enrollment/http"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/logs"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/policy"
	"xmdm/server/internal/telemetry"

	qrcode "github.com/skip2/go-qrcode"
)

func (d *dashboard) devices(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Devices")
	if !ok {
		return
	}
	healthFilter := deviceHealthFilterFromRequest(r)
	searchQuery := strings.TrimSpace(r.URL.Query().Get("search"))
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items := []device.Device{}
	if d.deps.Devices != nil {
		var err error
		items, err = d.deps.Devices.ListDevicesByFilter(r.Context(), d.deps.TenantID, params, device.DeviceListFilter{
			Health:    healthFilter,
			NameQuery: searchQuery,
		})
		if err != nil {
			d.renderPageError(w, r, session, "Devices", err)
			return
		}
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	policies, err := d.loadPoliciesForDevices(r.Context(), items)
	if err != nil {
		d.renderPageError(w, r, session, "Devices", err)
		return
	}
	policyOptions, err := d.loadPoliciesForSelect(r.Context(), "")
	if err != nil {
		d.renderPageError(w, r, session, "Devices", err)
		return
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		groups, err = d.deps.Groups.ListActiveGroups(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Devices", err)
			return
		}
	}
	var telemetryByDevice map[string]telemetry.Record
	if d.deps.Telemetry != nil && len(items) > 0 {
		deviceIDs := make([]string, 0, len(items))
		for _, item := range items {
			if strings.TrimSpace(item.ID) != "" {
				deviceIDs = append(deviceIDs, item.ID)
			}
		}
		if len(deviceIDs) > 0 {
			telemetryByDevice, err = d.deps.Telemetry.ListLatestByDeviceIDs(r.Context(), d.deps.TenantID, deviceIDs)
			if err != nil {
				d.renderPageError(w, r, session, "Devices", err)
				return
			}
		}
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Devices",
		Subtitle: "Track enrolled, pending, and retired devices, then update policy or group assignments.",
		Forms: []formData{{
			Title:  "Create device",
			Action: "/admin/devices/create",
			Fields: []fieldData{
				{Name: "name", Label: "Display name", Type: "text", Placeholder: "warehouse-tablet-001", Required: true},
				{Name: "policyId", Label: "Policy", Type: "select", Placeholder: "Select a policy", Options: allPolicyOptions(policyOptions), Required: true},
				{Name: "groupIds", Label: "Groups", Type: "multiselect", Placeholder: "Select one or more groups", Options: activeGroupOptions(groups)},
			},
			Help:   "",
			Submit: "Create device",
		}},
		Items: withPager(template.HTML(deviceFilterBarHTML(r, healthFilter, searchQuery)+string(devicesTable(items, policies, telemetryByDevice, true))), pagerHTML(r, page, limit, hasNext)),
	})
}

func (d *dashboard) deviceDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Device Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	found, err := d.deps.Devices.GetDevice(r.Context(), d.deps.TenantID, id)
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	policies, err := d.loadPoliciesForSelect(r.Context(), firstPolicyID(found.PolicyID))
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		groups, err = d.deps.Groups.ListActiveGroups(r.Context(), d.deps.TenantID)
		if err != nil {
			d.renderPageError(w, r, session, "Device Detail", err)
			return
		}
	}
	data := d.deviceDetailPageData(r, session, &found, policies, groups, "")
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) deviceEnrollmentQR(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Device Detail") {
		return
	}
	session, ok := sessionFromRequest(r, d.svc)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	id := r.PathValue("id")
	found, policies, groups, err := d.loadDeviceDetail(r.Context(), id)
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	if found.Status != device.StatusPending {
		d.renderPageError(w, r, session, "Device Detail", fmt.Errorf("device must be pending"))
		return
	}
	state, err := d.deviceEnrollmentQRState(r.Context(), firstNonEmpty(found.ID, found.Name))
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	result, err := d.generateEnrollmentQR(r.Context(), state)
	if err != nil {
		d.renderPageError(w, r, session, "Device Detail", err)
		return
	}
	d.recordAudit(r, "create", "enrollment_tokens", result.Issued.ID, map[string]any{"expiresAt": result.Issued.ExpiresAt, "deviceId": firstNonEmpty(found.ID, found.Name)})
	data := d.deviceDetailPageData(r, session, found, policies, groups, enrollmentQRCallout(result))
	data.Flash = "QR generated"
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) loadDeviceDetail(ctx context.Context, id string) (*device.Device, []policy.Policy, []group.Group, error) {
	found, err := d.deps.Devices.GetDevice(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, nil, nil, err
	}
	policies, err := d.loadPoliciesForSelect(ctx, firstPolicyID(found.PolicyID))
	if err != nil {
		return nil, nil, nil, err
	}
	groups := []group.Group{}
	if d.deps.Groups != nil {
		groups, err = d.deps.Groups.ListActiveGroups(ctx, d.deps.TenantID)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return &found, policies, groups, nil
}

func (d *dashboard) loadPoliciesForSelect(ctx context.Context, selectedID string) ([]policy.Policy, error) {
	items := []policy.Policy{}
	if d.deps.Policies != nil {
		var err error
		items, err = d.deps.Policies.ListActivePolicies(ctx, d.deps.TenantID)
		if err != nil {
			return nil, err
		}
	}
	selectedID = strings.TrimSpace(selectedID)
	if selectedID == "" || containsPolicyID(items, selectedID) {
		return items, nil
	}
	if d.deps.Policies == nil {
		return items, nil
	}
	found, err := d.deps.Policies.GetPolicy(ctx, d.deps.TenantID, selectedID)
	if err != nil {
		if err == httpx.ErrNotFound {
			return items, nil
		}
		return nil, err
	}
	return append([]policy.Policy{found}, items...), nil
}

func (d *dashboard) loadPoliciesForDevices(ctx context.Context, items []device.Device) ([]policy.Policy, error) {
	if d.deps.Policies == nil {
		return []policy.Policy{}, nil
	}
	ids := policyIDsForDevices(items)
	policies := make([]policy.Policy, 0, len(ids))
	for _, id := range ids {
		rec, err := d.deps.Policies.GetPolicy(ctx, d.deps.TenantID, id)
		if err != nil {
			if err == httpx.ErrNotFound {
				continue
			}
			return nil, err
		}
		policies = append(policies, rec)
	}
	return policies, nil
}

func (d *dashboard) deviceDetailPageData(r *http.Request, session *auth.Session, found *device.Device, policies []policy.Policy, groups []group.Group, callout template.HTML) pageData {
	body := string(panelSectionHTML("", "Current device", template.HTML(pre(found))))
	policyMap := policyNameByID(policies)
	policyID := firstPolicyID(found.PolicyID)
	if policyID != "" {
		rec, ok := policyMap[policyID]
		if !ok && d.deps.Policies != nil {
			if exact, err := d.deps.Policies.GetPolicy(r.Context(), d.deps.TenantID, policyID); err == nil {
				rec = exact
				ok = true
			}
		}
		if ok {
			var b strings.Builder
			label := rec.Name
			if strings.TrimSpace(label) == "" {
				label = rec.ID
			}
			b.WriteString(`<div class="policy-summary">`)
			b.WriteString(summaryTextItem("Created", formatDashboardTime(rec.CreatedAt)))
			b.WriteString(summaryTextItem("ID", rec.ID))
			fmt.Fprintf(&b, `<div class="policy-summary-item"><div class="policy-summary-label">Name</div><div class="policy-summary-value"><a href="/admin/policies/%s">%s</a></div></div>`, escAttr(rec.ID), esc(label))
			b.WriteString(summaryTextItem("Version", strconv.Itoa(rec.Version)))
			b.WriteString(summaryHTMLItem("Kiosk mode", template.HTML(boolBadge(rec.KioskMode, "enabled", "disabled"))))
			b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(rec.Status))))
			body += string(collapsiblePanelHTML("Active policy", false, template.HTML(b.String())))
		} else {
			body += string(collapsiblePanelHTML("Active policy", false, template.HTML(`<p class="muted">`+esc(policyID)+`</p>`)))
		}
	} else {
		body += string(collapsiblePanelHTML("Active policy", false, template.HTML(`<p class="muted">No policy linked.</p>`)))
	}
	if len(found.GroupIDs) > 0 {
		groupMap := groupNameByID(groups)
		var b strings.Builder
		b.WriteString(`<div class="policy-summary">`)
		for _, groupID := range found.GroupIDs {
			label := groupID
			if rec, ok := groupMap[groupID]; ok && strings.TrimSpace(rec.Name) != "" {
				label = rec.Name
			} else if d.deps.Groups != nil {
				if exact, err := d.deps.Groups.GetGroup(r.Context(), d.deps.TenantID, groupID); err == nil && strings.TrimSpace(exact.Name) != "" {
					label = exact.Name
				}
			}
			b.WriteString(summaryTextItem("Group", label))
		}
		body += string(collapsiblePanelHTML("Assigned groups", false, template.HTML(b.String())))
	} else {
		body += string(collapsiblePanelHTML("Assigned groups", false, template.HTML(`<p class="muted">No groups linked.</p>`)))
	}
	body += string(collapsiblePanelHTML("Config preview", false, d.deviceConfigPreviewBody(r.Context(), found)))
	deviceKey := firstNonEmpty(found.ID, found.Name)
	deviceRowID := found.RecordID()
	if d.deps.Logs != nil {
		page, params := listPaginationParamsForKeys(r, "logsPage", "logsLimit", pagination.DefaultLimit)
		limit := params.Limit - 1
		rows, err := d.deps.Logs.Search(r.Context(), d.deps.TenantID, logs.SearchFilter{DeviceID: deviceKey, Limit: params.Limit, Offset: params.Offset, Pagination: params})
		if err == nil {
			items, hasNext := paginateItems(rows, limit)
			body += string(collapsiblePanelHTML("Recent logs", false, withPager(template.HTML(pre(items)), pagerHTMLForKeys(r, "logsPage", "logsLimit", page, limit, hasNext))))
		}
	}
	if d.deps.DeviceInfo != nil {
		page, params := listPaginationParamsForKeys(r, "deviceInfoPage", "deviceInfoLimit", pagination.DefaultLimit)
		limit := params.Limit - 1
		rows, err := d.deps.DeviceInfo.Search(r.Context(), d.deps.TenantID, deviceinfo.SearchFilter{DeviceID: deviceKey, Limit: params.Limit, Offset: params.Offset, Pagination: params})
		if err == nil {
			items, hasNext := paginateItems(rows, limit)
			body += string(collapsiblePanelHTML("Recent device info", false, withPager(template.HTML(pre(items)), pagerHTMLForKeys(r, "deviceInfoPage", "deviceInfoLimit", page, limit, hasNext))))
		}
	}
	if d.deps.Commands != nil {
		page, params := listPaginationParamsForKeys(r, "commandsPage", "commandsLimit", pagination.DefaultLimit)
		limit := params.Limit - 1
		rows, err := d.deps.Commands.ListPending(r.Context(), d.deps.TenantID, deviceRowID, params)
		if err == nil {
			items, hasNext := paginateItems(rows, limit)
			body += string(collapsiblePanelHTML("Pending commands", false, withPager(template.HTML(pre(items)), pagerHTMLForKeys(r, "commandsPage", "commandsLimit", page, limit, hasNext))))
		}
	}
	if d.deps.PluginManager != nil {
		actions := d.deps.PluginManager.DeviceActionsFor(session, found.ID)
		if len(actions) > 0 {
			body += string(pluginDeviceActionsSection(actions))
		}
	}
	data := pageData{
		Title:    "Device Detail",
		Subtitle: "Edit the device label or retire it from active use.",
		Items:    template.HTML(body),
	}
	if d.canWrite(session) && found.Status != device.StatusRetired && found.Status != device.StatusWiped {
		policyID := ""
		if found.PolicyID != nil {
			policyID = *found.PolicyID
		}
		options := allPolicyOptions(policies)
		if policyID != "" {
			foundPolicy := false
			for _, option := range options {
				if option.Value == policyID {
					foundPolicy = true
					break
				}
			}
			if !foundPolicy {
				options = append(options, optionData{Value: policyID, Label: policyID})
			}
		}
		groupOptions := activeGroupOptions(groups)
		if len(found.GroupIDs) > 0 {
			for _, groupID := range found.GroupIDs {
				foundGroup := false
				for _, option := range groupOptions {
					if option.Value == groupID {
						foundGroup = true
						break
					}
				}
				if !foundGroup {
					groupOptions = append(groupOptions, optionData{Value: groupID, Label: groupID})
				}
			}
		}
		data.Forms = []formData{{
			Title:  "Update device",
			Action: "/admin/devices/" + found.ID + "/update",
			Fields: []fieldData{
				{Name: "name", Label: "Display name", Type: "text", Value: found.Name, Placeholder: "warehouse-tablet-001", Required: true},
				{Name: "policyId", Label: "Policy", Type: "select", Value: policyID, Placeholder: "Select a policy", Options: options, Required: true},
				{Name: "groupIds", Label: "Groups", Type: "multiselect", Values: append([]string(nil), found.GroupIDs...), Placeholder: "Select one or more groups", Options: groupOptions},
			},
			Submit: "Update device",
		}, {
			Title:  "Retire device",
			Action: "/admin/devices/" + found.ID + "/retire",
			Submit: "Retire device",
			Danger: true,
		}}
	}
	if d.canWrite(session) && found.Status == device.StatusPending {
		form := deviceEnrollmentQRForm(found.ID)
		form.After = callout
		data.Forms = append(data.Forms, form)
	}
	return data
}

type enrollmentQRFormState struct {
	ServerURL       string
	TTL             string
	ComponentName   string
	PackageURL      string
	PackageChecksum string
	DeviceID        string
}

type enrollmentQRResult struct {
	Issued      enrollment.IssuedToken
	PayloadJSON string
	PNGDataURL  string
}

const (
	deviceEnrollmentTTL        = "2h"
	defaultAgentAppPackage     = "com.xmdm.launcher"
	agentEnrollmentPackagePath = "/api/v1/enrollment/agent.apk"
)

func (d *dashboard) deviceEnrollmentQRState(ctx context.Context, deviceID string) (enrollmentQRFormState, error) {
	serverURL, err := normalizedPublicServerURL(d.deps.ServerPublicURL)
	if err != nil {
		return enrollmentQRFormState{}, err
	}
	appPackage := firstNonEmpty(strings.TrimSpace(d.deps.AgentAppPackage), defaultAgentAppPackage)
	appRecord, latest, err := latestPublishedAgentAppVersion(ctx, d.deps.Apps, d.deps.TenantID, appPackage)
	if err != nil {
		if err == httpx.ErrNotFound {
			return enrollmentQRFormState{}, fmt.Errorf("agent managed app %q must have a latest published APK version", appPackage)
		}
		return enrollmentQRFormState{}, err
	}
	packageURL := strings.TrimRight(serverURL, "/") + agentEnrollmentPackagePath
	return enrollmentQRFormState{
		ServerURL:       serverURL,
		TTL:             deviceEnrollmentTTL,
		ComponentName:   appRecord.PackageName + "/.AdminReceiver",
		PackageURL:      packageURL,
		PackageChecksum: latest.Checksum,
		DeviceID:        deviceID,
	}, nil
}

func normalizedPublicServerURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("server public url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid server public url")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func latestPublishedAgentAppVersion(ctx context.Context, store apps.Repository, tenantID, agentAppPackage string) (apps.App, apps.Version, error) {
	if store == nil {
		return apps.App{}, apps.Version{}, httpx.ErrNotFound
	}
	packageName := firstNonEmpty(strings.TrimSpace(agentAppPackage), defaultAgentAppPackage)
	item, err := store.GetAppByPackageName(ctx, tenantID, packageName)
	if err != nil {
		return apps.App{}, apps.Version{}, err
	}
	if item.Status != apps.StatusActive {
		return apps.App{}, apps.Version{}, httpx.ErrNotFound
	}
	latest, err := store.GetLatestPublishedVersion(ctx, tenantID, item.ID)
	if err != nil {
		return apps.App{}, apps.Version{}, err
	}
	if latest.ArtifactID == nil || strings.TrimSpace(latest.Checksum) == "" {
		return apps.App{}, apps.Version{}, httpx.ErrNotFound
	}
	return item, latest, nil
}

func deviceEnrollmentQRForm(deviceID string) formData {
	return formData{
		Title:  "Generate enrollment QR",
		Action: "/admin/devices/" + deviceID + "/enrollment/qr",
		Help:   "",
		Submit: "Generate QR",
	}
}

func (d *dashboard) generateEnrollmentQR(ctx context.Context, state enrollmentQRFormState) (enrollmentQRResult, error) {
	ttl, err := time.ParseDuration(strings.TrimSpace(state.TTL))
	if err != nil || ttl <= 0 {
		return enrollmentQRResult{}, fmt.Errorf("invalid ttl")
	}
	issued, err := d.deps.Enrollment.IssueToken(ctx, d.deps.TenantID, time.Now().UTC().Add(ttl))
	if err != nil {
		return enrollmentQRResult{}, err
	}
	payload, err := buildEnrollmentQRPayload(state, issued.Secret)
	if err != nil {
		return enrollmentQRResult{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return enrollmentQRResult{}, err
	}
	png, err := qrcode.Encode(string(raw), qrcode.Medium, 512)
	if err != nil {
		return enrollmentQRResult{}, err
	}
	result := enrollmentQRResult{
		Issued:      issued,
		PayloadJSON: string(raw),
		PNGDataURL:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	}
	return result, nil
}

func buildEnrollmentQRPayload(state enrollmentQRFormState, token string) (map[string]any, error) {
	serverURL := strings.TrimSpace(state.ServerURL)
	packageURL := strings.TrimSpace(state.PackageURL)
	packageChecksum := strings.TrimSpace(state.PackageChecksum)
	deviceID := strings.TrimSpace(state.DeviceID)
	if serverURL == "" || packageURL == "" || packageChecksum == "" || deviceID == "" {
		return nil, fmt.Errorf("server url, package url, package checksum, and device id are required")
	}
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid server url")
	}
	extras := map[string]any{
		"com.xmdm.BASE_URL":         parsed.String(),
		"com.xmdm.ENROLLMENT_TOKEN": token,
		"com.xmdm.DEVICE_ID":        deviceID,
	}
	return map[string]any{
		"android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME":            firstNonEmpty(strings.TrimSpace(state.ComponentName), "com.xmdm.launcher/.AdminReceiver"),
		"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION": packageURL,
		"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM":          packageChecksum,
		"android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED":          true,
		"android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE":                    extras,
	}, nil
}

func enrollmentQRCallout(result enrollmentQRResult) template.HTML {
	var b strings.Builder
	b.WriteString(`<div style="display:grid;gap:1rem">`)
	b.WriteString(`<section><h3>QR JSON</h3>`)
	fmt.Fprintf(&b, `<pre class="qr-json">%s</pre>`, template.HTMLEscapeString(result.PayloadJSON))
	b.WriteString(`</section>`)
	fmt.Fprintf(&b, `<section><h3>QR preview</h3><img alt="Enrollment QR preview" style="max-width:320px;width:100%%;height:auto;border:1px solid var(--border);border-radius:.5rem;background:#fff;padding:.5rem" src="%s"></section>`, escAttr(result.PNGDataURL))
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func allPolicyOptions(items []policy.Policy) []optionData {
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

func activeGroupOptions(items []group.Group) []optionData {
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

func groupNameByID(items []group.Group) map[string]group.Group {
	groups := make(map[string]group.Group, len(items))
	for _, item := range items {
		groups[item.ID] = item
	}
	return groups
}

func policyNameByID(items []policy.Policy) map[string]policy.Policy {
	policies := make(map[string]policy.Policy, len(items))
	for _, item := range items {
		policies[item.ID] = item
	}
	return policies
}

func containsPolicyID(items []policy.Policy, id string) bool {
	id = strings.TrimSpace(id)
	for _, item := range items {
		if strings.TrimSpace(item.ID) == id {
			return true
		}
	}
	return false
}

func firstPolicyID(policyID *string) string {
	if policyID == nil {
		return ""
	}
	return strings.TrimSpace(*policyID)
}

func policyIDsForDevices(items []device.Device) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.PolicyID == nil {
			continue
		}
		id := strings.TrimSpace(*item.PolicyID)
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

func (d *dashboard) deviceConfigPreviewBody(ctx context.Context, found *device.Device) template.HTML {
	if found == nil || found.PolicyID == nil || strings.TrimSpace(*found.PolicyID) == "" {
		return ""
	}
	if d.deps.Policies == nil {
		return template.HTML(`<p class="muted">Preview unavailable until policy data is configured.</p>`)
	}
	deviceKey := firstNonEmpty(found.ID, found.Name)
	config, err := enrollmenthttp.BuildConfigSnapshot(ctx, d.deps.Policies, d.deps.Apps, d.deps.ManagedFiles, d.deps.Artifacts, d.deps.Certificates, d.deps.TenantID, deviceKey, found.PolicyID, found.BootstrapExtras, d.deps.Runtime)
	if err != nil {
		return template.HTML(`<p class="muted">Preview unavailable: ` + esc(err.Error()) + `</p>`)
	}
	preview := struct {
		Version      string                           `json:"version"`
		Runtime      enrollment.RuntimeSnapshot       `json:"runtime,omitempty"`
		Device       enrollment.DeviceSnapshot        `json:"device"`
		Policy       enrollment.PolicySnapshot        `json:"policy"`
		Apps         []enrollment.AppSnapshot         `json:"apps"`
		Files        []enrollment.ManagedFileSnapshot `json:"files"`
		Certificates []enrollment.CertificateSnapshot `json:"certificates"`
	}{
		Version:      config.Version,
		Runtime:      config.Runtime,
		Device:       config.Device,
		Policy:       config.Policy,
		Apps:         config.Apps,
		Files:        config.Files,
		Certificates: config.Certificates,
	}
	return template.HTML(pre(preview))
}

func (d *dashboard) createDevice(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Devices") {
		return
	}
	req, err := deviceUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/devices", err.Error())
		return
	}
	if strings.TrimSpace(req.PolicyID) == "" {
		d.redirectError(w, r, "/admin/devices", "policy is required")
		return
	}
	if d.deps.Groups == nil && len(req.GroupIDs) > 0 {
		d.redirectError(w, r, "/admin/devices", "groups are unavailable")
		return
	}
	if strings.TrimSpace(req.SecretHash) == "" {
		secret, err := enrollment.NewTokenSecret()
		if err != nil {
			d.redirectError(w, r, "/admin/devices", err.Error())
			return
		}
		req.SecretHash = enrollment.HashToken(secret)
	}
	rec, err := d.deps.Devices.CreateDevice(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/devices", err.Error())
		return
	}
	d.recordAudit(r, "create", "devices", rec.ID, map[string]any{"name": rec.Name, "deviceId": rec.ID, "policyId": firstPolicyID(rec.PolicyID), "groupIds": req.GroupIDs})
	d.redirectOK(w, r, "/admin/devices", "device created")
}

func (d *dashboard) updateDevice(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Devices") {
		return
	}
	req, err := deviceUpsertFromForm(r)
	if err != nil {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), err.Error())
		return
	}
	if strings.TrimSpace(req.PolicyID) == "" {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), "policy is required")
		return
	}
	if strings.TrimSpace(req.SecretHash) != "" {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), "device secret rotation is disabled")
		return
	}
	found, err := d.deps.Devices.GetDevice(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), err.Error())
		return
	}
	if found.Status == device.StatusRetired || found.Status == device.StatusWiped {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), "conflict")
		return
	}
	rec, err := d.deps.Devices.UpdateDevice(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/devices/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "devices", rec.ID, map[string]any{"name": rec.Name, "deviceId": rec.ID, "policyId": firstPolicyID(rec.PolicyID)})
	d.redirectOK(w, r, "/admin/devices/"+rec.ID, "device updated")
}

func (d *dashboard) retireDevice(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Devices") {
		return
	}
	rec, err := d.deps.Devices.RetireDevice(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/devices", err.Error())
		return
	}
	d.recordAudit(r, "retire", "devices", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/devices", "device retired")
}

func deviceUpsertFromForm(r *http.Request) (device.DeviceUpsert, error) {
	if err := r.ParseForm(); err != nil {
		return device.DeviceUpsert{}, err
	}
	return device.DeviceUpsert{Name: strings.TrimSpace(r.FormValue("name")), SecretHash: hashPlainValue(r.FormValue("deviceSecret")), PolicyID: strings.TrimSpace(r.FormValue("policyId")), GroupIDs: splitFormValues(r.Form["groupIds"])}, nil
}

func splitFormValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func hashPlainValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return enrollment.HashToken(value)
}

const (
	lowBatteryThresholdPercent = 20
	staleOnlineThreshold       = 24 * time.Hour
)

func devicesTable(items []device.Device, policies []policy.Policy, telemetryByDevice map[string]telemetry.Record, includeTelemetryColumns bool) template.HTML {
	var rows strings.Builder
	policyMap := policyNameByID(policies)
	for _, item := range items {
		policyID := ""
		if item.PolicyID != nil {
			policyID = *item.PolicyID
		}
		policyLabel := policyID
		if rec, ok := policyMap[policyID]; ok {
			policyLabel = rec.Name
			if rec.Status != "active" {
				policyLabel += " (" + rec.Status + ")"
			}
		}
		label := item.Name
		if strings.TrimSpace(label) == "" {
			label = item.ID
		}
		if includeTelemetryColumns {
			lastOnline := template.HTML(esc("No telemetry"))
			battery := template.HTML(esc("Unknown"))
			if rec, ok := telemetryByDevice[item.ID]; ok {
				batteryLevel, batteryClass := batteryLevelInfo(rec.Payload)
				battery = valueOrPillHTML(batteryLevel, batteryClass)
				lastOnlineValue, lastOnlineClass := lastOnlineInfo(rec.ObservedAt)
				lastOnline = valueOrPillHTML(lastOnlineValue, lastOnlineClass)
			}
			rows.WriteString(tableRowHTML(
				template.HTML(esc(formatDashboardTime(item.CreatedAt))),
				template.HTML(`<a href="/admin/devices/`+escAttr(item.ID)+`">`+esc(label)+`</a>`),
				template.HTML(statusBadge(item.Status)),
				battery,
				lastOnline,
				template.HTML(esc(policyLabel)),
			))
			continue
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(`<a href="/admin/devices/`+escAttr(item.ID)+`">`+esc(label)+`</a>`),
			template.HTML(esc(policyLabel)),
			template.HTML(statusBadge(item.Status)),
		))
	}
	if includeTelemetryColumns {
		return tableHTML("", []string{"Created", "Name", "Status", "Battery", "Last online", "Policy"}, rows.String())
	}
	return tableHTML("", []string{"Created", "ID", "Name", "Policy", "Status"}, rows.String())
}

func deviceHealthFilterFromRequest(r *http.Request) device.HealthFilter {
	switch strings.TrimSpace(strings.ToLower(r.URL.Query().Get("health"))) {
	case "low", "low-battery", "battery-low":
		return device.HealthFilterLowBattery
	case "stale", "stale-online", "online-stale":
		return device.HealthFilterStale
	default:
		return device.HealthFilterAll
	}
}

func deviceFilterBarHTML(r *http.Request, active device.HealthFilter, searchQuery string) string {
	var b strings.Builder
	b.WriteString(`<div class="device-filter-bar">`)
	b.WriteString(`<form class="device-search-form" method="get" action="` + escAttr(r.URL.Path) + `">`)
	b.WriteString(`<label class="device-search-label" for="device-search">Search</label>`)
	fmt.Fprintf(&b, `<input id="device-search" name="search" type="search" value="%s" placeholder="Search name">`, escAttr(searchQuery))
	if active != device.HealthFilterAll {
		fmt.Fprintf(&b, `<input type="hidden" name="health" value="%s">`, escAttr(string(active)))
	}
	b.WriteString(`<button type="submit" class="device-search-submit">Search</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`<span class="device-filter-label">Filter</span>`)
	b.WriteString(renderDeviceFilterLink(r, "All", device.HealthFilterAll, active))
	b.WriteString(renderDeviceFilterLink(r, "Low battery", device.HealthFilterLowBattery, active))
	b.WriteString(renderDeviceFilterLink(r, "Stale online", device.HealthFilterStale, active))
	b.WriteString(`</div>`)
	return b.String()
}

func renderDeviceFilterLink(r *http.Request, label string, value, active device.HealthFilter) string {
	class := "device-filter-link"
	if value == active {
		class += " device-filter-link-active"
	}
	return `<a class="` + class + `" href="` + escAttr(deviceFilterURL(r, value)) + `">` + esc(label) + `</a>`
}

func deviceFilterURL(r *http.Request, value device.HealthFilter) string {
	query := cloneQuery(r.URL.Query())
	if value == device.HealthFilterAll {
		query.Del("health")
	} else {
		query.Set("health", string(value))
	}
	query.Del("page")
	return r.URL.Path + queryString(query)
}

func queryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

func batteryLevelInfo(payload map[string]any) (string, string) {
	battery, ok := payload["battery"].(map[string]any)
	if !ok {
		return "Unknown", ""
	}
	level, ok := battery["level"]
	if !ok {
		return "Unknown", ""
	}
	value, ok := parseBatteryLevel(level)
	if !ok {
		return "Unknown", ""
	}
	label := formatBatteryLevel(value)
	if value <= float64(lowBatteryThresholdPercent) {
		return label + "%", "status-pill status-low"
	}
	return label + "%", ""
}

func lastOnlineInfo(observedAt time.Time) (string, string) {
	if observedAt.IsZero() {
		return "No telemetry", ""
	}
	if time.Since(observedAt) > staleOnlineThreshold {
		return formatDashboardTime(observedAt), "status-pill status-stale"
	}
	return formatDashboardTime(observedAt), ""
}

func parseBatteryLevel(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return n, true
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, false
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func formatBatteryLevel(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func valueOrPillHTML(value, class string) template.HTML {
	if strings.TrimSpace(class) == "" {
		return template.HTML(esc(value))
	}
	return template.HTML(`<span class="` + class + `">` + esc(value) + `</span>`)
}

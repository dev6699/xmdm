package adminhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	apps "xmdm/server/internal/apps"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/managedfiles"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/plugins"
	"xmdm/server/internal/policy"
)

func (d *dashboard) policies(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Policies")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Policies.ListPolicies(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Policies", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	d.renderForSession(w, r, session, pageData{
		Title:    "Policies",
		Subtitle: "Define kiosk behavior and package rules before devices enroll.",
		Forms: []formData{{
			Title:  "Create policy",
			Action: "/admin/policies/create",
			Fields: policyFields(policy.Policy{}),
			Submit: "Create policy",
		}},
		Items: withPager(policiesTable(items), pagerHTML(r, page, limit, hasNext)),
	})
}

func (d *dashboard) policyDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Policy Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	found, err := d.deps.Policies.GetPolicy(r.Context(), d.deps.TenantID, id)
	if err != nil {
		d.renderPageError(w, r, session, "Policy Detail", err)
		return
	}
	ctx := r.Context()
	appsList := []apps.App{}
	if d.deps.Apps != nil {
		_, appParams := listPaginationParamsForKeys(r, "policyAppsPage", "policyAppsLimit", pagination.DefaultLimit)
		appsList, err = d.deps.Apps.ListApps(ctx, d.deps.TenantID, appParams)
		if err != nil {
			d.renderPageError(w, r, session, "Policy Detail", err)
			return
		}
	}
	appAssignments := make([]policy.PolicyApp, 0, len(appsList))
	if d.deps.Policies != nil {
		for _, app := range appsList {
			rec, err := d.deps.Policies.GetPolicyApp(ctx, d.deps.TenantID, found.ID, app.ID)
			if err != nil {
				if err != httpx.ErrNotFound {
					d.renderPageError(w, r, session, "Policy Detail", err)
					return
				}
				continue
			}
			appAssignments = append(appAssignments, rec)
		}
	}
	certificatesList := []certificates.Certificate{}
	if d.deps.Certificates != nil {
		_, certParams := listPaginationParamsForKeys(r, "policyCertificatesPage", "policyCertificatesLimit", pagination.DefaultLimit)
		certificatesList, err = d.deps.Certificates.ListCertificates(ctx, d.deps.TenantID, certParams)
		if err != nil {
			d.renderPageError(w, r, session, "Policy Detail", err)
			return
		}
	}
	certificateAssignments := make([]policy.PolicyCertificate, 0, len(certificatesList))
	if d.deps.Policies != nil {
		for _, cert := range certificatesList {
			rec, err := d.deps.Policies.GetPolicyCertificate(ctx, d.deps.TenantID, found.ID, cert.ID)
			if err != nil {
				if err != httpx.ErrNotFound {
					d.renderPageError(w, r, session, "Policy Detail", err)
					return
				}
				continue
			}
			certificateAssignments = append(certificateAssignments, rec)
		}
	}
	managedFiles := []managedfiles.ManagedFile{}
	if d.deps.ManagedFiles != nil {
		_, fileParams := listPaginationParamsForKeys(r, "policyManagedFilesPage", "policyManagedFilesLimit", pagination.DefaultLimit)
		managedFiles, err = d.deps.ManagedFiles.ListManagedFiles(ctx, d.deps.TenantID, fileParams)
		if err != nil {
			d.renderPageError(w, r, session, "Policy Detail", err)
			return
		}
	}
	managedFileAssignments := make([]policy.PolicyManagedFile, 0, len(managedFiles))
	if d.deps.Policies != nil {
		for _, item := range managedFiles {
			rec, err := d.deps.Policies.GetPolicyManagedFile(ctx, d.deps.TenantID, found.ID, item.ID)
			if err != nil {
				if err != httpx.ErrNotFound {
					d.renderPageError(w, r, session, "Policy Detail", err)
					return
				}
				continue
			}
			managedFileAssignments = append(managedFileAssignments, rec)
		}
	}
	data := d.policyDetailPageData(r, session, d.csrfToken(r), found, appsList, appAssignments, certificatesList, certificateAssignments, managedFiles, managedFileAssignments)
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) togglePolicyApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policy Detail") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), "invalid form")
		return
	}
	policyID := r.PathValue("id")
	appID := r.PathValue("appId")
	binding, err := d.deps.Policies.GetPolicyApp(r.Context(), d.deps.TenantID, policyID, appID)
	if err != nil {
		if err != httpx.ErrNotFound {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	active := err == nil && binding.Status == policy.StatusActive
	message := "app enabled"
	action := "update"
	if active {
		if err := d.deps.Policies.RemovePolicyApp(r.Context(), d.deps.TenantID, policyID, appID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
		message = "app disabled"
	} else {
		if _, err := d.deps.Policies.AddPolicyApp(r.Context(), d.deps.TenantID, policyID, appID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	d.recordAudit(r, action, "policy_apps", policyID+":"+appID, map[string]any{"policyId": policyID, "appId": appID, "enabled": !active})
	d.redirectOK(w, r, "/admin/policies/"+policyID, message)
}

func (d *dashboard) togglePolicyCertificate(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policy Detail") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), "invalid form")
		return
	}
	policyID := r.PathValue("id")
	certificateID := r.PathValue("certificateId")
	binding, err := d.deps.Policies.GetPolicyCertificate(r.Context(), d.deps.TenantID, policyID, certificateID)
	if err != nil {
		if err != httpx.ErrNotFound {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	active := err == nil && binding.Status == policy.StatusActive
	message := "certificate enabled"
	action := "update"
	if active {
		if err := d.deps.Policies.RemovePolicyCertificate(r.Context(), d.deps.TenantID, policyID, certificateID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
		message = "certificate disabled"
	} else {
		if _, err := d.deps.Policies.AddPolicyCertificate(r.Context(), d.deps.TenantID, policyID, certificateID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	d.recordAudit(r, action, "policy_certificates", policyID+":"+certificateID, map[string]any{"policyId": policyID, "certificateId": certificateID, "enabled": !active})
	d.redirectOK(w, r, "/admin/policies/"+policyID, message)
}

func (d *dashboard) togglePolicyManagedFile(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policy Detail") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), "invalid form")
		return
	}
	policyID := r.PathValue("id")
	managedFileID := r.PathValue("managedFileId")
	binding, err := d.deps.Policies.GetPolicyManagedFile(r.Context(), d.deps.TenantID, policyID, managedFileID)
	if err != nil {
		if err != httpx.ErrNotFound {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	active := err == nil && binding.Status == policy.StatusActive
	message := "managed file enabled"
	action := "update"
	if active {
		if err := d.deps.Policies.RemovePolicyManagedFile(r.Context(), d.deps.TenantID, policyID, managedFileID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
		message = "managed file disabled"
	} else {
		if _, err := d.deps.Policies.AddPolicyManagedFile(r.Context(), d.deps.TenantID, policyID, managedFileID); err != nil {
			d.redirectError(w, r, "/admin/policies/"+policyID, err.Error())
			return
		}
	}
	d.recordAudit(r, action, "policy_managed_files", policyID+":"+managedFileID, map[string]any{"policyId": policyID, "managedFileId": managedFileID, "enabled": !active})
	d.redirectOK(w, r, "/admin/policies/"+policyID, message)
}

func (d *dashboard) createPolicy(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policies") {
		return
	}
	req, err := policyUpsertFromForm(r, nil)
	if err != nil {
		d.redirectError(w, r, "/admin/policies", err.Error())
		return
	}
	rec, err := d.deps.Policies.CreatePolicy(r.Context(), d.deps.TenantID, req)
	if err != nil {
		d.redirectError(w, r, "/admin/policies", err.Error())
		return
	}
	d.recordAudit(r, "create", "policies", rec.ID, map[string]any{"name": rec.Name, "version": rec.Version})
	d.redirectOK(w, r, "/admin/policies", "policy created")
}

func (d *dashboard) updatePolicy(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policies") {
		return
	}
	existing, err := d.deps.Policies.GetPolicy(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	req, err := policyUpsertFromForm(r, &existing)
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	rec, err := d.deps.Policies.UpdatePolicy(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "policies", rec.ID, map[string]any{"name": rec.Name, "version": rec.Version})
	d.redirectOK(w, r, "/admin/policies/"+rec.ID, "policy updated")
}

func (d *dashboard) retirePolicy(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Policies") {
		return
	}
	rec, err := d.deps.Policies.RetirePolicy(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/policies/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "policies", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/policies/"+rec.ID, "policy retired")
}

func (d *dashboard) policyDetailPageData(r *http.Request, session *auth.Session, csrf string, found policy.Policy, items []apps.App, assignments []policy.PolicyApp, certificatesList []certificates.Certificate, certificateAssignments []policy.PolicyCertificate, managedFiles []managedfiles.ManagedFile, managedFileAssignments []policy.PolicyManagedFile) pageData {
	managedApps := policyManagedAppsSummary(r, found.ID, items, assignments, csrf, d.canWrite(session) && found.Status != "retired")
	managedCertificates := policyManagedCertificatesSummary(r, found.ID, certificatesList, certificateAssignments, csrf, d.canWrite(session) && found.Status != "retired")
	managedFileBindings := policyManagedFilesSummary(r, found.ID, managedFiles, managedFileAssignments, csrf, d.canWrite(session) && found.Status != "retired")
	data := pageData{
		Title:    "Policy Detail",
		Subtitle: "Edit the policy definition, enable managed apps, managed files, and certificates, or retire it from active use.",
		Items:    template.HTML(string(policySummary(found)) + string(managedApps) + string(managedFileBindings) + string(managedCertificates)),
	}
	if d.canWrite(session) && found.Status != "retired" {
		data.Forms = []formData{{
			Title:  "Update policy",
			Action: "/admin/policies/" + found.ID + "/update",
			Fields: policyFields(found),
			Submit: "Update policy",
		}, {
			Title:  "Retire policy",
			Action: "/admin/policies/" + found.ID + "/retire",
			Submit: "Retire policy",
			Danger: true,
		}}
	}
	return data
}

func policyUpsertFromForm(r *http.Request, existing *policy.Policy) (policy.PolicyUpsert, error) {
	if err := r.ParseForm(); err != nil {
		return policy.PolicyUpsert{}, err
	}
	restrictions := policyRestrictionPayload{
		AllowPackages:                splitLines(r.FormValue("allowPackages")),
		BlockPackages:                splitLines(r.FormValue("blockPackages")),
		SuspendPackages:              splitLines(r.FormValue("suspendPackages")),
		KioskKeepScreenOn:            hasFormField(r, "kioskKeepScreenOn"),
		KioskStayAwakeWhilePluggedIn: hasFormField(r, "kioskStayAwakeWhilePluggedIn"),
		KioskUnlockOnBoot:            hasFormField(r, "kioskUnlockOnBoot"),
		KioskExitPasscode:            strings.TrimSpace(r.FormValue("kioskExitPasscode")),
	}
	raw, err := json.Marshal(restrictions)
	if err != nil {
		return policy.PolicyUpsert{}, fmt.Errorf("invalid restrictions")
	}
	req := policy.PolicyUpsert{Name: strings.TrimSpace(r.FormValue("name")), KioskMode: hasFormField(r, "kioskMode"), KioskAppPackage: strings.TrimSpace(r.FormValue("kioskAppPackage")), Restrictions: json.RawMessage(raw)}
	if req.KioskMode && !kioskExitPasscodeConfigured(req.Restrictions) && (existing == nil || !existing.KioskMode) {
		return policy.PolicyUpsert{}, fmt.Errorf("kiosk policies require restrictions.kioskExitPasscode")
	}
	return req, nil
}

func kioskExitPasscodeConfigured(restrictions json.RawMessage) bool {
	if len(restrictions) == 0 || string(restrictions) == "null" {
		return false
	}
	var parsed policyRestrictionPayload
	if err := json.Unmarshal(restrictions, &parsed); err != nil {
		return false
	}
	return strings.TrimSpace(parsed.KioskExitPasscode) != ""
}

func splitLines(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func policyFields(item policy.Policy) []fieldData {
	restrictions := policyRestrictionFormData(item.Restrictions)
	kioskModeValue := ""
	if item.KioskMode {
		kioskModeValue = "on"
	}
	return []fieldData{
		{Name: "name", Label: "Name", Type: "text", Value: item.Name, Placeholder: "default-policy", Required: true},
		{Name: "kioskAppPackage", Label: "Kiosk app package", Type: "text", Value: item.KioskAppPackage, Placeholder: "com.android.chrome"},
		{Name: "kioskMode", Label: "Enable kiosk mode", Type: "checkbox", Value: kioskModeValue},
		{Name: "kioskExitPasscode", Label: "Kiosk exit passcode", Type: "text", Value: restrictions.KioskExitPasscode, Placeholder: "1234"},
		{Name: "allowPackages", Label: "Allow packages", Type: "textarea", Value: restrictions.AllowPackages, Placeholder: "One package per line\ncom.android.chrome\ncom.android.settings"},
		{Name: "blockPackages", Label: "Block packages", Type: "textarea", Value: restrictions.BlockPackages, Placeholder: "One package per line\ncom.android.camera\ncom.android.phone"},
		{Name: "suspendPackages", Label: "Suspend packages", Type: "textarea", Value: restrictions.SuspendPackages, Placeholder: "One package per line\ncom.android.music\ncom.example.social"},
		{Name: "kioskKeepScreenOn", Label: "Keep screen on", Type: "checkbox", Value: boolValue(restrictions.KioskKeepScreenOn)},
		{Name: "kioskStayAwakeWhilePluggedIn", Label: "Stay awake while plugged in", Type: "checkbox", Value: boolValue(restrictions.KioskStayAwakeWhilePluggedIn)},
		{Name: "kioskUnlockOnBoot", Label: "Unlock on boot", Type: "checkbox", Value: boolValue(restrictions.KioskUnlockOnBoot)},
	}
}

func policyManagedAppsSummary(r *http.Request, policyID string, items []apps.App, assignments []policy.PolicyApp, csrf string, canWrite bool) template.HTML {
	assignmentMap := appAssignmentStatusByID(assignments)
	page, params := listPaginationParamsForKeys(r, "policyAppsPage", "policyAppsLimit", pagination.DefaultLimit)
	limit := params.Limit - 1
	pageItems, hasNext := paginateItems(items, limit)
	activeCount := 0
	enabledCount := 0
	for _, app := range pageItems {
		if app.Status != apps.StatusActive {
			continue
		}
		activeCount++
		if assignmentMap[app.ID] {
			enabledCount++
		}
	}
	var pageRows strings.Builder
	rendered := 0
	for _, app := range pageItems {
		if app.Status != apps.StatusActive {
			continue
		}
		rendered++
		enabled := assignmentMap[app.ID]
		label := app.Name
		if strings.TrimSpace(label) == "" {
			label = app.PackageName
		}
		var actionCell string
		if canWrite {
			actionCell = policyAppToggleForm("/admin/policies/"+policyID+"/apps/"+app.ID+"/toggle", csrf, enabled)
		}
		pageRows.WriteString(tableRowHTML(
			template.HTML(`<a href="/admin/apps/`+escAttr(app.ID)+`">`+esc(label)+`</a>`),
			template.HTML(esc(app.PackageName)),
			template.HTML(statusBadge(app.Status)),
			template.HTML(boolBadge(enabled, "enabled", "disabled")),
			template.HTML(actionCell),
		))
	}
	if rendered == 0 {
		pageRows.WriteString(`<tr><td colspan="5" class="muted">No active managed apps on this page.</td></tr>`)
	}
	body := `<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed apps</span><span class="section-heading-meta">` + esc(fmt.Sprintf("%d enabled / %d available", enabledCount, activeCount)) + `</span></h2>` + string(tableHTML("", []string{"Name", "Package", "Status", "Policy state", "Action"}, pageRows.String()))
	body += string(pagerHTMLForKeys(r, "policyAppsPage", "policyAppsLimit", page, limit, hasNext))
	body += `</section>`
	return template.HTML(body)
}

func policyManagedCertificatesSummary(r *http.Request, policyID string, items []certificates.Certificate, assignments []policy.PolicyCertificate, csrf string, canWrite bool) template.HTML {
	assignmentMap := certificateAssignmentStatusByID(assignments)
	page, params := listPaginationParamsForKeys(r, "policyCertificatesPage", "policyCertificatesLimit", pagination.DefaultLimit)
	limit := params.Limit - 1
	pageItems, hasNext := paginateItems(items, limit)
	activeCount := 0
	enabledCount := 0
	for _, cert := range pageItems {
		if cert.Status != certificates.StatusActive {
			continue
		}
		activeCount++
		if assignmentMap[cert.ID] {
			enabledCount++
		}
	}
	var pageRows strings.Builder
	rendered := 0
	for _, cert := range pageItems {
		if cert.Status != certificates.StatusActive {
			continue
		}
		rendered++
		enabled := assignmentMap[cert.ID]
		label := cert.Name
		if strings.TrimSpace(label) == "" {
			label = cert.ID
		}
		artifact := cert.ArtifactID
		if cert.Artifact != nil && strings.TrimSpace(cert.Artifact.StorageKey) != "" {
			artifact = cert.Artifact.StorageKey
		}
		var actionCell string
		if canWrite {
			actionCell = policyCertificateToggleForm("/admin/policies/"+policyID+"/certificates/"+cert.ID+"/toggle", csrf, enabled)
		}
		pageRows.WriteString(tableRowHTML(
			template.HTML(`<a href="/admin/certificates/`+escAttr(cert.ID)+`">`+esc(label)+`</a>`),
			template.HTML(esc(artifact)),
			template.HTML(statusBadge(cert.Status)),
			template.HTML(boolBadge(enabled, "enabled", "disabled")),
			template.HTML(actionCell),
		))
	}
	if rendered == 0 {
		pageRows.WriteString(`<tr><td colspan="5" class="muted">No active managed certificates on this page.</td></tr>`)
	}
	body := `<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed certificates</span><span class="section-heading-meta">` + esc(fmt.Sprintf("%d enabled / %d available", enabledCount, activeCount)) + `</span></h2>` + string(tableHTML("", []string{"Name", "Artifact", "Status", "Policy state", "Action"}, pageRows.String()))
	body += string(pagerHTMLForKeys(r, "policyCertificatesPage", "policyCertificatesLimit", page, limit, hasNext))
	body += `</section>`
	return template.HTML(body)
}

func policyManagedFilesSummary(r *http.Request, policyID string, items []managedfiles.ManagedFile, assignments []policy.PolicyManagedFile, csrf string, canWrite bool) template.HTML {
	assignmentMap := managedFileAssignmentStatusByID(assignments)
	page, params := listPaginationParamsForKeys(r, "policyManagedFilesPage", "policyManagedFilesLimit", pagination.DefaultLimit)
	limit := params.Limit - 1
	pageItems, hasNext := paginateItems(items, limit)
	activeCount := 0
	enabledCount := 0
	for _, item := range pageItems {
		if item.Status != managedfiles.StatusActive {
			continue
		}
		activeCount++
		if assignmentMap[item.ID] {
			enabledCount++
		}
	}
	var pageRows strings.Builder
	rendered := 0
	for _, item := range pageItems {
		if item.Status != managedfiles.StatusActive {
			continue
		}
		rendered++
		enabled := assignmentMap[item.ID]
		path := item.Path
		if strings.TrimSpace(path) == "" {
			path = item.ID
		}
		fileLabel := item.FileID
		if item.File != nil && strings.TrimSpace(item.File.Name) != "" {
			fileLabel = item.File.Name
		}
		var actionCell string
		if canWrite {
			actionCell = policyManagedFileToggleForm("/admin/policies/"+policyID+"/managed-files/"+item.ID+"/toggle", csrf, enabled)
		}
		pageRows.WriteString(tableRowHTML(
			template.HTML(`<a href="/admin/managed-files/`+escAttr(item.ID)+`">`+esc(path)+`</a>`),
			template.HTML(esc(fileLabel)),
			template.HTML(boolBadge(item.ReplaceVariables, "enabled", "disabled")),
			template.HTML(statusBadge(item.Status)),
			template.HTML(boolBadge(enabled, "enabled", "disabled")),
			template.HTML(actionCell),
		))
	}
	if rendered == 0 {
		pageRows.WriteString(`<tr><td colspan="6" class="muted">No active managed files on this page.</td></tr>`)
	}
	body := `<section class="panel policy-detail-section"><h2 class="section-heading"><span>Managed files</span><span class="section-heading-meta">` + esc(fmt.Sprintf("%d enabled / %d available", enabledCount, activeCount)) + `</span></h2>` + string(tableHTML("", []string{"Path", "File", "Template", "Status", "Policy state", "Action"}, pageRows.String()))
	body += string(pagerHTMLForKeys(r, "policyManagedFilesPage", "policyManagedFilesLimit", page, limit, hasNext))
	body += `</section>`
	return template.HTML(body)
}

func policySummary(found policy.Policy) template.HTML {
	kioskAppPackage := found.KioskAppPackage
	if strings.TrimSpace(kioskAppPackage) == "" {
		kioskAppPackage = "—"
	}
	var b strings.Builder
	b.WriteString(`<section class="panel"><h2 class="section-heading"><span>Current policy</span><span class="section-heading-meta">`)
	b.WriteString(esc(found.Status))
	b.WriteString(` · v`)
	b.WriteString(strconv.Itoa(found.Version))
	b.WriteString(`</span></h2>`)
	b.WriteString(`<div class="policy-summary">`)
	b.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	b.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	b.WriteString(summaryTextItem("ID", found.ID))
	b.WriteString(summaryTextItem("Name", found.Name))
	b.WriteString(summaryTextItem("Version", strconv.Itoa(found.Version)))
	b.WriteString(summaryHTMLItem("Kiosk mode", template.HTML(boolBadge(found.KioskMode, "enabled", "disabled"))))
	b.WriteString(summaryTextItem("Kiosk app package", kioskAppPackage))
	b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	b.WriteString(summaryWideHTMLItem("Stored restrictions", string(renderPolicyRestrictions(found.Restrictions)), "policy-restrictions-value"))
	b.WriteString(`</div>`)
	b.WriteString(string(rawDataDetails("Raw policy data", found)))
	b.WriteString(`</section>`)
	return template.HTML(b.String())
}

func renderPolicyRestrictions(raw json.RawMessage) template.HTML {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return template.HTML(`<span class="structured-muted">No restrictions stored.</span>`)
	}

	var parsed policyRestrictionPayload
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return renderStructuredJSON(raw)
	}

	var b strings.Builder
	b.WriteString(`<div class="policy-summary">`)
	b.WriteString(summaryHTMLItem("Allow packages", restrictionPackageList(parsed.AllowPackages)))
	b.WriteString(summaryHTMLItem("Block packages", restrictionPackageList(parsed.BlockPackages)))
	b.WriteString(summaryHTMLItem("Suspend packages", restrictionPackageList(parsed.SuspendPackages)))
	b.WriteString(summaryHTMLItem("Keep screen on", template.HTML(boolBadge(parsed.KioskKeepScreenOn, "enabled", "disabled"))))
	b.WriteString(summaryHTMLItem("Stay awake while plugged in", template.HTML(boolBadge(parsed.KioskStayAwakeWhilePluggedIn, "enabled", "disabled"))))
	b.WriteString(summaryHTMLItem("Unlock on boot", template.HTML(boolBadge(parsed.KioskUnlockOnBoot, "enabled", "disabled"))))
	b.WriteString(summaryHTMLItem("Exit passcode configured", template.HTML(boolBadge(strings.TrimSpace(parsed.KioskExitPasscode) != "", "yes", "no"))))
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func restrictionPackageList(items []string) template.HTML {
	if len(items) == 0 {
		return template.HTML(`<span class="structured-muted">None</span>`)
	}
	var b strings.Builder
	b.WriteString(`<div class="structured-list">`)
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		fmt.Fprintf(&b, `<span class="structured-pill">%s</span>`, esc(item))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func pluginDeviceActionsSection(actions []plugins.DeviceAction) template.HTML {
	if len(actions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="panel"><h2>Plugin actions</h2><div class="policy-summary">`)
	for _, action := range actions {
		title := action.ActionID
		if strings.TrimSpace(action.PluginID) != "" {
			title = action.PluginID + " / " + action.ActionID
		}
		button := `<a class="button btn-primary" href="` + escAttr(action.Href) + `" title="` + escAttr(title) + `">` + esc(action.Label) + `</a>`
		b.WriteString(summaryHTMLItem("Action", template.HTML(button)))
	}
	b.WriteString(`</div></section>`)
	return template.HTML(b.String())
}

func summaryTextItem(label, value string) string {
	return `<div class="policy-summary-item"><div class="policy-summary-label">` + esc(label) + `</div><div class="policy-summary-value">` + esc(value) + `</div></div>`
}

func summaryHTMLItem(label string, value template.HTML) string {
	return `<div class="policy-summary-item"><div class="policy-summary-label">` + esc(label) + `</div><div class="policy-summary-value">` + string(value) + `</div></div>`
}

func summaryWideHTMLItem(label, value string, valueClass string) string {
	classAttr := "policy-summary-item policy-summary-wide"
	if strings.TrimSpace(valueClass) != "" {
		classAttr += " " + escAttr(valueClass)
	}
	return `<div class="` + classAttr + `"><div class="policy-summary-label">` + esc(label) + `</div><div class="policy-summary-value">` + value + `</div></div>`
}

func appAssignmentStatusByID(assignments []policy.PolicyApp) map[string]bool {
	out := make(map[string]bool, len(assignments))
	for _, assignment := range assignments {
		out[assignment.AppID] = assignment.Status == policy.StatusActive
	}
	return out
}

func certificateAssignmentStatusByID(assignments []policy.PolicyCertificate) map[string]bool {
	out := make(map[string]bool, len(assignments))
	for _, assignment := range assignments {
		out[assignment.CertificateID] = assignment.Status == policy.StatusActive
	}
	return out
}

func managedFileAssignmentStatusByID(assignments []policy.PolicyManagedFile) map[string]bool {
	out := make(map[string]bool, len(assignments))
	for _, assignment := range assignments {
		out[assignment.ManagedFileID] = assignment.Status == policy.StatusActive
	}
	return out
}

func boolValue(v bool) string {
	if v {
		return "on"
	}
	return ""
}

func policyRestrictionFormData(raw json.RawMessage) policyRestrictionFormState {
	var parsed policyRestrictionPayload
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &parsed)
	}
	return policyRestrictionFormState{
		AllowPackages:                strings.Join(parsed.AllowPackages, "\n"),
		BlockPackages:                strings.Join(parsed.BlockPackages, "\n"),
		SuspendPackages:              strings.Join(parsed.SuspendPackages, "\n"),
		KioskKeepScreenOn:            parsed.KioskKeepScreenOn,
		KioskStayAwakeWhilePluggedIn: parsed.KioskStayAwakeWhilePluggedIn,
		KioskUnlockOnBoot:            parsed.KioskUnlockOnBoot,
		KioskExitPasscode:            strings.TrimSpace(parsed.KioskExitPasscode),
	}
}

type policyRestrictionFormState struct {
	AllowPackages                string
	BlockPackages                string
	SuspendPackages              string
	KioskKeepScreenOn            bool
	KioskStayAwakeWhilePluggedIn bool
	KioskUnlockOnBoot            bool
	KioskExitPasscode            string
}

type policyRestrictionPayload struct {
	AllowPackages                []string `json:"allowPackages,omitempty"`
	BlockPackages                []string `json:"blockPackages,omitempty"`
	SuspendPackages              []string `json:"suspendPackages,omitempty"`
	KioskKeepScreenOn            bool     `json:"kioskKeepScreenOn,omitempty"`
	KioskStayAwakeWhilePluggedIn bool     `json:"kioskStayAwakeWhilePluggedIn,omitempty"`
	KioskUnlockOnBoot            bool     `json:"kioskUnlockOnBoot,omitempty"`
	KioskExitPasscode            string   `json:"kioskExitPasscode,omitempty"`
}

func policyAppToggleForm(action, csrf string, enabled bool) string {
	label := "Enable"
	buttonClass := ""
	if enabled {
		label = "Disable"
		buttonClass = ` class="danger"`
	}
	return `<form class="inline" method="post" action="` + escAttr(action) + `"><input type="hidden" name="csrfToken" value="` + escAttr(csrf) + `"><button type="submit"` + buttonClass + `>` + esc(label) + `</button></form>`
}

func policyCertificateToggleForm(action, csrf string, enabled bool) string {
	return policyAppToggleForm(action, csrf, enabled)
}

func policyManagedFileToggleForm(action, csrf string, enabled bool) string {
	return policyAppToggleForm(action, csrf, enabled)
}

func policiesTable(items []policy.Policy) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(`<a href="/admin/policies/`+escAttr(item.ID)+`">`+esc(item.Name)+`</a>`),
			template.HTML(boolBadge(item.KioskMode, "enabled", "disabled")),
			template.HTML(statusBadge(item.Status)),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Name", "Kiosk", "Status"}, rows.String())
}

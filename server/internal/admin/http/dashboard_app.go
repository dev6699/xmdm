package adminhttp

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"

	"github.com/google/uuid"
)

func (d *dashboard) apps(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Apps")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Apps.ListApps(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Apps", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	versions := map[string][]apps.Version{}
	for _, item := range items {
		latest, err := d.deps.Apps.GetLatestPublishedVersion(r.Context(), d.deps.TenantID, item.ID)
		if err == nil {
			versions[item.ID] = []apps.Version{latest}
		}
	}
	d.renderForSession(w, r, session, pageData{Title: "Apps", Subtitle: "Manage approved APKs and their published versions.", Forms: []formData{managedAppForm("/admin/apps/create")}, Items: withPager(appsTable(items, versions), pagerHTML(r, page, limit, hasNext))})
}

func (d *dashboard) appDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "App Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, versions, latest, err := d.loadAppDetail(r.Context(), r.PathValue("id"), r)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	data := d.appDetailPageData(r, session, found, versions, latest)
	d.renderForSession(w, r, session, data)
}

func (d *dashboard) loadAppDetail(ctx context.Context, id string, r *http.Request) (*apps.App, []apps.Version, *apps.Version, error) {
	found, err := d.deps.Apps.GetApp(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, nil, nil, err
	}
	_, params := listPaginationParamsForKeys(r, "versionsPage", "versionsLimit", pagination.DefaultLimit)
	versions, err := d.deps.Apps.ListVersions(ctx, d.deps.TenantID, found.ID, params)
	if err != nil {
		return nil, nil, nil, err
	}
	latest, err := d.deps.Apps.GetLatestPublishedVersion(ctx, d.deps.TenantID, found.ID)
	if err != nil {
		if err == httpx.ErrNotFound {
			return &found, versions, nil, nil
		}
		return nil, nil, nil, err
	}
	return &found, versions, &latest, nil
}

func (d *dashboard) appDetailPageData(r *http.Request, session *auth.Session, found *apps.App, versions []apps.Version, latest *apps.Version) pageData {
	page, params := listPaginationParamsForKeys(r, "versionsPage", "versionsLimit", pagination.DefaultLimit)
	limit := params.Limit - 1
	items, hasNext := paginateItems(versions, limit)
	body := panelSectionHTML("", "Current app", appSummary(*found, latest))
	if len(versions) > 0 {
		body += panelSectionHTML("", "Versions", withPager(appVersionsTable(items), pagerHTMLForKeys(r, "versionsPage", "versionsLimit", page, limit, hasNext)))
	}
	data := pageData{
		Title:    "App Detail",
		Subtitle: "Manage app versions and, for non-seeded apps, the app metadata.",
		Items:    template.HTML(body),
	}
	if found.SystemOwned {
		data.Callout = panelMessageHTML("Seeded app", "This app is system-owned. Use the Publish new version flow below to add APK releases. The app metadata and lifecycle are locked.")
	}
	if d.canWrite(session) && found.Status != apps.StatusRetired {
		data.Forms = []formData{publishAppVersionForm("/admin/apps/" + found.ID + "/versions/publish")}
		if !found.SystemOwned {
			data.Forms = append(data.Forms, formData{
				Title:  "Update app",
				Action: "/admin/apps/" + found.ID + "/update",
				Fields: []fieldData{
					{Name: "packageName", Label: "Package name", Type: "text", Value: found.PackageName, Placeholder: "com.example.app", Required: true},
					{Name: "name", Label: "App name", Type: "text", Value: found.Name, Placeholder: "Example App", Required: true},
				},
				Submit: "Update app",
			}, formData{
				Title:  "Retire app",
				Action: "/admin/apps/" + found.ID + "/retire",
				Submit: "Retire app",
				Danger: true,
			})
		}
	}
	return data
}

func (d *dashboard) downloadApp(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "App Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if d.deps.Artifacts == nil || d.deps.Apps == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	found, _, latest, err := d.loadAppDetail(r.Context(), r.PathValue("id"), r)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	if latest == nil {
		http.NotFound(w, r)
		return
	}
	latestVersion, err := d.deps.Apps.GetVersion(r.Context(), d.deps.TenantID, found.ID, latest.ID)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	if latestVersion.Artifact == nil {
		http.NotFound(w, r)
		return
	}
	body, err := d.deps.Artifacts.Get(r.Context(), latestVersion.Artifact.StorageKey)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		d.renderPageError(w, r, session, "App Detail", err)
		return
	}
	w.Header().Set("Content-Type", latestVersion.Artifact.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
	w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
	downloadName := found.PackageName
	if downloadName == "" {
		downloadName = found.Name
	}
	if downloadName == "" {
		downloadName = found.ID
	}
	if latestVersion.VersionName != "" {
		downloadName += "-" + latestVersion.VersionName
	}
	downloadName += ".apk"
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(content))
}

func (d *dashboard) createApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	req, content, err := managedAppUpsertFromMultipart(r)
	if err != nil {
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	appRec, existingVersion, reusedExisting, err := d.findManagedAppForCreate(r.Context(), req.App, req.VersionCode, req.File.Checksum)
	if err != nil {
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	if existingVersion != nil {
		d.recordAudit(r, "create", "apps", appRec.ID, map[string]any{
			"packageName": appRec.PackageName,
			"name":        appRec.Name,
			"versionId":   existingVersion.ID,
			"versionName": existingVersion.VersionName,
			"versionCode": existingVersion.VersionCode,
			"artifactId":  existingVersion.ArtifactID,
		})
		d.redirectOK(w, r, "/admin/apps/"+appRec.ID, "managed app already up to date")
		return
	}
	if len(content) > 0 {
		if err := d.deps.Artifacts.Put(r.Context(), req.StorageKey, bytes.NewReader(content), req.MimeType, req.SizeBytes); err != nil {
			d.redirectError(w, r, "/admin/apps", err.Error())
			return
		}
	}
	fileRec, err := d.deps.Files.CreateFile(r.Context(), d.deps.TenantID, req.File)
	if err != nil {
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	artifactID := fileRec.ArtifactID
	versionRec, err := d.deps.Apps.CreateVersion(r.Context(), d.deps.TenantID, appRec.ID, apps.VersionUpsert{
		VersionName: req.VersionName,
		VersionCode: req.VersionCode,
		ArtifactID:  &artifactID,
		Checksum:    fileRec.Checksum,
		Publish:     req.Publish,
	})
	if err != nil {
		_, _ = d.deps.Files.RetireFile(r.Context(), d.deps.TenantID, fileRec.ID)
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		if !reusedExisting {
			_, _ = d.deps.Apps.RetireApp(r.Context(), d.deps.TenantID, appRec.ID)
		}
		d.redirectError(w, r, "/admin/apps", err.Error())
		return
	}
	d.recordAudit(r, "create", "apps", appRec.ID, map[string]any{
		"packageName": appRec.PackageName,
		"name":        appRec.Name,
		"versionId":   versionRec.ID,
		"versionName": versionRec.VersionName,
		"versionCode": versionRec.VersionCode,
		"artifactId":  fileRec.ArtifactID,
		"fileName":    fileRec.Name,
	})
	d.redirectOK(w, r, "/admin/apps/"+appRec.ID, "managed app created")
}

func (d *dashboard) findManagedAppForCreate(ctx context.Context, appReq apps.AppUpsert, versionCode int64, checksumValue string) (apps.App, *apps.Version, bool, error) {
	item, err := d.deps.Apps.GetAppByPackageName(ctx, d.deps.TenantID, appReq.PackageName)
	if err != nil {
		if err != httpx.ErrNotFound {
			return apps.App{}, nil, false, err
		}
	}
	if item.ID != "" {
		if item.Status != apps.StatusActive {
			return apps.App{}, nil, false, fmt.Errorf("app package already exists")
		}
		version, err := d.deps.Apps.GetVersionByCode(ctx, d.deps.TenantID, item.ID, versionCode)
		if err != nil {
			if err == httpx.ErrNotFound {
				return item, nil, true, nil
			}
			return apps.App{}, nil, false, err
		}
		if version.Checksum != checksumValue {
			return apps.App{}, nil, false, fmt.Errorf("app version already exists with different content")
		}
		return item, &version, true, nil
	}
	rec, err := d.deps.Apps.CreateApp(ctx, d.deps.TenantID, appReq)
	if err != nil {
		return apps.App{}, nil, false, err
	}
	return rec, nil, false, nil
}

func (d *dashboard) updateApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), "invalid form")
		return
	}
	rec, err := d.deps.Apps.UpdateApp(r.Context(), d.deps.TenantID, r.PathValue("id"), apps.AppUpsert{PackageName: strings.TrimSpace(r.FormValue("packageName")), Name: strings.TrimSpace(r.FormValue("name"))})
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "update", "apps", rec.ID, map[string]any{"packageName": rec.PackageName, "name": rec.Name})
	d.redirectOK(w, r, "/admin/apps/"+rec.ID, "app updated")
}

func (d *dashboard) retireApp(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	rec, err := d.deps.Apps.RetireApp(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "retire", "apps", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/apps/"+rec.ID, "app retired")
}

func (d *dashboard) createAppVersion(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	if err := r.ParseForm(); err != nil {
		d.redirectError(w, r, "/admin/apps", "invalid form")
		return
	}
	versionCode, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("versionCode")), 10, 64)
	if err != nil {
		d.redirectError(w, r, "/admin/apps", "invalid version code")
		return
	}
	artifactID := strings.TrimSpace(r.FormValue("artifactId"))
	req := apps.VersionUpsert{VersionName: strings.TrimSpace(r.FormValue("versionName")), VersionCode: versionCode, Checksum: strings.TrimSpace(r.FormValue("checksum")), Publish: true}
	if artifactID != "" {
		req.ArtifactID = &artifactID
	}
	rec, err := d.deps.Apps.CreateVersion(r.Context(), d.deps.TenantID, r.PathValue("id"), req)
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), err.Error())
		return
	}
	d.recordAudit(r, "create", "app_versions", rec.ID, map[string]any{"appId": rec.AppID, "versionName": rec.VersionName, "versionCode": rec.VersionCode, "status": rec.Status})
	d.redirectOK(w, r, "/admin/apps/"+rec.AppID, "app version created")
}

func (d *dashboard) publishAppVersion(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Apps") {
		return
	}
	app, err := d.deps.Apps.GetApp(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), err.Error())
		return
	}
	if app.Status != apps.StatusActive {
		d.redirectError(w, r, "/admin/apps/"+r.PathValue("id"), "app is not active")
		return
	}
	req, content, existingVersion, err := managedAppVersionPublishFromMultipart(r, app)
	if err != nil {
		d.redirectError(w, r, "/admin/apps/"+app.ID, err.Error())
		return
	}
	if existingVersion == nil {
		existingVersion, err = d.findManagedAppVersionForPublish(r.Context(), app.ID, req.VersionCode, req.File.Checksum)
		if err != nil {
			d.redirectError(w, r, "/admin/apps/"+app.ID, err.Error())
			return
		}
	}
	if existingVersion != nil && existingVersion.Status == apps.VersionStatusPublished {
		d.recordAudit(r, "create", "app_versions", existingVersion.ID, map[string]any{
			"appId":       existingVersion.AppID,
			"versionName": existingVersion.VersionName,
			"versionCode": existingVersion.VersionCode,
			"artifactId":  existingVersion.ArtifactID,
		})
		d.redirectOK(w, r, "/admin/apps/"+app.ID, "app version already up to date")
		return
	}
	if len(content) > 0 {
		if err := d.deps.Artifacts.Put(r.Context(), req.StorageKey, bytes.NewReader(content), req.MimeType, req.SizeBytes); err != nil {
			d.redirectError(w, r, "/admin/apps/"+app.ID, err.Error())
			return
		}
	}
	fileRec, err := d.deps.Files.CreateFile(r.Context(), d.deps.TenantID, req.File)
	if err != nil {
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		d.redirectError(w, r, "/admin/apps/"+app.ID, err.Error())
		return
	}
	artifactID := fileRec.ArtifactID
	versionRec, err := d.deps.Apps.CreateVersion(r.Context(), d.deps.TenantID, app.ID, apps.VersionUpsert{
		VersionName: req.VersionName,
		VersionCode: req.VersionCode,
		ArtifactID:  &artifactID,
		Checksum:    fileRec.Checksum,
		Publish:     true,
	})
	if err != nil {
		_, _ = d.deps.Files.RetireFile(r.Context(), d.deps.TenantID, fileRec.ID)
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		d.redirectError(w, r, "/admin/apps/"+app.ID, err.Error())
		return
	}
	d.recordAudit(r, "create", "app_versions", versionRec.ID, map[string]any{
		"appId":       versionRec.AppID,
		"packageName": app.PackageName,
		"name":        app.Name,
		"versionName": versionRec.VersionName,
		"versionCode": versionRec.VersionCode,
		"status":      versionRec.Status,
		"fileName":    fileRec.Name,
		"artifactId":  fileRec.ArtifactID,
	})
	d.redirectOK(w, r, "/admin/apps/"+app.ID, "app version published")
}

type managedAppCreateRequest struct {
	App         apps.AppUpsert
	VersionName string
	VersionCode int64
	Publish     bool
	File        files.FileUpsert
	StorageKey  string
	MimeType    string
	SizeBytes   int64
}

type managedAppVersionPublishRequest struct {
	VersionName string
	VersionCode int64
	File        files.FileUpsert
	StorageKey  string
	MimeType    string
	SizeBytes   int64
}

func managedAppForm(action string) formData {
	return formData{
		Title:   "Create managed app",
		Action:  action,
		EncType: "multipart/form-data",
		Fields: []fieldData{
			{Name: "packageName", Label: "Package name", Type: "text", Placeholder: "com.example.app", Required: true},
			{Name: "name", Label: "App name", Type: "text", Placeholder: "Example App", Required: true},
			{Name: "versionCode", Label: "Version code", Type: "number", Placeholder: "100", Required: true},
			{Name: "file", Label: "APK file", Type: "file", Required: true},
		},
		Help:   template.HTML(`Upload the APK once. The dashboard derives the artifact storage key, checksum, and file record on the server, then creates the app and publishes its first version in one step.`),
		Submit: "Create managed app",
	}
}

func publishAppVersionForm(action string) formData {
	return formData{
		Title:   "Publish new version",
		Action:  action,
		EncType: "multipart/form-data",
		Fields: []fieldData{
			{Name: "versionCode", Label: "Version code", Type: "number", Placeholder: "100", Required: true},
			{Name: "file", Label: "APK file", Type: "file", Required: true},
		},
		Help:   template.HTML(`Upload a new APK for this app. The package name and app name come from the existing app record, so only the version code and file are needed.`),
		Submit: "Publish version",
	}
}

func managedAppUpsertFromMultipart(r *http.Request) (managedAppCreateRequest, []byte, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return managedAppCreateRequest{}, nil, err
	}
	versionCode, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("versionCode")), 10, 64)
	if err != nil {
		return managedAppCreateRequest{}, nil, fmt.Errorf("invalid version code")
	}
	app := apps.AppUpsert{PackageName: strings.TrimSpace(r.FormValue("packageName")), Name: strings.TrimSpace(r.FormValue("name"))}
	if app.PackageName == "" || app.Name == "" {
		return managedAppCreateRequest{}, nil, fmt.Errorf("invalid form")
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return managedAppCreateRequest{}, nil, fmt.Errorf("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return managedAppCreateRequest{}, nil, err
	}
	if len(content) == 0 {
		return managedAppCreateRequest{}, nil, fmt.Errorf("file is empty")
	}
	checksumValue := checksum.SHA256Base64URL(content)
	versionName := strings.TrimSpace(r.FormValue("versionName"))
	if versionName == "" {
		versionName = fmt.Sprintf("v%d", versionCode)
	}
	fileName := sanitizeArtifactName(fmt.Sprintf("%s-%d.apk", app.PackageName, versionCode))
	storageKey := "artifacts/apps/" + uuid.NewString() + "/" + fileName
	return managedAppCreateRequest{
		App:         app,
		VersionName: versionName,
		VersionCode: versionCode,
		Publish:     true,
		File: files.FileUpsert{
			Name:       fileName,
			StorageKey: storageKey,
			Checksum:   checksumValue,
			SizeBytes:  int64(len(content)),
			MimeType:   "application/vnd.android.package-archive",
		},
		StorageKey: storageKey,
		MimeType:   "application/vnd.android.package-archive",
		SizeBytes:  int64(len(content)),
	}, content, nil
}

func managedAppVersionPublishFromMultipart(r *http.Request, app apps.App) (managedAppVersionPublishRequest, []byte, *apps.Version, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return managedAppVersionPublishRequest{}, nil, nil, err
	}
	versionCode, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("versionCode")), 10, 64)
	if err != nil {
		return managedAppVersionPublishRequest{}, nil, nil, fmt.Errorf("invalid version code")
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return managedAppVersionPublishRequest{}, nil, nil, fmt.Errorf("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return managedAppVersionPublishRequest{}, nil, nil, err
	}
	if len(content) == 0 {
		return managedAppVersionPublishRequest{}, nil, nil, fmt.Errorf("file is empty")
	}
	checksumValue := checksum.SHA256Base64URL(content)
	versionName := strings.TrimSpace(r.FormValue("versionName"))
	if versionName == "" {
		versionName = fmt.Sprintf("v%d", versionCode)
	}
	fileName := sanitizeArtifactName(fmt.Sprintf("%s-%d.apk", app.PackageName, versionCode))
	storageKey := "artifacts/apps/" + uuid.NewString() + "/" + fileName
	req := managedAppVersionPublishRequest{
		VersionName: versionName,
		VersionCode: versionCode,
		File: files.FileUpsert{
			Name:       fileName,
			StorageKey: storageKey,
			Checksum:   checksumValue,
			SizeBytes:  int64(len(content)),
			MimeType:   "application/vnd.android.package-archive",
		},
		StorageKey: storageKey,
		MimeType:   "application/vnd.android.package-archive",
		SizeBytes:  int64(len(content)),
	}
	return req, content, nil, nil
}

func (d *dashboard) findManagedAppVersionForPublish(ctx context.Context, appID string, versionCode int64, checksumValue string) (*apps.Version, error) {
	version, err := d.deps.Apps.GetVersionByCode(ctx, d.deps.TenantID, appID, versionCode)
	if err != nil {
		if err == httpx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	if version.Status != apps.VersionStatusPublished {
		return nil, nil
	}
	if version.Checksum != checksumValue {
		return nil, fmt.Errorf("app version already exists with different content")
	}
	return &version, nil
}

func sanitizeArtifactName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.NewReplacer("\\", "_", "/", "_", " ", "_").Replace(value)
	if value == "." || value == "" {
		return "artifact.apk"
	}
	return value
}

func appsTable(items []apps.App, versions map[string][]apps.Version) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		latest := appLatestPublishedVersion(versions[item.ID])
		systemOwned := ""
		if item.SystemOwned {
			systemOwned = `<span class="status-pill status-enabled">✓</span>`
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(`<a href="/admin/apps/`+escAttr(item.ID)+`">`+esc(item.Name)+`</a>`),
			template.HTML(esc(item.PackageName)),
			template.HTML(systemOwned),
			template.HTML(esc(appVersionLabel(latest))),
			template.HTML(statusBadge(item.Status)),
		))
	}
	return tableHTML("", []string{"Created", "Name", "Package", "System owned", "Latest published", "Status"}, rows.String())
}

func appLatestPublishedVersion(items []apps.Version) *apps.Version {
	var latest *apps.Version
	for i := range items {
		if items[i].Status != apps.VersionStatusPublished {
			continue
		}
		if latest == nil {
			latest = &items[i]
			continue
		}
		if items[i].PublishedAt != nil && latest.PublishedAt != nil {
			if items[i].PublishedAt.After(*latest.PublishedAt) {
				latest = &items[i]
			}
			continue
		}
		if items[i].PublishedAt != nil && latest.PublishedAt == nil {
			latest = &items[i]
			continue
		}
		if items[i].PublishedAt == nil && latest.PublishedAt == nil && items[i].CreatedAt.After(latest.CreatedAt) {
			latest = &items[i]
		}
	}
	return latest
}

func appVersionLabel(item *apps.Version) string {
	if item == nil {
		return "—"
	}
	published := item.CreatedAt
	if item.PublishedAt != nil && !item.PublishedAt.IsZero() {
		published = *item.PublishedAt
	}
	return item.VersionName + " (#" + strconv.FormatInt(item.VersionCode, 10) + ") · " + formatDashboardTime(published)
}

func appSummary(found apps.App, latest *apps.Version) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="policy-summary">`)
	b.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	b.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	b.WriteString(summaryTextItem("ID", found.ID))
	b.WriteString(summaryTextItem("Name", found.Name))
	b.WriteString(summaryTextItem("Package", found.PackageName))
	b.WriteString(summaryHTMLItem("Locked", template.HTML(boolBadge(found.SystemOwned, "system-owned", "editable"))))
	if latest != nil && (latest.ArtifactID != nil || latest.Artifact != nil) {
		b.WriteString(summaryHTMLItem("Download", template.HTML(fmt.Sprintf(`<a class="button btn-primary" href="/admin/apps/%s/download">Download latest APK</a>`, escAttr(found.ID)))))
	}
	b.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	b.WriteString(`</div>`)
	b.WriteString(string(rawDataDetails("Raw app data", found)))
	if latest != nil {
		b.WriteString(string(rawDataDetails("Raw latest version data", latest)))
	}
	return template.HTML(b.String())
}

func appVersionsTable(items []apps.Version) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		published := "—"
		if item.PublishedAt != nil {
			published = formatDashboardTime(*item.PublishedAt)
		}
		artifact := item.Checksum
		if item.ArtifactID != nil && strings.TrimSpace(*item.ArtifactID) != "" {
			artifact = *item.ArtifactID
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(esc(item.VersionName)),
			template.HTML(esc(strconv.FormatInt(item.VersionCode, 10))),
			template.HTML(statusBadge(item.Status)),
			template.HTML(esc(published)),
			template.HTML(`<code>`+esc(artifact)+`</code>`),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Name", "Code", "Status", "Published", "Artifact"}, rows.String())
}

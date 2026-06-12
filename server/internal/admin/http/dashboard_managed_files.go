package adminhttp

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/files"
	"xmdm/server/internal/managedfiles"
	"xmdm/server/internal/pagination"

	"github.com/google/uuid"
)

func (d *dashboard) managedFiles(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Managed Files")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.ManagedFiles.ListManagedFiles(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Managed Files", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	d.renderForSession(w, r, session, pageData{Title: "Managed Files", Subtitle: "Upload device-side files and keep them ready for policy binding.", Forms: []formData{{Title: "Upload managed file", Action: "/admin/managed-files/create", EncType: "multipart/form-data", Fields: []fieldData{{Name: "path", Label: "Device path", Type: "text", Required: true, Placeholder: "/sdcard/xmdm/device-config.txt"}, {Name: "replaceVariables", Label: "Replace variables", Type: "checkbox", Value: "on"}, {Name: "file", Label: "File", Type: "file", Required: true}}, Submit: "Upload managed file"}}, Items: withPager(managedFilesTable(items), pagerHTML(r, page, limit, hasNext))})
}

func (d *dashboard) managedFileDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Managed File Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, err := d.loadManagedFileDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	d.renderForSession(w, r, session, d.managedFileDetailPageData(session, found))
}

func (d *dashboard) loadManagedFileDetail(ctx context.Context, id string) (*managedfiles.ManagedFile, error) {
	found, err := d.deps.ManagedFiles.GetManagedFile(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, err
	}
	return &found, nil
}

func (d *dashboard) managedFileDetailPageData(session *auth.Session, found *managedfiles.ManagedFile) pageData {
	var body strings.Builder
	body.WriteString(`<div class="policy-summary">`)
	body.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	body.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	body.WriteString(summaryTextItem("ID", found.ID))
	body.WriteString(summaryTextItem("Path", found.Path))
	body.WriteString(summaryTextItem("File ID", found.FileID))
	if found.File != nil {
		body.WriteString(summaryTextItem("Source file", found.File.Name))
		body.WriteString(summaryTextItem("Artifact", found.File.ArtifactID))
		body.WriteString(summaryTextItem("Checksum", found.File.Checksum))
		body.WriteString(summaryTextItem("MIME", found.File.MimeType))
		body.WriteString(summaryHTMLItem("Download", template.HTML(fmt.Sprintf(`<a class="button btn-primary" href="/admin/managed-files/%s/download">Download file</a>`, escAttr(found.ID)))))
	}
	body.WriteString(summaryHTMLItem("Template", template.HTML(boolBadge(found.ReplaceVariables, "enabled", "disabled"))))
	body.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	body.WriteString(`</div>`)
	body.WriteString(string(rawDataDetails("Raw managed file data", found)))
	data := pageData{
		Title:    "Managed File Detail",
		Subtitle: "Review the managed file binding or retire it from active use.",
		Items:    panelSectionHTML("", "Current managed file", template.HTML(body.String())),
	}
	if d.canWrite(session) && found.Status != managedfiles.StatusRetired {
		data.Forms = []formData{{
			Title:  "Retire managed file",
			Action: "/admin/managed-files/" + found.ID + "/retire",
			Submit: "Retire managed file",
			Danger: true,
		}}
	}
	return data
}

func (d *dashboard) downloadManagedFile(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Managed File Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if d.deps.Artifacts == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	found, err := d.loadManagedFileDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	if found.File == nil || found.File.Artifact == nil || found.Status != managedfiles.StatusActive {
		http.NotFound(w, r)
		return
	}
	body, err := d.deps.Artifacts.Get(r.Context(), found.File.Artifact.StorageKey)
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		d.renderPageError(w, r, session, "Managed File Detail", err)
		return
	}
	w.Header().Set("Content-Type", found.File.Artifact.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
	w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
	downloadName := found.File.Name
	if downloadName == "" {
		downloadName = found.Path
	}
	if downloadName == "" {
		downloadName = found.ID
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(content))
}

func (d *dashboard) createManagedFile(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Managed Files") {
		return
	}
	fileReq, bindingReq, content, err := managedFileUpsertFromMultipart(r)
	if err != nil {
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	if len(content) > 0 {
		if err := d.deps.Artifacts.Put(r.Context(), fileReq.StorageKey, bytes.NewReader(content), fileReq.MimeType, int64(len(content))); err != nil {
			d.redirectError(w, r, "/admin/managed-files", err.Error())
			return
		}
	}
	fileRec, err := d.deps.Files.CreateFile(r.Context(), d.deps.TenantID, fileReq)
	if err != nil {
		_ = d.deps.Artifacts.Delete(r.Context(), fileReq.StorageKey)
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	bindingReq.FileID = fileRec.ID
	rec, err := d.deps.ManagedFiles.CreateManagedFile(r.Context(), d.deps.TenantID, bindingReq)
	if err != nil {
		_, _ = d.deps.Files.RetireFile(r.Context(), d.deps.TenantID, fileRec.ID)
		_ = d.deps.Artifacts.Delete(r.Context(), fileReq.StorageKey)
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	d.recordAudit(r, "create", "managed_files", rec.ID, map[string]any{"fileId": rec.FileID, "path": rec.Path, "sourceName": fileRec.Name, "replaceVariables": rec.ReplaceVariables})
	d.redirectOK(w, r, "/admin/managed-files", "managed file uploaded")
}

func (d *dashboard) retireManagedFile(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Managed Files") {
		return
	}
	rec, err := d.deps.ManagedFiles.RetireManagedFile(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/managed-files", err.Error())
		return
	}
	d.recordAudit(r, "retire", "managed_files", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/managed-files", "managed file retired")
}

func managedFileUpsertFromMultipart(r *http.Request) (files.FileUpsert, managedfiles.ManagedFileUpsert, []byte, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return files.FileUpsert{}, managedfiles.ManagedFileUpsert{}, nil, err
	}
	content, err := uploadedContent(r)
	if err != nil {
		return files.FileUpsert{}, managedfiles.ManagedFileUpsert{}, nil, err
	}
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		return files.FileUpsert{}, managedfiles.ManagedFileUpsert{}, nil, fmt.Errorf("path is required")
	}
	filename := uploadedFileName(r)
	return files.FileUpsert{
			Name:       managedFileName(path, filename),
			StorageKey: managedFileStorageKey(path, filename),
			Checksum:   checksum.SHA256Base64URL(content),
			SizeBytes:  int64(len(content)),
			MimeType:   managedFileMimeType(filename, content),
		},
		managedfiles.ManagedFileUpsert{
			Path:             path,
			ReplaceVariables: hasFormField(r, "replaceVariables"),
		},
		content,
		nil
}

func uploadedContent(r *http.Request) ([]byte, error) {
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	return content, nil
}

func uploadedFileName(r *http.Request) string {
	if r.MultipartForm == nil {
		return ""
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 || files[0] == nil {
		return ""
	}
	return strings.TrimSpace(files[0].Filename)
}

func managedFileName(path, filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." {
		base = strings.TrimSpace(filepath.Base(path))
	}
	if base == "" || base == "." {
		base = "managed-file"
	}
	base = strings.NewReplacer("/", "_", "\\", "_").Replace(base)
	pathSuffix := checksum.SHA256Base64URL([]byte(path))
	if len(pathSuffix) > 8 {
		pathSuffix = pathSuffix[:8]
	}
	uploadSuffix := uuid.NewString()
	if len(uploadSuffix) > 8 {
		uploadSuffix = uploadSuffix[:8]
	}
	return "managed-" + base + "-" + pathSuffix + "-" + uploadSuffix
}

func managedFileStorageKey(path, filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." {
		base = strings.TrimSpace(filepath.Base(path))
	}
	if base == "" || base == "." {
		base = "managed-file"
	}
	base = strings.NewReplacer("/", "_", "\\", "_").Replace(base)
	return "artifacts/managed-files/" + uuid.NewString() + "/" + base
}

func managedFileMimeType(filename string, content []byte) string {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		if found := mime.TypeByExtension(ext); found != "" {
			return found
		}
	}
	if detected := http.DetectContentType(content); detected != "" {
		return detected
	}
	return "application/octet-stream"
}

func managedFilesTable(items []managedfiles.ManagedFile) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		path := item.Path
		if strings.TrimSpace(path) == "" {
			path = item.ID
		}
		fileLabel := item.FileID
		if item.File != nil && strings.TrimSpace(item.File.Name) != "" {
			fileLabel = item.File.Name
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(`<a href="/admin/managed-files/`+escAttr(item.ID)+`">`+esc(path)+`</a>`),
			template.HTML(esc(fileLabel)),
			template.HTML(boolBadge(item.ReplaceVariables, "enabled", "disabled")),
			template.HTML(statusBadge(item.Status)),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Path", "File", "Template", "Status"}, rows.String())
}

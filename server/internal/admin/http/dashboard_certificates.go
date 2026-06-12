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
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/pagination"

	"github.com/google/uuid"
)

func (d *dashboard) certificates(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Certificates")
	if !ok {
		return
	}
	page, params := listPaginationParams(r, pagination.DefaultLimit)
	limit := params.Limit - 1
	items, err := d.deps.Certificates.ListCertificates(r.Context(), d.deps.TenantID, params)
	if err != nil {
		d.renderPageError(w, r, session, "Certificates", err)
		return
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	d.renderForSession(w, r, session, pageData{Title: "Certificates", Subtitle: "Upload certificate artifacts for policy distribution.", Forms: []formData{certificateUploadForm("/admin/certificates/create")}, Items: withPager(certificatesTable(items), pagerHTML(r, page, limit, hasNext))})
}

func (d *dashboard) certificateDetail(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Certificate Detail")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	found, err := d.loadCertificateDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	d.renderForSession(w, r, session, d.certificateDetailPageData(session, found))
}

func (d *dashboard) downloadCertificate(w http.ResponseWriter, r *http.Request) {
	session, ok := d.requireRead(w, r, "Certificate Detail")
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
	found, err := d.loadCertificateDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	if found.Artifact == nil {
		http.NotFound(w, r)
		return
	}
	body, err := d.deps.Artifacts.Get(r.Context(), found.Artifact.StorageKey)
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		d.renderPageError(w, r, session, "Certificate Detail", err)
		return
	}
	w.Header().Set("Content-Type", found.Artifact.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
	w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
	downloadName := found.Name
	if downloadName == "" {
		downloadName = found.ID
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(content))
}

func (d *dashboard) loadCertificateDetail(ctx context.Context, id string) (*certificates.Certificate, error) {
	found, err := d.deps.Certificates.GetCertificate(ctx, d.deps.TenantID, id)
	if err != nil {
		return nil, err
	}
	return &found, nil
}

func (d *dashboard) certificateDetailPageData(session *auth.Session, found *certificates.Certificate) pageData {
	var body strings.Builder
	body.WriteString(`<div class="policy-summary">`)
	body.WriteString(summaryTextItem("Created", formatDashboardTime(found.CreatedAt)))
	body.WriteString(summaryTextItem("Updated", formatDashboardTime(found.UpdatedAt)))
	body.WriteString(summaryTextItem("ID", found.ID))
	body.WriteString(summaryTextItem("Name", found.Name))
	body.WriteString(summaryTextItem("Artifact", found.ArtifactID))
	body.WriteString(summaryTextItem("Checksum", found.Checksum))
	if found.Artifact != nil {
		body.WriteString(summaryHTMLItem("Download", template.HTML(fmt.Sprintf(`<a class="button btn-primary" href="/admin/certificates/%s/download">Download certificate</a>`, escAttr(found.ID)))))
	}
	if found.Artifact != nil {
		body.WriteString(summaryTextItem("Storage key", found.Artifact.StorageKey))
		body.WriteString(summaryTextItem("Size", strconv.FormatInt(found.Artifact.SizeBytes, 10)))
		body.WriteString(summaryTextItem("MIME", found.Artifact.MimeType))
	}
	body.WriteString(summaryHTMLItem("Status", template.HTML(statusBadge(found.Status))))
	body.WriteString(`</div>`)
	body.WriteString(string(rawDataDetails("Raw certificate data", found)))
	data := pageData{
		Title:    "Certificate Detail",
		Subtitle: "Review the certificate artifact or retire it from active use.",
		Items:    panelSectionHTML("", "Current certificate", template.HTML(body.String())),
	}
	if d.canWrite(session) && found.Status != certificates.StatusRetired {
		data.Forms = []formData{{
			Title:  "Retire certificate",
			Action: "/admin/certificates/" + found.ID + "/retire",
			Submit: "Retire certificate",
			Danger: true,
		}}
	}
	return data
}

func (d *dashboard) createCertificate(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Certificates") {
		return
	}
	req, content, err := certificateUpsertFromMultipart(r)
	if err != nil {
		d.redirectError(w, r, "/admin/certificates", err.Error())
		return
	}
	if len(content) > 0 {
		if actual := checksum.SHA256Base64URL(content); actual != req.Checksum {
			d.redirectError(w, r, "/admin/certificates", "checksum mismatch")
			return
		}
		if err := d.deps.Artifacts.Put(r.Context(), req.StorageKey, bytes.NewReader(content), req.MimeType, int64(len(content))); err != nil {
			d.redirectError(w, r, "/admin/certificates", err.Error())
			return
		}
	}
	rec, err := d.deps.Certificates.CreateCertificate(r.Context(), d.deps.TenantID, req)
	if err != nil {
		_ = d.deps.Artifacts.Delete(r.Context(), req.StorageKey)
		d.redirectError(w, r, "/admin/certificates", err.Error())
		return
	}
	d.recordAudit(r, "create", "certificates", rec.ID, map[string]any{"name": rec.Name, "checksum": rec.Checksum, "artifactId": rec.ArtifactID})
	d.redirectOK(w, r, "/admin/certificates", "certificate uploaded")
}

func (d *dashboard) retireCertificate(w http.ResponseWriter, r *http.Request) {
	if !d.requirePostWrite(w, r, "Certificates") {
		return
	}
	rec, err := d.deps.Certificates.RetireCertificate(r.Context(), d.deps.TenantID, r.PathValue("id"))
	if err != nil {
		d.redirectError(w, r, "/admin/certificates", err.Error())
		return
	}
	d.recordAudit(r, "retire", "certificates", rec.ID, map[string]any{"status": rec.Status})
	d.redirectOK(w, r, "/admin/certificates", "certificate retired")
}

func certificateUploadForm(action string) formData {
	return formData{
		Title:   "Upload certificate",
		Action:  action,
		EncType: "multipart/form-data",
		Fields: []fieldData{
			{Name: "name", Label: "Name", Type: "text", Placeholder: "Root CA", Required: true},
			{Name: "file", Label: "File", Type: "file", Required: true},
		},
		Help:   "",
		Submit: "Upload certificate",
	}
}

func certificateUpsertFromMultipart(r *http.Request) (certificates.CertificateUpsert, []byte, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return certificates.CertificateUpsert{}, nil, err
	}
	content, err := uploadedContent(r)
	if err != nil {
		return certificates.CertificateUpsert{}, nil, err
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return certificates.CertificateUpsert{}, nil, fmt.Errorf("name is required")
	}
	filename := uploadedFileName(r)
	return certificates.CertificateUpsert{
		Name:       name,
		StorageKey: certificateStorageKey(name, filename),
		Checksum:   checksum.SHA256Base64URL(content),
		SizeBytes:  int64(len(content)),
		MimeType:   certificateMimeType(filename, content),
	}, content, nil
}

func certificateStorageKey(name, filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." {
		base = strings.TrimSpace(name)
	}
	if base == "" {
		base = "certificate"
	}
	base = strings.NewReplacer("/", "_", "\\", "_").Replace(base)
	return "artifacts/certificates/" + uuid.NewString() + "/" + base
}

func certificateMimeType(filename string, content []byte) string {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		if ext == ".pem" {
			return "application/x-pem-file"
		}
		if ext == ".cer" || ext == ".crt" || ext == ".der" {
			return "application/x-x509-ca-cert"
		}
		if ext == ".p12" || ext == ".pfx" {
			return "application/x-pkcs12"
		}
		if found := mime.TypeByExtension(ext); found != "" {
			return found
		}
	}
	if detected := http.DetectContentType(content); detected != "" {
		return detected
	}
	return "application/octet-stream"
}

func certificatesTable(items []certificates.Certificate) template.HTML {
	var rows strings.Builder
	for _, item := range items {
		artifact := item.ArtifactID
		if item.Artifact != nil && strings.TrimSpace(item.Artifact.StorageKey) != "" {
			artifact = item.Artifact.StorageKey
		}
		rows.WriteString(tableRowHTML(
			template.HTML(esc(formatDashboardTime(item.CreatedAt))),
			template.HTML(esc(item.ID)),
			template.HTML(`<a href="/admin/certificates/`+escAttr(item.ID)+`">`+esc(item.Name)+`</a>`),
			template.HTML(esc(artifact)),
			template.HTML(statusBadge(item.Status)),
		))
	}
	return tableHTML("", []string{"Created", "ID", "Name", "Artifact", "Status"}, rows.String())
}

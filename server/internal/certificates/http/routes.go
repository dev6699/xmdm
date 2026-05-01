package certificatehttp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
)

const deviceSecretHeader = "X-XMDM-Device-Secret"

func Register(mux httpx.Router, svc *auth.Service, devices device.Repository, store certificates.Repository, artifactStore artifacts.Store, auditStore audit.Store, tenantID string) {
	mux.HandleFunc("/devices/{deviceId}/certificates/{certificateId}/artifact", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if devices == nil || store == nil || artifactStore == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		deviceID := strings.TrimSpace(r.PathValue("deviceId"))
		certificateID := strings.TrimSpace(r.PathValue("certificateId"))
		secret := strings.TrimSpace(r.Header.Get(deviceSecretHeader))
		if deviceID == "" || certificateID == "" || secret == "" {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		if _, err := devices.Authenticate(r.Context(), tenantID, deviceID, secret); err != nil {
			switch err {
			case httpx.ErrInvalidInput:
				log.Printf("certificates artifact auth invalid input: device=%s certificate=%s", deviceID, certificateID)
				http.Error(w, "invalid input", http.StatusBadRequest)
			case httpx.ErrNotFound:
				log.Printf("certificates artifact auth unauthorized: device=%s certificate=%s", deviceID, certificateID)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			default:
				log.Printf("certificates artifact auth failed: device=%s certificate=%s err=%v", deviceID, certificateID, err)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}
		rec, err := store.GetCertificate(r.Context(), tenantID, certificateID)
		if err != nil {
			if err == httpx.ErrNotFound {
				http.NotFound(w, r)
				return
			}
			log.Printf("certificates artifact load failed: device=%s certificate=%s err=%v", deviceID, certificateID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if rec.Status != certificates.StatusActive || rec.Artifact == nil {
			http.NotFound(w, r)
			return
		}
		body, err := artifactStore.Get(r.Context(), rec.Artifact.StorageKey)
		if err != nil {
			log.Printf("certificates artifact fetch failed: device=%s certificate=%s storage_key=%s err=%v", deviceID, certificateID, rec.Artifact.StorageKey, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer body.Close()
		content, err := io.ReadAll(body)
		if err != nil {
			log.Printf("certificates artifact read failed: device=%s certificate=%s storage_key=%s err=%v", deviceID, certificateID, rec.Artifact.StorageKey, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", rec.Artifact.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
		w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
		w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
		downloadName := rec.Name
		if downloadName == "" {
			downloadName = rec.ID
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+downloadName+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, bytes.NewReader(content))
	})

	mux.HandleFunc("/certificates", func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if !auth.HasPermission(session.Permissions, auth.PermissionAdminRead) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			items, err := store.ListCertificates(r.Context(), tenantID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, items)
		case http.MethodPost:
			if !auth.HasPermission(session.Permissions, auth.PermissionAdminWrite) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			payload, content, err := decodeCertificateRequest(r)
			if err != nil {
				if err == httpx.ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if len(content) > 0 {
				actualChecksum := checksum.SHA256Base64URL(content)
				if actualChecksum != payload.Checksum {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				if err := artifactStore.Put(r.Context(), payload.StorageKey, bytes.NewReader(content), payload.MimeType, int64(len(content))); err != nil {
					if err == httpx.ErrInvalidInput {
						http.Error(w, "invalid input", http.StatusBadRequest)
						return
					}
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
			}
			rec, err := store.CreateCertificate(r.Context(), tenantID, payload)
			if err != nil {
				if len(content) > 0 {
					_ = artifactStore.Delete(r.Context(), payload.StorageKey)
				}
				switch err {
				case httpx.ErrInvalidInput:
					http.Error(w, "invalid input", http.StatusBadRequest)
				case httpx.ErrConflict:
					http.Error(w, "conflict", http.StatusConflict)
				default:
					http.Error(w, "internal error", http.StatusInternalServerError)
				}
				return
			}
			details := map[string]any{
				"name":       rec.Name,
				"checksum":   rec.Checksum,
				"artifactId": rec.ArtifactID,
			}
			if rec.Artifact != nil {
				details["storageKey"] = rec.Artifact.StorageKey
				details["sizeBytes"] = rec.Artifact.SizeBytes
				details["mimeType"] = rec.Artifact.MimeType
			}
			if _, err := auditStore.Record(r.Context(), tenantID, session.Username, "create", "certificates", rec.RecordID(), details); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/certificates/{id}", func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodDelete:
			if !auth.HasPermission(session.Permissions, auth.PermissionAdminWrite) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			rec, err := store.RetireCertificate(r.Context(), tenantID, id)
			if err != nil {
				if err == httpx.ErrNotFound {
					http.NotFound(w, r)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			details := map[string]any{
				"name":     rec.Name,
				"status":   rec.Status,
				"checksum": rec.Checksum,
			}
			if rec.Artifact != nil {
				details["artifactId"] = rec.Artifact.ID
			}
			if _, err := auditStore.Record(r.Context(), tenantID, session.Username, "retire", "certificates", rec.RecordID(), details); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func decodeCertificateRequest(r *http.Request) (certificates.CertificateUpsert, []byte, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return certificates.CertificateUpsert{}, nil, err
		}
		payload := certificates.CertificateUpsert{
			Name:       r.FormValue("name"),
			StorageKey: r.FormValue("storageKey"),
			Checksum:   r.FormValue("checksum"),
			MimeType:   r.FormValue("mimeType"),
		}
		sizeBytes, err := strconv.ParseInt(r.FormValue("sizeBytes"), 10, 64)
		if err != nil {
			return certificates.CertificateUpsert{}, nil, httpx.ErrInvalidInput
		}
		payload.SizeBytes = sizeBytes
		file, _, err := r.FormFile("file")
		if err != nil {
			return certificates.CertificateUpsert{}, nil, httpx.ErrInvalidInput
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return certificates.CertificateUpsert{}, nil, err
		}
		if payload.Name == "" || payload.StorageKey == "" || payload.Checksum == "" || payload.MimeType == "" || payload.SizeBytes < 0 {
			return certificates.CertificateUpsert{}, nil, httpx.ErrInvalidInput
		}
		if payload.SizeBytes != int64(len(content)) {
			return certificates.CertificateUpsert{}, nil, httpx.ErrInvalidInput
		}
		return payload, content, nil
	}
	var payload certificates.CertificateUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return certificates.CertificateUpsert{}, nil, err
	}
	if payload.Name == "" || payload.StorageKey == "" || payload.Checksum == "" || payload.MimeType == "" || payload.SizeBytes < 0 {
		return certificates.CertificateUpsert{}, nil, httpx.ErrInvalidInput
	}
	return payload, nil, nil
}

func sessionFromRequest(r *http.Request, svc *auth.Service) (*auth.Session, bool) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return nil, false
	}
	session, ok := svc.Authenticate(cookie.Value)
	if !ok {
		return nil, false
	}
	return session, true
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

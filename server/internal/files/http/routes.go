package filehttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
)

func Register(mux httpx.Router, svc *auth.Service, store files.Repository, artifactStore artifacts.Store, auditStore audit.Store, tenantID string) {
	mux.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if !auth.HasPermission(session.Permissions, auth.PermissionAdminRead) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			items, err := store.ListFiles(r.Context(), tenantID)
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
			payload, content, err := decodeFileRequest(r)
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
			rec, err := store.CreateFile(r.Context(), tenantID, payload)
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
				"mimeType":   rec.MimeType,
				"artifactId": rec.ArtifactID,
			}
			if rec.Artifact != nil {
				details["storageKey"] = rec.Artifact.StorageKey
				details["sizeBytes"] = rec.Artifact.SizeBytes
			}
			if _, err := auditStore.Record(r.Context(), tenantID, session.Username, "create", "files", rec.RecordID(), details); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
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
			rec, err := store.RetireFile(r.Context(), tenantID, id)
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
			if _, err := auditStore.Record(r.Context(), tenantID, session.Username, "retire", "files", rec.RecordID(), details); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func decodeFileRequest(r *http.Request) (files.FileUpsert, []byte, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return files.FileUpsert{}, nil, err
		}
		payload := files.FileUpsert{
			Name:       r.FormValue("name"),
			StorageKey: r.FormValue("storageKey"),
			Checksum:   r.FormValue("checksum"),
			MimeType:   r.FormValue("mimeType"),
		}
		sizeBytes, err := strconv.ParseInt(r.FormValue("sizeBytes"), 10, 64)
		if err != nil {
			return files.FileUpsert{}, nil, httpx.ErrInvalidInput
		}
		payload.SizeBytes = sizeBytes
		file, _, err := r.FormFile("file")
		if err != nil {
			return files.FileUpsert{}, nil, httpx.ErrInvalidInput
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return files.FileUpsert{}, nil, err
		}
		if payload.Name == "" || payload.StorageKey == "" || payload.Checksum == "" || payload.MimeType == "" || payload.SizeBytes < 0 {
			return files.FileUpsert{}, nil, httpx.ErrInvalidInput
		}
		if payload.SizeBytes != int64(len(content)) {
			return files.FileUpsert{}, nil, httpx.ErrInvalidInput
		}
		return payload, content, nil
	}
	var payload files.FileUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return files.FileUpsert{}, nil, err
	}
	if payload.Name == "" || payload.StorageKey == "" || payload.Checksum == "" || payload.MimeType == "" || payload.SizeBytes < 0 {
		return files.FileUpsert{}, nil, httpx.ErrInvalidInput
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

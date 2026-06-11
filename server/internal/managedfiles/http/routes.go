package managedfilehttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/managedfiles"
)

const deviceSecretHeader = "X-XMDM-Device-Secret"

func Register(mux httpx.Router, svc *auth.Service, store managedfiles.Repository, devices device.Repository, artifactStore artifacts.Store, tenantID string) {
	mux.HandleFunc("/devices/{deviceId}/managed-files/{managedFileId}/artifact", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if store == nil || devices == nil || artifactStore == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		deviceID := strings.TrimSpace(r.PathValue("deviceId"))
		managedFileID := strings.TrimSpace(r.PathValue("managedFileId"))
		secret := strings.TrimSpace(r.Header.Get(deviceSecretHeader))
		if deviceID == "" || managedFileID == "" || secret == "" {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		authDevice, err := devices.Authenticate(r.Context(), tenantID, deviceID, secret)
		if err != nil {
			switch err {
			case httpx.ErrInvalidInput:
				log.Printf("managed files artifact auth invalid input: device=%s managed_file=%s", deviceID, managedFileID)
				http.Error(w, "invalid input", http.StatusBadRequest)
			case httpx.ErrNotFound:
				log.Printf("managed files artifact auth unauthorized: device=%s managed_file=%s", deviceID, managedFileID)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			default:
				log.Printf("managed files artifact auth failed: device=%s managed_file=%s err=%v", deviceID, managedFileID, err)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}
		managedFile, err := store.GetManagedFile(r.Context(), tenantID, managedFileID)
		if err != nil {
			if err == httpx.ErrNotFound {
				http.NotFound(w, r)
				return
			}
			log.Printf("managed files artifact load failed: device=%s managed_file=%s err=%v", deviceID, managedFileID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if managedFile.Status != managedfiles.StatusActive || managedFile.File == nil || managedFile.File.Artifact == nil {
			http.NotFound(w, r)
			return
		}
		body, err := artifactStore.Get(r.Context(), managedFile.File.Artifact.StorageKey)
		if err != nil {
			log.Printf("managed files artifact fetch failed: device=%s managed_file=%s storage_key=%s err=%v", deviceID, managedFileID, managedFile.File.Artifact.StorageKey, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer body.Close()
		content, err := io.ReadAll(body)
		if err != nil {
			log.Printf("managed files artifact read failed: device=%s managed_file=%s storage_key=%s err=%v", deviceID, managedFileID, managedFile.File.Artifact.StorageKey, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if managedFile.ReplaceVariables {
			content = managedfiles.RenderTemplate(content, deviceID, authDevice.BootstrapExtras)
		}
		w.Header().Set("Content-Type", managedFile.File.Artifact.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
		w.Header().Set("X-XMDM-Artifact-Checksum", checksum.SHA256Base64URL(content))
		w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(int64(len(content)), 10))
		downloadName := managedFile.Path
		if downloadName == "" {
			downloadName = managedFile.File.Name
		}
		if downloadName == "" {
			downloadName = managedFile.ID
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, bytes.NewReader(content))
	})

	mux.HandleFunc("/managed-files", func(w http.ResponseWriter, r *http.Request) {
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
			items, err := store.ListManagedFiles(r.Context(), tenantID, httpx.RequestPaginationParams(r, 100))
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
			payload, err := decodeManagedFileRequest(r)
			if err != nil {
				if err == httpx.ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			rec, err := store.CreateManagedFile(r.Context(), tenantID, payload)
			if err != nil {
				switch err {
				case httpx.ErrInvalidInput:
					http.Error(w, "invalid input", http.StatusBadRequest)
				case httpx.ErrNotFound:
					http.NotFound(w, r)
				case httpx.ErrConflict:
					http.Error(w, "conflict", http.StatusConflict)
				default:
					http.Error(w, "internal error", http.StatusInternalServerError)
				}
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/managed-files/{id}", func(w http.ResponseWriter, r *http.Request) {
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
			rec, err := store.RetireManagedFile(r.Context(), tenantID, id)
			if err != nil {
				if err == httpx.ErrNotFound {
					http.NotFound(w, r)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func decodeManagedFileRequest(r *http.Request) (managedfiles.ManagedFileUpsert, error) {
	var payload managedfiles.ManagedFileUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return managedfiles.ManagedFileUpsert{}, err
	}
	if payload.FileID == "" || strings.TrimSpace(payload.Path) == "" {
		return managedfiles.ManagedFileUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
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

package apphttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
)

const deviceSecretHeader = "X-XMDM-Device-Secret"

func Register(mux httpx.Router, svc *auth.Service, store apps.Repository, devices device.Repository, artifactStore artifacts.Store, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[apps.AppUpsert, apps.App]{
		Kind:      "apps",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodeAppRequest,
		List: func(ctx context.Context) ([]apps.App, error) {
			return store.ListApps(ctx, tenantID)
		},
		Create: func(ctx context.Context, req apps.AppUpsert) (apps.App, error) {
			return store.CreateApp(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req apps.AppUpsert) (apps.App, error) {
			return store.UpdateApp(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (apps.App, error) {
			return store.RetireApp(ctx, tenantID, id)
		},
		Audit: func(rec apps.App) map[string]any {
			return map[string]any{
				"packageName": rec.PackageName,
				"name":        rec.Name,
			}
		},
	})

	mux.HandleFunc("/devices/{deviceId}/apps/{appId}/versions/{versionId}/artifact", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if devices == nil || artifactStore == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		deviceID := strings.TrimSpace(r.PathValue("deviceId"))
		appID := strings.TrimSpace(r.PathValue("appId"))
		versionID := strings.TrimSpace(r.PathValue("versionId"))
		secret := strings.TrimSpace(r.Header.Get(deviceSecretHeader))
		if deviceID == "" || appID == "" || versionID == "" || secret == "" {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		if _, err := devices.Authenticate(r.Context(), tenantID, deviceID, secret); err != nil {
			switch err {
			case httpx.ErrInvalidInput:
				log.Printf("apps artifact auth invalid input: device=%s app=%s version=%s", deviceID, appID, versionID)
				http.Error(w, "invalid input", http.StatusBadRequest)
			case httpx.ErrNotFound:
				log.Printf("apps artifact auth unauthorized: device=%s app=%s version=%s", deviceID, appID, versionID)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			default:
				log.Printf("apps artifact auth failed: device=%s app=%s version=%s err=%v", deviceID, appID, versionID, err)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}
		version, err := store.GetVersion(r.Context(), tenantID, appID, versionID)
		if err != nil {
			if err == httpx.ErrNotFound {
				http.NotFound(w, r)
				return
			}
			log.Printf("apps artifact version load failed: device=%s app=%s version=%s err=%v", deviceID, appID, versionID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if version.Status != apps.VersionStatusPublished || version.ArtifactID == nil || version.Artifact == nil {
			log.Printf("apps artifact version not downloadable: device=%s app=%s version=%s status=%s artifact_id_nil=%t artifact_nil=%t", deviceID, appID, versionID, version.Status, version.ArtifactID == nil, version.Artifact == nil)
			http.NotFound(w, r)
			return
		}
		body, err := artifactStore.Get(r.Context(), version.Artifact.StorageKey)
		if err != nil {
			log.Printf("apps artifact fetch failed: device=%s app=%s version=%s storage_key=%s err=%v", deviceID, appID, versionID, version.Artifact.StorageKey, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer body.Close()
		w.Header().Set("Content-Type", version.Artifact.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(version.Artifact.SizeBytes, 10))
		w.Header().Set("X-XMDM-Artifact-Checksum", version.Artifact.Checksum)
		w.Header().Set("X-XMDM-Artifact-Size", strconv.FormatInt(version.Artifact.SizeBytes, 10))
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.apk"`, appID, version.VersionName))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, body)
	})

	mux.HandleFunc("/apps/{id}/versions", func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		appID := r.PathValue("id")
		if appID == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if !auth.HasPermission(session.Permissions, auth.PermissionAdminRead) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			versions, err := store.ListVersions(r.Context(), tenantID, appID)
			if err != nil {
				if err == httpx.ErrNotFound {
					http.NotFound(w, r)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, versions)
		case http.MethodPost:
			if !auth.HasPermission(session.Permissions, auth.PermissionAdminWrite) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			payload, err := decodeVersionRequest(r)
			if err != nil {
				if err == httpx.ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			version, err := store.CreateVersion(r.Context(), tenantID, appID, payload)
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
			details := map[string]any{
				"versionName": version.VersionName,
				"versionCode": version.VersionCode,
				"status":      version.Status,
			}
			if version.ArtifactID != nil {
				details["artifactId"] = *version.ArtifactID
			}
			if _, err := auditStore.Record(r.Context(), tenantID, session.Username, "create", "app_versions", version.RecordID(), details); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, version)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func decodeAppRequest(r *http.Request) (apps.AppUpsert, error) {
	var payload apps.AppUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return apps.AppUpsert{}, err
	}
	if payload.PackageName == "" || payload.Name == "" {
		return apps.AppUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}

func decodeVersionRequest(r *http.Request) (apps.VersionUpsert, error) {
	var payload apps.VersionUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return apps.VersionUpsert{}, err
	}
	if payload.VersionName == "" || payload.VersionCode <= 0 || payload.Checksum == "" {
		return apps.VersionUpsert{}, httpx.ErrInvalidInput
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

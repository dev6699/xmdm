package loghttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/logs"
	"xmdm/server/internal/pagination"
)

const deviceSecretHeader = "X-XMDM-Device-Secret"

type UploadResponse struct {
	Logs []logs.Record `json:"logs"`
}

func Register(mux httpx.Router, svc *auth.Service, devices device.Repository, store logs.Repository, tenantID string) {
	mux.HandleFunc("/devices/{deviceId}/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if devices == nil || store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		deviceID := strings.TrimSpace(r.PathValue("deviceId"))
		secret := strings.TrimSpace(r.Header.Get(deviceSecretHeader))
		if deviceID == "" || secret == "" {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		if _, err := devices.Authenticate(r.Context(), tenantID, deviceID, secret); err != nil {
			switch err {
			case httpx.ErrInvalidInput:
				http.Error(w, "invalid input", http.StatusBadRequest)
			case httpx.ErrNotFound:
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			default:
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}
		req, err := decodeUploadRequest(r)
		if err != nil {
			switch err {
			case httpx.ErrInvalidInput, logs.ErrLogsInvalid:
				http.Error(w, "invalid input", http.StatusBadRequest)
			default:
				http.Error(w, "invalid json", http.StatusBadRequest)
			}
			return
		}
		rec, err := store.Upload(r.Context(), tenantID, deviceID, secret, req)
		if err != nil {
			writeLogsError(w, err)
			return
		}
		writeJSON(w, UploadResponse{Logs: rec})
	})

	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionAdminRead) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		filter, err := decodeSearchFilter(r)
		if err != nil {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		rec, err := store.Search(r.Context(), tenantID, filter)
		if err != nil {
			writeLogsError(w, err)
			return
		}
		writeJSON(w, UploadResponse{Logs: rec})
	})
}

func decodeUploadRequest(r *http.Request) (logs.UploadRequest, error) {
	var payload logs.UploadRequest
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return logs.UploadRequest{}, err
	}
	if len(payload.Entries) == 0 {
		return logs.UploadRequest{}, logs.ErrLogsInvalid
	}
	return payload, nil
}

func decodeSearchFilter(r *http.Request) (logs.SearchFilter, error) {
	var filter logs.SearchFilter
	filter.DeviceID = strings.TrimSpace(r.URL.Query().Get("deviceId"))
	filter.Source = strings.TrimSpace(r.URL.Query().Get("source"))
	filter.Level = strings.TrimSpace(r.URL.Query().Get("level"))
	filter.Query = strings.TrimSpace(r.URL.Query().Get("q"))
	if since := strings.TrimSpace(r.URL.Query().Get("since")); since != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return logs.SearchFilter{}, httpx.ErrInvalidInput
		}
		filter.Since = &parsed
	}
	if until := strings.TrimSpace(r.URL.Query().Get("until")); until != "" {
		parsed, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return logs.SearchFilter{}, httpx.ErrInvalidInput
		}
		filter.Until = &parsed
	}
	if limit := strings.TrimSpace(r.URL.Query().Get("limit")); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil || value <= 0 {
			return logs.SearchFilter{}, httpx.ErrInvalidInput
		}
		filter.Limit = value
	}
	filter.Pagination = pagination.Params{Limit: filter.Limit, Offset: filter.Offset}
	return filter, nil
}

func writeLogsError(w http.ResponseWriter, err error) {
	switch err {
	case httpx.ErrInvalidInput, logs.ErrLogsInvalid:
		http.Error(w, "invalid input", http.StatusBadRequest)
	case logs.ErrDeviceNotFound:
		http.Error(w, "not found", http.StatusNotFound)
	case logs.ErrDeviceUnauthorized:
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
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

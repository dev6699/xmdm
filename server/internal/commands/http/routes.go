package commandhttp

import (
	"encoding/json"
	"net/http"
	"strings"

	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
)

const deviceSecretHeader = "X-XMDM-Device-Secret"

type PollResponse struct {
	Commands []commands.Command `json:"commands"`
}

func Register(mux httpx.Router, devices device.Repository, store commands.Repository, tenantID string) {
	mux.HandleFunc("/devices/{deviceId}/commands", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
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
		items, err := store.ListPending(r.Context(), tenantID, deviceID)
		if err != nil {
			switch err {
			case httpx.ErrInvalidInput:
				http.Error(w, "invalid input", http.StatusBadRequest)
			case httpx.ErrNotFound:
				http.NotFound(w, r)
			default:
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, PollResponse{Commands: items})
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

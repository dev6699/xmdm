package plugins

import (
	"encoding/json"
	"net/http"
)

type Manager struct {
	enabled bool
}

func Disabled() *Manager {
	return &Manager{}
}

func Enabled() *Manager {
	return &Manager{enabled: true}
}

func (m *Manager) Register(mux *http.ServeMux) {
	if m == nil || !m.enabled {
		return
	}
	mux.HandleFunc("/admin/plugins", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{"demo-plugin"})
	})
}

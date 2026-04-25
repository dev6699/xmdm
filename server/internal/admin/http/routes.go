package adminhttp

import (
	"encoding/json"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/plugins"
)

func Register(mux httpx.Router, svc *auth.Service, pluginManager *plugins.Manager, auditStore audit.Store, commandStore commands.Repository, tenantID string) {
	adminMux := httpx.WithPrefix(mux, "/admin")
	registerSessionRoutes(adminMux, svc)
	registerCommandRoutes(adminMux, svc, auditStore, commandStore, tenantID)
	if pluginManager != nil {
		pluginManager.Register(adminMux)
	}
}

type commandCreateRequest struct {
	Type    string          `json:"type"`
	Payload map[string]any  `json:"payload,omitempty"`
	Target  commands.Target `json:"target"`
}

func registerSessionRoutes(mux httpx.Router, svc *auth.Service) {
	loginPath := "/login"
	logoutPath := "/logout"
	mePath := "me"

	mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<form method="post"><input name="username"><input name="password" type="password"><button type="submit">Login</button></form>`))
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			session, err := svc.Login(r.FormValue("username"), r.FormValue("password"))
			if err != nil {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     auth.SessionCookieName,
				Value:    session.ID,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Expires:  session.ExpiresAt,
			})
			http.Redirect(w, r, mePath, http.StatusSeeOther)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(logoutPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
			svc.Logout(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     auth.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc(mePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":"` + session.Username + `"}`))
	})
}

func registerCommandRoutes(mux httpx.Router, svc *auth.Service, auditStore audit.Store, commandStore commands.Repository, tenantID string) {
	mux.HandleFunc("/commands", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionAdminWrite) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if commandStore == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var req commandCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Type == "" {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		created, err := commandStore.Enqueue(r.Context(), tenantID, commands.Upsert{
			Type:    req.Type,
			Payload: req.Payload,
			Target:  req.Target,
		})
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
		if auditStore != nil {
			for _, rec := range created {
				details := map[string]any{
					"type":   rec.Type,
					"status": rec.Status,
					"target": req.Target.Type,
				}
				if rec.DeviceID != "" {
					details["deviceId"] = rec.DeviceID
				}
				if _, err := auditStore.Record(r.Context(), tenantID, session.Username, "create", "commands", rec.ID, details); err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"commands": created})
	})
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

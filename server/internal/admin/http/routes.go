package adminhttp

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

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

var commandFormTemplate = template.Must(template.New("admin-commands").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>XMDM Commands</title></head>
<body>
<h1>Create Command</h1>
<form method="post">
  <label>Type <input name="type" value="reboot"></label><br>
  <label>Target
    <select name="targetType">
      <option value="broadcast">Broadcast</option>
      <option value="device">Device</option>
      <option value="group">Group</option>
    </select>
  </label><br>
  <label>Device ID <input name="targetDeviceId" placeholder="device-123"></label><br>
  <label>Group ID <input name="targetGroupId" placeholder="group-uuid"></label><br>
  <label>Payload JSON <textarea name="payload" rows="8" cols="60">{}</textarea></label><br>
  <button type="submit">Send</button>
</form>
</body>
</html>`))

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
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionAdminWrite) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := commandFormTemplate.Execute(w, nil); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		case http.MethodPost:
			if commandStore == nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			req, err := decodeCommandCreateRequest(r)
			if err != nil {
				if err == httpx.ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
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
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func decodeCommandCreateRequest(r *http.Request) (commandCreateRequest, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") || contentType == "" {
		var req commandCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return commandCreateRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return commandCreateRequest{}, err
	}
	var req commandCreateRequest
	req.Type = strings.TrimSpace(r.FormValue("type"))
	if raw := strings.TrimSpace(r.FormValue("payload")); raw != "" {
		if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&req.Payload); err != nil {
			return commandCreateRequest{}, httpx.ErrInvalidInput
		}
	}
	req.Target = commands.Target{
		Type:     strings.TrimSpace(r.FormValue("targetType")),
		DeviceID: strings.TrimSpace(r.FormValue("targetDeviceId")),
		GroupID:  strings.TrimSpace(r.FormValue("targetGroupId")),
	}
	if req.Target.Type == "" {
		req.Target.Type = commands.TargetBroadcast
	}
	return req, nil
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

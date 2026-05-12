package adminhttp

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/plugins"
)

const (
	csrfCookieName = "xmdm_csrf"
	csrfFieldName  = "csrfToken"
)

func Register(mux httpx.Router, svc *auth.Service, pluginManager *plugins.Manager, auditStore audit.Store, commandStore commands.Repository, tenantID string) {
	adminMux := httpx.WithPrefix(mux, "/admin")
	registerSessionRoutes(adminMux, svc)
	registerCommandRoutes(adminMux, svc, auditStore, commandStore, tenantID)
	registerAuditRoutes(adminMux, svc, auditStore, tenantID)
	if pluginManager != nil {
		pluginManager.Register(adminMux)
	}
}

type commandCreateRequest struct {
	Type      string          `json:"type"`
	Payload   map[string]any  `json:"payload,omitempty"`
	ExpiresAt string          `json:"expiresAt,omitempty"`
	Target    commands.Target `json:"target"`
}

func registerSessionRoutes(mux httpx.Router, svc *auth.Service) {
	loginPath := "/login"
	logoutPath := "/logout"
	mePath := "me"

	mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			token := issueCSRFCookie(w, r)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<form method="post"><input type="hidden" name="` + csrfFieldName + `" value="` + token + `"><input name="username"><input name="password" type="password"><button type="submit">Login</button></form>`))
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			if hasCSRFCookie(r) && !csrfTokenMatches(r) {
				http.Error(w, "forbidden", http.StatusForbidden)
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
		if hasCSRFCookie(r) && !csrfTokenMatches(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
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
		token := issueCSRFCookie(w, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":"` + session.Username + `","csrfToken":"` + token + `"}`))
	})
}

func registerCommandRoutes(mux httpx.Router, svc *auth.Service, auditStore audit.Store, commandStore commands.Repository, tenantID string) {
	mux.HandleFunc("/commands", func(w http.ResponseWriter, r *http.Request) {
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
			if commandStore == nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			items, err := commandStore.ListRecent(r.Context(), tenantID, 25)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"commands": items})
		case http.MethodPost:
			if !auth.HasPermission(session.Permissions, auth.PermissionAdminWrite) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !isJSONSubmission(r) && !csrfTokenMatches(r) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
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
			expiresAt, err := parseCommandExpiresAt(req.ExpiresAt)
			if err != nil {
				http.Error(w, "invalid input", http.StatusBadRequest)
				return
			}
			created, err := commandStore.Enqueue(r.Context(), tenantID, commands.Upsert{
				Type:      req.Type,
				Payload:   req.Payload,
				ExpiresAt: expiresAt,
				Target:    req.Target,
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

func hasCSRFCookie(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	return err == nil && strings.TrimSpace(cookie.Value) != ""
}

func isJSONSubmission(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, "application/json") || contentType == ""
}

func issueCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return cookie.Value
	}
	token, err := newCSRFCookieToken()
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

func csrfTokenMatches(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return false
	}
	token := strings.TrimSpace(r.FormValue(csrfFieldName))
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-XMDM-CSRF-Token"))
	}
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) == 1
}

func newCSRFCookieToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func registerAuditRoutes(mux httpx.Router, svc *auth.Service, auditStore audit.Store, tenantID string) {
	mux.HandleFunc("/audit", func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionAdminRead) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if auditStore == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		items, err := auditStore.List(r.Context(), tenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"events": items})
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
	req.ExpiresAt = strings.TrimSpace(r.FormValue("expiresAt"))
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

func parseCommandExpiresAt(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, httpx.ErrInvalidInput
		}
	}
	if !parsed.After(time.Now().UTC()) {
		return nil, httpx.ErrInvalidInput
	}
	return &parsed, nil
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

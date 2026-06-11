package adminhttp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"xmdm/server/internal/auth"
)

func (d *dashboard) login(w http.ResponseWriter, r *http.Request) {
	nextPath := safeAdminRedirectPath(r.URL.Query().Get("next"))
	switch r.Method {
	case http.MethodGet:
		token := issueCSRFCookie(w, r)
		d.render(w, pageData{
			Title:     "Login",
			CSRFToken: token,
			Forms: []formData{{
				Title:  "Enter the control plane",
				Action: "/admin/login",
				Fields: []fieldData{
					{Name: "next", Type: "hidden", Value: nextPath},
					{Name: "username", Label: "Username", Type: "text", Required: true},
					{Name: "password", Label: "Password", Type: "password", Required: true},
				},
				Submit: "Login",
			}},
		})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			d.renderError(w, http.StatusBadRequest, "Login", "invalid form")
			return
		}
		if hasCSRFCookie(r) && !csrfTokenMatches(r) {
			d.renderError(w, http.StatusForbidden, "Login", "forbidden")
			return
		}
		session, err := d.svc.Login(r.FormValue("username"), r.FormValue("password"))
		if err == nil {
			http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: session.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: session.ExpiresAt})
			if next := safeAdminRedirectPath(r.FormValue("next")); next != "" {
				http.Redirect(w, r, next, http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		if !errors.Is(err, auth.ErrInvalidCredentials) || d.deps.Identity == nil {
			d.renderError(w, http.StatusUnauthorized, "Login", "invalid credentials")
			return
		}
		user, role, userErr := d.deps.Identity.AuthenticateUser(r.Context(), d.deps.TenantID, strings.TrimSpace(r.FormValue("username")), r.FormValue("password"))
		if userErr != nil {
			d.renderError(w, http.StatusUnauthorized, "Login", "invalid credentials")
			return
		}
		session = d.svc.IssueSession(user.Email, dashboardPermissions(role.Permissions))
		http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: session.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: session.ExpiresAt})
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (d *dashboard) me(w http.ResponseWriter, r *http.Request) {
	session, ok := sessionFromRequest(r, d.svc)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"username":    session.Username,
		"permissions": session.Permissions,
		"csrfToken":   issueCSRFCookie(w, r),
	})
}

func (d *dashboard) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if hasCSRFCookie(r) && !csrfTokenMatches(r) {
		d.renderError(w, http.StatusForbidden, "Logout", "forbidden")
		return
	}
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		d.svc.Logout(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if wantsHTMLResponse(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func safeAdminRedirectPath(next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return ""
	}
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	if !strings.HasPrefix(next, "/admin/") && next != "/admin" {
		return ""
	}
	return next
}

const (
	csrfCookieName = "xmdm_csrf"
	csrfFieldName  = "csrfToken"
)

func hasCSRFCookie(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	return err == nil && strings.TrimSpace(cookie.Value) != ""
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

func wantsHTMLResponse(r *http.Request) bool {
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml") {
		return true
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Mode"), "navigate") {
		return true
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Dest"), "document") {
		return true
	}
	referer := strings.ToLower(r.Header.Get("Referer"))
	return strings.Contains(referer, "/admin")
}

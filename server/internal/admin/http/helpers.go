package adminhttp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/plugins"
)

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

func isAllowedCommandType(session *auth.Session, pluginManager *plugins.Manager, commandType string) bool {
	commandType = strings.TrimSpace(commandType)
	if commandType == "" {
		return false
	}
	if commands.IsBuiltinType(commandType) {
		return true
	}
	if pluginManager == nil || session == nil {
		return false
	}
	_, ok := pluginManager.SupportsCommandType(session, commandType)
	return ok
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

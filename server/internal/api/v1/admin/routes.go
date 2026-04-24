package adminv1

import (
	"net/http"

	"xmdm/server/internal/admin"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	devicehttp "xmdm/server/internal/device/http"
	grouphttp "xmdm/server/internal/group/http"
	"xmdm/server/internal/httpx"
	identityhttp "xmdm/server/internal/identity/http"
	policyhttp "xmdm/server/internal/policy/http"
)

func Register(mux httpx.Router, svc *auth.Service, store admin.Repository, auditStore audit.Store, tenantID string) {
	registerSessionRoutes(mux, svc)
	identityhttp.Register(mux, svc, store, auditStore, tenantID)
	grouphttp.Register(mux, svc, store, auditStore, tenantID)
	policyhttp.Register(mux, svc, store, auditStore, tenantID)
	devicehttp.Register(mux, svc, store, auditStore, tenantID)
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

package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/pagination"
)

const requestPaginationMaxLimit = 100

func RequestPaginationParams(r *http.Request, maxLimit int) pagination.Params {
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	limit := pagination.DefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	params := pagination.Normalize(pagination.Params{Limit: limit}, pagination.DefaultLimit, maxLimit)
	params.Offset = pagination.Offset(page, params.Limit)
	return params
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

func DecodeJSONBody[T any](r *http.Request, dst *T) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

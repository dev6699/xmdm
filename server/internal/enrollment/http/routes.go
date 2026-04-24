package enrollmenthttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
)

type QRRequest struct {
	ServerURL                  string               `json:"serverUrl"`
	ServerProject              string               `json:"serverProject"`
	EnrollmentToken            string               `json:"enrollmentToken"`
	DeviceAdminComponentName   string               `json:"deviceAdminComponentName"`
	DeviceAdminPackageURL      string               `json:"deviceAdminPackageDownloadLocation"`
	DeviceAdminPackageChecksum string               `json:"deviceAdminPackageChecksum"`
	LeaveAllSystemAppsEnabled  bool                 `json:"leaveAllSystemAppsEnabled"`
	SkipEncryption             bool                 `json:"skipEncryption"`
	UseMobileData              bool                 `json:"useMobileData"`
	DeviceIdentityPolicy       DeviceIdentityPolicy `json:"deviceIdentityPolicy"`
	BootstrapExtras            map[string]any       `json:"bootstrapExtras"`
}

type DeviceIdentityPolicy struct {
	DeviceID    string `json:"deviceId,omitempty"`
	DeviceIDUse string `json:"deviceIdUse"`
}

type AndroidQRPayload struct {
	DeviceAdminComponentName  string         `json:"android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME"`
	PackageDownloadLocation   string         `json:"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION"`
	PackageChecksum           string         `json:"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM"`
	LeaveAllSystemAppsEnabled bool           `json:"android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED"`
	SkipEncryption            bool           `json:"android.app.extra.PROVISIONING_SKIP_ENCRYPTION,omitempty"`
	UseMobileData             bool           `json:"android.app.extra.PROVISIONING_USE_MOBILE_DATA,omitempty"`
	AdminExtrasBundle         map[string]any `json:"android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE"`
}

type TokenIssueRequest struct {
	TTLSeconds int `json:"ttlSeconds"`
}

type TokenLookupRequest struct {
	Token string `json:"token"`
}

type EnrollmentRequest struct {
	EnrollmentToken      string               `json:"enrollmentToken"`
	DeviceIdentityPolicy DeviceIdentityPolicy `json:"deviceIdentityPolicy"`
	BootstrapExtras      map[string]any       `json:"bootstrapExtras"`
}

func Register(mux httpx.Router, svc *auth.Service, store enrollment.Repository, tenantID string) {
	mux.HandleFunc("", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		req, err := decodeEnrollmentRequest(r)
		if err != nil {
			writeRequestError(w, err)
			return
		}
		bound, err := store.BindDevice(r.Context(), tenantID, req.EnrollmentToken, req.DeviceIdentityPolicy.DeviceID)
		if err != nil {
			writeEnrollmentError(w, err)
			return
		}
		writeJSON(w, bound)
	})

	mux.HandleFunc("/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionDevicesWrite) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		req := TokenIssueRequest{TTLSeconds: 24 * 60 * 60}
		if err := decodeTokenIssueRequest(r, &req); err != nil {
			writeRequestError(w, err)
			return
		}
		if req.TTLSeconds <= 0 {
			writeRequestError(w, httpx.ErrInvalidInput)
			return
		}

		issued, err := store.IssueToken(r.Context(), tenantID, time.Now().Add(time.Duration(req.TTLSeconds)*time.Second))
		if err != nil {
			writeEnrollmentError(w, err)
			return
		}

		writeJSON(w, issued)
	})

	mux.HandleFunc("/tokens/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req, err := decodeTokenLookupRequest(r)
		if err != nil {
			writeRequestError(w, err)
			return
		}
		token, err := store.ValidateToken(r.Context(), tenantID, req.Token)
		if err != nil {
			writeEnrollmentError(w, err)
			return
		}
		writeJSON(w, token)
	})

	mux.HandleFunc("/tokens/consume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req, err := decodeTokenLookupRequest(r)
		if err != nil {
			writeRequestError(w, err)
			return
		}
		token, err := store.ConsumeToken(r.Context(), tenantID, req.Token)
		if err != nil {
			writeEnrollmentError(w, err)
			return
		}
		writeJSON(w, token)
	})

	mux.HandleFunc("/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionDevicesWrite) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		tokenID := r.PathValue("id")
		if tokenID == "" {
			writeRequestError(w, httpx.ErrInvalidInput)
			return
		}
		token, err := store.RevokeToken(r.Context(), tenantID, tokenID)
		if err != nil {
			writeEnrollmentError(w, err)
			return
		}
		writeJSON(w, token)
	})

	mux.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionDevicesWrite) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		payload, err := decodeQRRequest(r)
		if err != nil {
			writeRequestError(w, err)
			return
		}

		raw, err := json.Marshal(toPayload(payload))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		png, err := qrcode.Encode(string(raw), qrcode.Medium, 256)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})

	mux.HandleFunc("/qr/json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionDevicesWrite) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		payload, err := decodeQRRequest(r)
		if err != nil {
			writeRequestError(w, err)
			return
		}

		writeJSON(w, toPayload(payload))
	})
}

func decodeTokenIssueRequest(r *http.Request, dst *TokenIssueRequest) error {
	if err := httpx.DecodeJSONBody(r, dst); err != nil {
		return err
	}
	return nil
}

func decodeTokenLookupRequest(r *http.Request) (TokenLookupRequest, error) {
	var payload TokenLookupRequest
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return TokenLookupRequest{}, err
	}
	payload.Token = strings.TrimSpace(payload.Token)
	if payload.Token == "" {
		return TokenLookupRequest{}, httpx.ErrInvalidInput
	}
	return payload, nil
}

func decodeEnrollmentRequest(r *http.Request) (EnrollmentRequest, error) {
	var payload EnrollmentRequest
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return EnrollmentRequest{}, err
	}
	payload.EnrollmentToken = strings.TrimSpace(payload.EnrollmentToken)
	payload.DeviceIdentityPolicy.DeviceID = strings.TrimSpace(payload.DeviceIdentityPolicy.DeviceID)
	payload.DeviceIdentityPolicy.DeviceIDUse = strings.TrimSpace(payload.DeviceIdentityPolicy.DeviceIDUse)
	if payload.EnrollmentToken == "" || payload.DeviceIdentityPolicy.DeviceID == "" {
		return EnrollmentRequest{}, httpx.ErrInvalidInput
	}
	return payload, nil
}

func toPayload(req QRRequest) AndroidQRPayload {
	return AndroidQRPayload{
		DeviceAdminComponentName:  defaultComponentName(req.DeviceAdminComponentName),
		PackageDownloadLocation:   req.DeviceAdminPackageURL,
		PackageChecksum:           req.DeviceAdminPackageChecksum,
		LeaveAllSystemAppsEnabled: req.LeaveAllSystemAppsEnabled,
		SkipEncryption:            req.SkipEncryption,
		UseMobileData:             req.UseMobileData,
		AdminExtrasBundle:         buildAdminExtrasBundle(req),
	}
}

func decodeQRRequest(r *http.Request) (QRRequest, error) {
	var payload QRRequest
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return QRRequest{}, err
	}

	payload.ServerURL = strings.TrimSpace(payload.ServerURL)
	payload.ServerProject = strings.TrimSpace(payload.ServerProject)
	payload.EnrollmentToken = strings.TrimSpace(payload.EnrollmentToken)
	payload.DeviceAdminComponentName = strings.TrimSpace(payload.DeviceAdminComponentName)
	payload.DeviceAdminPackageURL = strings.TrimSpace(payload.DeviceAdminPackageURL)
	payload.DeviceAdminPackageChecksum = strings.TrimSpace(payload.DeviceAdminPackageChecksum)
	payload.DeviceIdentityPolicy.DeviceID = strings.TrimSpace(payload.DeviceIdentityPolicy.DeviceID)
	payload.DeviceIdentityPolicy.DeviceIDUse = strings.TrimSpace(payload.DeviceIdentityPolicy.DeviceIDUse)

	if payload.ServerURL == "" || payload.DeviceAdminPackageURL == "" || payload.DeviceAdminPackageChecksum == "" {
		return QRRequest{}, httpx.ErrInvalidInput
	}
	if payload.DeviceIdentityPolicy.DeviceID == "" && payload.DeviceIdentityPolicy.DeviceIDUse == "" {
		return QRRequest{}, httpx.ErrInvalidInput
	}
	parsedURL, err := parseServerURL(payload.ServerURL)
	if err != nil {
		return QRRequest{}, httpx.ErrInvalidInput
	}
	payload.ServerURL = parsedURL.String()
	if payload.DeviceAdminComponentName == "" {
		payload.DeviceAdminComponentName = "com.xmdm.launcher/.AdminReceiver"
	}
	if payload.BootstrapExtras == nil {
		payload.BootstrapExtras = map[string]any{}
	}
	return payload, nil
}

func buildAdminExtrasBundle(req QRRequest) map[string]any {
	bundle := map[string]any{
		"com.xmdm.BASE_URL": req.ServerURL,
	}
	if req.ServerProject != "" {
		bundle["com.xmdm.SERVER_PROJECT"] = req.ServerProject
	}
	if req.EnrollmentToken != "" {
		bundle["com.xmdm.ENROLLMENT_TOKEN"] = req.EnrollmentToken
	}
	if req.DeviceIdentityPolicy.DeviceID != "" {
		bundle["com.xmdm.DEVICE_ID"] = req.DeviceIdentityPolicy.DeviceID
	}
	if req.DeviceIdentityPolicy.DeviceIDUse != "" {
		bundle["com.xmdm.DEVICE_ID_USE"] = req.DeviceIdentityPolicy.DeviceIDUse
	}
	for key, value := range req.BootstrapExtras {
		switch key {
		case "customer":
			putString(bundle, "com.xmdm.CUSTOMER", value)
		case "configuration", "config":
			putString(bundle, "com.xmdm.CONFIG", value)
		case "groups":
			putGroups(bundle, value)
		case "certs":
			putString(bundle, "com.xmdm.CERTS", value)
		case "secondaryBaseUrl":
			putString(bundle, "com.xmdm.SECONDARY_BASE_URL", value)
		default:
			bundle[key] = value
		}
	}
	return bundle
}

func putString(dst map[string]any, key string, value any) {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			dst[key] = v
		}
	case []any:
		if len(v) > 0 {
			parts := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					parts = append(parts, strings.TrimSpace(s))
				}
			}
			if len(parts) > 0 {
				dst[key] = strings.Join(parts, ",")
			}
		}
	}
}

func putGroups(dst map[string]any, value any) {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			dst["com.xmdm.GROUP"] = strings.TrimSpace(v)
		}
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		if len(parts) > 0 {
			dst["com.xmdm.GROUP"] = strings.Join(parts, ",")
		}
	}
}

func defaultComponentName(value string) string {
	if value != "" {
		return value
	}
	return "com.xmdm.launcher/.AdminReceiver"
}

func parseServerURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, httpx.ErrInvalidInput
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
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

func writeRequestError(w http.ResponseWriter, err error) {
	if err == httpx.ErrInvalidInput {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}
	http.Error(w, "invalid json", http.StatusBadRequest)
}

func writeEnrollmentError(w http.ResponseWriter, err error) {
	switch err {
	case httpx.ErrInvalidInput:
		http.Error(w, "invalid input", http.StatusBadRequest)
	case httpx.ErrNotFound, enrollment.ErrTokenNotFound:
		http.Error(w, "not found", http.StatusNotFound)
	case enrollment.ErrTokenConsumed, enrollment.ErrTokenExpired, enrollment.ErrTokenRevoked, enrollment.ErrTokenConflict:
		http.Error(w, err.Error(), http.StatusConflict)
	case enrollment.ErrDeviceConflict:
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	_ = enc.Encode(value)
	_, _ = w.Write(bytes.TrimSpace(buf.Bytes()))
}

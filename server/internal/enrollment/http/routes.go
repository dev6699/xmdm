package enrollmenthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/managedfiles"
	"xmdm/server/internal/policy"

	qrcode "github.com/skip2/go-qrcode"
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

func Register(mux httpx.Router, svc *auth.Service, devices device.Repository, store enrollment.Repository, appStore apps.Repository, managedFileStore managedfiles.Repository, artifactStore artifacts.Store, certStore certificates.Repository, policyStore policy.Repository, runtime enrollment.RuntimeSnapshot, tenantID string) {
	enrollmentMux := httpx.WithPrefix(mux, "/enrollment")

	enrollmentMux.HandleFunc("", func(w http.ResponseWriter, r *http.Request) {
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
		bound, err := store.BindDevice(r.Context(), tenantID, req.EnrollmentToken, req.DeviceIdentityPolicy.DeviceID, req.BootstrapExtras)
		if err != nil {
			writeEnrollmentError(w, err)
			return
		}
		writeJSON(w, bound)
	})

	mux.HandleFunc("/devices/{deviceId}/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if devices == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		deviceID := strings.TrimSpace(r.PathValue("deviceId"))
		secret := strings.TrimSpace(r.Header.Get("X-XMDM-Device-Secret"))
		if deviceID == "" || secret == "" {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		authDevice, err := devices.Authenticate(r.Context(), tenantID, deviceID, secret)
		if err != nil {
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
		deviceIdentity := strings.TrimSpace(authDevice.ID)
		if deviceIdentity == "" {
			deviceIdentity = authDevice.Name
		}
		config, err := buildSignedConfigSnapshot(r.Context(), policyStore, appStore, managedFileStore, artifactStore, certStore, tenantID, deviceIdentity, authDevice.PolicyID, authDevice.BootstrapExtras, runtime, secret)
		if err != nil {
			switch err {
			case httpx.ErrNotFound:
				http.Error(w, "not found", http.StatusNotFound)
			default:
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, config)
	})

	enrollmentMux.HandleFunc("/tokens", func(w http.ResponseWriter, r *http.Request) {
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

	enrollmentMux.HandleFunc("/tokens/validate", func(w http.ResponseWriter, r *http.Request) {
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

	enrollmentMux.HandleFunc("/tokens/consume", func(w http.ResponseWriter, r *http.Request) {
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

	enrollmentMux.HandleFunc("/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
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

	enrollmentMux.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
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

	enrollmentMux.HandleFunc("/qr/json", func(w http.ResponseWriter, r *http.Request) {
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

func BuildConfigSnapshot(ctx context.Context, policyStore policy.Repository, appStore apps.Repository, managedFileStore managedfiles.Repository, artifactStore artifacts.Store, certStore certificates.Repository, tenantID, deviceID string, policyID *string, bootstrapExtras map[string]any, runtime enrollment.RuntimeSnapshot) (enrollment.ConfigSnapshot, error) {
	appsSnapshot, err := listPublishedPolicyApps(ctx, policyStore, appStore, deviceID, tenantID, policyID)
	if err != nil {
		return enrollment.ConfigSnapshot{}, err
	}
	deviceIDUse := managedfiles.TemplateValues(deviceID, bootstrapExtras)["DEVICE_ID_USE"]
	filesSnapshot, err := listPolicyManagedFiles(ctx, policyStore, managedFileStore, artifactStore, deviceID, tenantID, policyID, bootstrapExtras)
	if err != nil {
		return enrollment.ConfigSnapshot{}, err
	}
	certs, err := listPolicyCertificates(ctx, policyStore, certStore, tenantID, deviceID, policyID)
	if err != nil {
		return enrollment.ConfigSnapshot{}, err
	}
	policySnapshot, err := devicePolicySnapshot(ctx, policyStore, tenantID, policyID)
	if err != nil {
		return enrollment.ConfigSnapshot{}, err
	}
	config := enrollment.NewBootstrapConfigSnapshot(deviceID, deviceIDUse, runtime, policySnapshot, appsSnapshot, filesSnapshot, certs)
	return config, nil
}

func buildSignedConfigSnapshot(ctx context.Context, policyStore policy.Repository, appStore apps.Repository, managedFileStore managedfiles.Repository, artifactStore artifacts.Store, certStore certificates.Repository, tenantID, deviceID string, policyID *string, bootstrapExtras map[string]any, runtime enrollment.RuntimeSnapshot, secret string) (enrollment.ConfigSnapshot, error) {
	config, err := BuildConfigSnapshot(ctx, policyStore, appStore, managedFileStore, artifactStore, certStore, tenantID, deviceID, policyID, bootstrapExtras, runtime)
	if err != nil {
		return enrollment.ConfigSnapshot{}, err
	}
	return enrollment.SignConfigSnapshot(config, secret)
}

func devicePolicySnapshot(ctx context.Context, store policy.Repository, tenantID string, policyID *string) (enrollment.PolicySnapshot, error) {
	if policyID == nil || strings.TrimSpace(*policyID) == "" {
		return enrollment.PolicySnapshot{}, httpx.ErrNotFound
	}
	rec, err := store.GetPolicy(ctx, tenantID, strings.TrimSpace(*policyID))
	if err != nil {
		return enrollment.PolicySnapshot{}, err
	}
	restrictions, err := policySnapshotRestrictions(rec.Restrictions)
	if err != nil {
		return enrollment.PolicySnapshot{}, err
	}
	snapshot := enrollment.PolicySnapshot{
		Name:            rec.Name,
		Version:         rec.Version,
		KioskMode:       rec.KioskMode,
		KioskAppPackage: rec.KioskAppPackage,
		Restrictions:    restrictions,
	}
	return snapshot, nil
}

type policySnapshotRestrictionsPayload struct {
	AllowPackages                []string `json:"allowPackages,omitempty"`
	BlockPackages                []string `json:"blockPackages,omitempty"`
	SuspendPackages              []string `json:"suspendPackages,omitempty"`
	KioskKeepScreenOn            bool     `json:"kioskKeepScreenOn,omitempty"`
	KioskStayAwakeWhilePluggedIn bool     `json:"kioskStayAwakeWhilePluggedIn,omitempty"`
	KioskUnlockOnBoot            bool     `json:"kioskUnlockOnBoot,omitempty"`
	KioskExitPasscode            string   `json:"kioskExitPasscode,omitempty"`
	KioskExitPasscodeHash        string   `json:"kioskExitPasscodeHash,omitempty"`
}

func policySnapshotRestrictions(raw json.RawMessage) (enrollment.PolicyRestrictions, error) {
	var parsed policySnapshotRestrictionsPayload
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return enrollment.PolicyRestrictions{}, err
		}
	}
	passcodeHash := strings.TrimSpace(parsed.KioskExitPasscodeHash)
	if passcodeHash == "" {
		passcode := strings.TrimSpace(parsed.KioskExitPasscode)
		if passcode != "" {
			passcodeHash = enrollment.HashToken(passcode)
		}
	}
	return enrollment.PolicyRestrictions{
		AllowPackages:                parsed.AllowPackages,
		BlockPackages:                parsed.BlockPackages,
		SuspendPackages:              parsed.SuspendPackages,
		KioskKeepScreenOn:            parsed.KioskKeepScreenOn,
		KioskStayAwakeWhilePluggedIn: parsed.KioskStayAwakeWhilePluggedIn,
		KioskUnlockOnBoot:            parsed.KioskUnlockOnBoot,
		KioskExitPasscodeHash:        passcodeHash,
	}, nil
}

func listPolicyManagedFiles(ctx context.Context, policyStore policy.Repository, store managedfiles.Repository, artifactStore artifacts.Store, deviceID, tenantID string, policyID *string, bootstrapExtras map[string]any) ([]enrollment.ManagedFileSnapshot, error) {
	if policyStore == nil || store == nil || policyID == nil || strings.TrimSpace(*policyID) == "" {
		return []enrollment.ManagedFileSnapshot{}, nil
	}
	assignments, err := policyStore.ListPolicyManagedFiles(ctx, tenantID, strings.TrimSpace(*policyID))
	if err != nil {
		return nil, err
	}
	assigned := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if assignment.Status != policy.StatusActive {
			continue
		}
		assigned[assignment.ManagedFileID] = struct{}{}
	}
	if len(assigned) == 0 {
		return []enrollment.ManagedFileSnapshot{}, nil
	}
	files, err := store.ListManagedFiles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	snapshots := make([]enrollment.ManagedFileSnapshot, 0, len(files))
	for _, file := range files {
		if _, ok := assigned[file.ID]; !ok {
			continue
		}
		if file.Status != managedfiles.StatusActive {
			continue
		}
		if file.File == nil || file.File.Artifact == nil {
			continue
		}
		name := file.File.Name
		if name == "" {
			name = file.ID
		}
		snapshot := enrollment.ManagedFileSnapshot{
			FileID:           file.ID,
			Name:             name,
			Path:             file.Path,
			DownloadPath:     managedFileDownloadPath(deviceID, file.ID),
			Checksum:         file.File.Checksum,
			MimeType:         file.File.MimeType,
			ReplaceVariables: file.ReplaceVariables,
		}
		if file.ReplaceVariables {
			rendered, err := renderedManagedFileContent(ctx, artifactStore, file, deviceID, bootstrapExtras)
			if err != nil {
				return nil, err
			}
			snapshot.Checksum = checksum.SHA256Base64URL(rendered)
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func managedFileDownloadPath(deviceID, managedFileID string) string {
	return "/api/v1/devices/" + deviceID + "/managed-files/" + managedFileID + "/artifact"
}

func renderedManagedFileContent(ctx context.Context, artifactStore artifacts.Store, file managedfiles.ManagedFile, deviceID string, bootstrapExtras map[string]any) ([]byte, error) {
	if artifactStore == nil || file.File == nil || file.File.Artifact == nil {
		return nil, httpx.ErrNotFound
	}
	body, err := artifactStore.Get(ctx, file.File.Artifact.StorageKey)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return managedfiles.RenderTemplate(content, deviceID, bootstrapExtras), nil
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

func listPolicyCertificates(ctx context.Context, policyStore policy.Repository, certStore certificates.Repository, tenantID, deviceID string, policyID *string) ([]enrollment.CertificateSnapshot, error) {
	if policyStore == nil || certStore == nil || policyID == nil || strings.TrimSpace(*policyID) == "" {
		return []enrollment.CertificateSnapshot{}, nil
	}
	assignments, err := policyStore.ListPolicyCertificates(ctx, tenantID, strings.TrimSpace(*policyID))
	if err != nil {
		return nil, err
	}
	assigned := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if assignment.Status != policy.StatusActive {
			continue
		}
		assigned[assignment.CertificateID] = struct{}{}
	}
	if len(assigned) == 0 {
		return []enrollment.CertificateSnapshot{}, nil
	}
	items, err := certStore.ListActiveCertificates(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]enrollment.CertificateSnapshot, 0, len(items))
	for _, item := range items {
		if _, ok := assigned[item.ID]; !ok {
			continue
		}
		out = append(out, enrollment.CertificateSnapshot{
			ID:           item.ID,
			Name:         item.Name,
			ArtifactID:   item.ArtifactID,
			Checksum:     item.Checksum,
			DownloadPath: certificateDownloadPath(deviceID, item.ID),
		})
	}
	return out, nil
}

func certificateDownloadPath(deviceID, certificateID string) string {
	return "/api/v1/devices/" + deviceID + "/certificates/" + certificateID + "/artifact"
}

func listPublishedPolicyApps(ctx context.Context, policyStore policy.Repository, appStore apps.Repository, deviceID, tenantID string, policyID *string) ([]enrollment.AppSnapshot, error) {
	if policyStore == nil || appStore == nil || policyID == nil || strings.TrimSpace(*policyID) == "" {
		return []enrollment.AppSnapshot{}, nil
	}
	assignments, err := policyStore.ListPolicyApps(ctx, tenantID, strings.TrimSpace(*policyID))
	if err != nil {
		return nil, err
	}
	assigned := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if assignment.Status != policy.StatusActive {
			continue
		}
		assigned[assignment.AppID] = struct{}{}
	}
	if len(assigned) == 0 {
		return []enrollment.AppSnapshot{}, nil
	}
	items, err := appStore.ListApps(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]enrollment.AppSnapshot, 0)
	for _, appRecord := range items {
		if appRecord.Status != apps.StatusActive {
			continue
		}
		if _, ok := assigned[appRecord.ID]; !ok {
			continue
		}
		versions, err := appStore.ListVersions(ctx, tenantID, appRecord.ID)
		if err != nil {
			return nil, err
		}
		var published *apps.Version
		for _, version := range versions {
			if version.Status != apps.VersionStatusPublished || version.ArtifactID == nil {
				continue
			}
			versionCopy := version
			published = &versionCopy
		}
		if published == nil {
			continue
		}
		out = append(out, enrollment.AppSnapshot{
			AppID:        appRecord.ID,
			PackageName:  appRecord.PackageName,
			Name:         appRecord.Name,
			VersionID:    published.ID,
			VersionName:  published.VersionName,
			VersionCode:  published.VersionCode,
			ArtifactID:   *published.ArtifactID,
			Checksum:     published.Checksum,
			DownloadPath: "/api/v1/devices/" + deviceID + "/apps/" + appRecord.ID + "/versions/" + published.ID + "/artifact",
		})
	}
	return out, nil
}

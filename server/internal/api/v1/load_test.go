package v1

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	adminhttp "xmdm/server/internal/admin/http"
	"xmdm/server/internal/apps"
	apphttp "xmdm/server/internal/apps/http"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	certificatehttp "xmdm/server/internal/certificates/http"
	"xmdm/server/internal/commands"
	commandhttp "xmdm/server/internal/commands/http"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	enrollmenthttp "xmdm/server/internal/enrollment/http"
	files "xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/managedfiles"
	managedfilehttp "xmdm/server/internal/managedfiles/http"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/plugins"
	"xmdm/server/internal/policy"
	"xmdm/server/internal/telemetry"
	telemetryhttp "xmdm/server/internal/telemetry/http"
)

const (
	loadTenantID          = "tenant-load"
	loadDeviceID          = "device-load"
	loadDeviceSecret      = "device-secret"
	loadAdminUsername     = "admin"
	loadAdminPassword     = "secret"
	loadAppID             = "app-load"
	loadAppVersionID      = "version-load"
	loadCertificateID     = "certificate-load"
	loadManagedFileID     = "managed-file-load"
	loadAppStorageKey     = "artifacts/app-load.apk"
	loadCertStorageKey    = "artifacts/cert-load.pem"
	loadManagedStorageKey = "artifacts/managed-load.txt"
	loadCommandRequests   = 32
	loadPollRequests      = 32
	loadAckRequests       = 32
	loadTelemetryRequests = 32
	loadConfigRequests    = 32
	loadDownloadRequests  = 32
	loadWorkers           = 8
	loadCSRFToken         = "load-csrf-token"
)

func TestLoadRoutes(t *testing.T) {
	fixture := newLoadFixture(t)
	t.Run("sync", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadConfigRequests, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/config", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	t.Run("push", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadCommandRequests, func() *http.Request {
			return fixture.newRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("type=reboot&targetType=device&targetDeviceId="+loadDeviceID), fixture.withSession(), fixture.withCSRF(), fixture.withForm())
		}, http.StatusOK)
	})
	t.Run("poll", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadPollRequests, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/commands", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	t.Run("ack", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadAckRequests, func() *http.Request {
			return fixture.newRequest(http.MethodPost, "/api/v1/devices/"+loadDeviceID+"/commands/cmd-load/ack", bytes.NewReader([]byte(`{"status":"acked","message":"done"}`)), fixture.withDeviceSecret(), fixture.withJSON())
		}, http.StatusOK)
	})
	t.Run("app-download", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadDownloadRequests, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/apps/"+loadAppID+"/versions/"+loadAppVersionID+"/artifact", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	t.Run("managed-file-download", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadDownloadRequests, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/managed-files/"+loadManagedFileID+"/artifact", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	t.Run("certificate-download", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadDownloadRequests, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/certificates/"+loadCertificateID+"/artifact", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	t.Run("telemetry", func(t *testing.T) {
		runLoadCase(t, fixture.client, loadWorkers, loadTelemetryRequests, func() *http.Request {
			return fixture.newRequest(http.MethodPost, "/api/v1/devices/"+loadDeviceID+"/telemetry", bytes.NewReader([]byte(`{"heartbeat":{"uptimeSeconds":123}}`)), fixture.withDeviceSecret(), fixture.withJSON())
		}, http.StatusOK)
	})
	t.Run("mixed", func(t *testing.T) {
		mixedFixture := newLoadFixture(t)
		builders := []func() *http.Request{
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/config", nil, mixedFixture.withDeviceSecret())
			},
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("type=reboot&targetType=device&targetDeviceId="+loadDeviceID), mixedFixture.withSession(), mixedFixture.withCSRF(), mixedFixture.withForm())
			},
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/commands", nil, mixedFixture.withDeviceSecret())
			},
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodPost, "/api/v1/devices/"+loadDeviceID+"/commands/cmd-load/ack", bytes.NewReader([]byte(`{"status":"acked","message":"done"}`)), mixedFixture.withDeviceSecret(), mixedFixture.withJSON())
			},
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/apps/"+loadAppID+"/versions/"+loadAppVersionID+"/artifact", nil, mixedFixture.withDeviceSecret())
			},
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/managed-files/"+loadManagedFileID+"/artifact", nil, mixedFixture.withDeviceSecret())
			},
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/certificates/"+loadCertificateID+"/artifact", nil, mixedFixture.withDeviceSecret())
			},
			func() *http.Request {
				return mixedFixture.newRequest(http.MethodPost, "/api/v1/devices/"+loadDeviceID+"/telemetry", bytes.NewReader([]byte(`{"heartbeat":{"uptimeSeconds":123}}`)), mixedFixture.withDeviceSecret(), mixedFixture.withJSON())
			},
		}
		runMixedLoadCase(t, mixedFixture.client, loadWorkers, loadConfigRequests, builders, http.StatusOK)
	})

	if got := fixture.commands.enqueued(); got != loadCommandRequests {
		t.Fatalf("unexpected enqueued commands: got %d want %d", got, loadCommandRequests)
	}
	if got := fixture.commands.acked(); got != loadAckRequests {
		t.Fatalf("unexpected ack count: got %d want %d", got, loadAckRequests)
	}
	if got := fixture.devices.authenticated(); got != loadConfigRequests+loadPollRequests+loadAckRequests+3*loadDownloadRequests {
		t.Fatalf("unexpected authenticate count: got %d want %d", got, loadConfigRequests+loadPollRequests+loadAckRequests+3*loadDownloadRequests)
	}
	if got := fixture.artifacts.gets(); got != 3*loadDownloadRequests {
		t.Fatalf("unexpected artifact fetch count: got %d want %d", got, 3*loadDownloadRequests)
	}
	if got := fixture.telemetry.uploads(); got != loadTelemetryRequests {
		t.Fatalf("unexpected telemetry uploads: got %d want %d", got, loadTelemetryRequests)
	}
}

func BenchmarkLoadRoutes(b *testing.B) {
	fixture := newLoadFixture(b)
	b.Run("sync", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/config", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	b.Run("push", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("type=reboot&targetType=device&targetDeviceId="+loadDeviceID), fixture.withSession(), fixture.withCSRF(), fixture.withForm())
		}, http.StatusOK)
	})
	b.Run("poll", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/commands", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	b.Run("ack", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodPost, "/api/v1/devices/"+loadDeviceID+"/commands/cmd-load/ack", bytes.NewReader([]byte(`{"status":"acked","message":"done"}`)), fixture.withDeviceSecret(), fixture.withJSON())
		}, http.StatusOK)
	})
	b.Run("app-download", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/apps/"+loadAppID+"/versions/"+loadAppVersionID+"/artifact", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	b.Run("managed-file-download", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/managed-files/"+loadManagedFileID+"/artifact", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	b.Run("certificate-download", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodGet, "/api/v1/devices/"+loadDeviceID+"/certificates/"+loadCertificateID+"/artifact", nil, fixture.withDeviceSecret())
		}, http.StatusOK)
	})
	b.Run("telemetry", func(b *testing.B) {
		b.ResetTimer()
		runLoadCase(b, fixture.client, loadWorkers, b.N, func() *http.Request {
			return fixture.newRequest(http.MethodPost, "/api/v1/devices/"+loadDeviceID+"/telemetry", bytes.NewReader([]byte(`{"heartbeat":{"uptimeSeconds":123}}`)), fixture.withDeviceSecret(), fixture.withJSON())
		}, http.StatusOK)
	})
}

type loadFixture struct {
	client        *http.Client
	baseURL       string
	sessionCookie *http.Cookie
	deviceSecret  string
	devices       *loadDeviceStore
	apps          *loadAppStore
	certificates  *loadCertificateStore
	managedFiles  *loadManagedFileStore
	artifacts     *loadArtifactStore
	commands      *loadCommandStore
	telemetry     *loadTelemetryStore
}

func newLoadFixture(tb testing.TB) *loadFixture {
	tb.Helper()

	artifactContent := []byte("artifact-bytes-for-load-tests")
	appArtifact := &files.Artifact{
		RecordBase: files.RecordBase{
			ID:       "artifact-app-load",
			TenantID: loadTenantID,
			Status:   files.StatusActive,
		},
		StorageKey: loadAppStorageKey,
		Checksum:   "checksum-app-load",
		SizeBytes:  int64(len(artifactContent)),
		MimeType:   "application/vnd.android.package-archive",
	}
	certArtifact := &files.Artifact{
		RecordBase: files.RecordBase{
			ID:       "artifact-cert-load",
			TenantID: loadTenantID,
			Status:   files.StatusActive,
		},
		StorageKey: loadCertStorageKey,
		Checksum:   "checksum-cert-load",
		SizeBytes:  int64(len(artifactContent)),
		MimeType:   "application/x-pem-file",
	}
	versionArtifactID := "artifact-app-load"

	devices := &loadDeviceStore{
		expectedSecret: loadDeviceSecret,
	}
	policyID := "policy-load"
	devices.device = device.Device{
		RecordBase: device.RecordBase{
			ID:       loadDeviceID,
			TenantID: loadTenantID,
			Status:   device.StatusEnrolled,
		},
		Name:            loadDeviceID,
		PolicyID:        &policyID,
		BootstrapExtras: map[string]any{},
	}
	appsStore := &loadAppStore{
		apps: []apps.App{
			{
				RecordBase: apps.RecordBase{
					ID:       loadAppID,
					TenantID: loadTenantID,
					Status:   apps.StatusActive,
				},
				PackageName: "com.example.loadapp",
				Name:        "Load App",
			},
		},
		versions: map[string][]apps.Version{
			loadAppID: {
				{
					ID:          loadAppVersionID,
					TenantID:    loadTenantID,
					AppID:       loadAppID,
					Status:      apps.VersionStatusPublished,
					VersionName: "1.0.0",
					VersionCode: 100,
					ArtifactID:  &versionArtifactID,
					Artifact:    appArtifact,
					Checksum:    "checksum-app-load",
					CreatedAt:   time.Now(),
				},
			},
		},
	}
	certsStore := &loadCertificateStore{
		certificates: []certificates.Certificate{
			{
				RecordBase: certificates.RecordBase{
					ID:       loadCertificateID,
					TenantID: loadTenantID,
					Status:   certificates.StatusActive,
				},
				Name:       "load-cert",
				ArtifactID: "artifact-cert-load",
				Checksum:   "checksum-cert-load",
				Artifact:   certArtifact,
			},
		},
	}
	managedFilesStore := &loadManagedFileStore{
		items: []managedfiles.ManagedFile{
			{
				RecordBase: managedfiles.RecordBase{
					ID:       loadManagedFileID,
					TenantID: loadTenantID,
					Status:   managedfiles.StatusActive,
				},
				FileID: loadManagedFileID,
				Path:   "device-config.txt",
				File: &files.File{
					RecordBase: files.RecordBase{
						ID:       loadManagedFileID,
						TenantID: loadTenantID,
						Status:   files.StatusActive,
					},
					Name:       "device-config.txt",
					ArtifactID: "artifact-managed-load",
					Checksum:   "checksum-managed-load",
					MimeType:   "text/plain",
					Artifact: &files.Artifact{
						RecordBase: files.RecordBase{
							ID:       "artifact-managed-load",
							TenantID: loadTenantID,
							Status:   files.StatusActive,
						},
						StorageKey: loadManagedStorageKey,
						Checksum:   "checksum-managed-load",
						SizeBytes:  int64(len(artifactContent)),
						MimeType:   "text/plain",
					},
				},
				ReplaceVariables: true,
			},
		},
	}
	artifactStore := &loadArtifactStore{
		artifacts: map[string][]byte{
			loadAppStorageKey:     artifactContent,
			loadCertStorageKey:    artifactContent,
			loadManagedStorageKey: []byte("device-config-bytes {{DEVICE_NUMBER}}"),
		},
	}
	commandStore := &loadCommandStore{}
	telemetryStore := &loadTelemetryStore{}
	policyStore := &loadPolicyStore{
		policies: []policy.Policy{
			{
				RecordBase: policy.RecordBase{
					ID:       "policy-load",
					TenantID: loadTenantID,
					Status:   "active",
				},
				Name:         "Load Policy",
				Version:      1,
				KioskMode:    true,
				Restrictions: []byte(`{"allowPackages":["com.example.loadapp"]}`),
			},
		},
		policyApps: []policy.PolicyApp{
			{PolicyID: "policy-load", AppID: loadAppID},
		},
	}

	svc := auth.NewServiceWithPermissions(loadAdminUsername, loadAdminPassword, time.Hour, []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite, auth.PermissionDevicesWrite})
	session, err := svc.Login(loadAdminUsername, loadAdminPassword)
	if err != nil {
		tb.Fatalf("login failed: %v", err)
	}

	mux := http.NewServeMux()
	adminhttp.RegisterDashboard(mux, svc, adminhttp.DashboardDependencies{
		Commands:      commandStore,
		Devices:       devices,
		PluginManager: plugins.Disabled(),
		TenantID:      loadTenantID,
	})
	apiMux := httpx.WithPrefix(mux, "/api/v1")
	enrollmenthttp.Register(apiMux, svc, devices, nil, appsStore, nil, artifactStore, certsStore, policyStore, enrollment.RuntimeSnapshot{
		CommandPollIntervalMs: 250,
		ConfigSyncIntervalMs:  1000,
	}, loadTenantID)
	commandhttp.Register(apiMux, devices, commandStore, loadTenantID)
	apphttp.Register(apiMux, svc, appsStore, devices, artifactStore, nil, loadTenantID, "com.xmdm.launcher")
	certificatehttp.Register(apiMux, svc, devices, certsStore, artifactStore, nil, loadTenantID)
	managedfilehttp.Register(apiMux, svc, managedFilesStore, devices, artifactStore, loadTenantID)
	telemetryhttp.Register(apiMux, telemetryStore, loadTenantID)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("listen load test server: %v", err)
	}
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	tb.Cleanup(server.Close)

	return &loadFixture{
		client:        server.Client(),
		baseURL:       server.URL,
		sessionCookie: &http.Cookie{Name: auth.SessionCookieName, Value: session.ID},
		deviceSecret:  loadDeviceSecret,
		devices:       devices,
		apps:          appsStore,
		certificates:  certsStore,
		managedFiles:  managedFilesStore,
		artifacts:     artifactStore,
		commands:      commandStore,
		telemetry:     telemetryStore,
	}
}

func (f *loadFixture) withSession() requestOption {
	return func(req *http.Request) {
		req.AddCookie(f.sessionCookie)
	}
}

func (f *loadFixture) withDeviceSecret() requestOption {
	return func(req *http.Request) {
		req.Header.Set("X-XMDM-Device-Secret", f.deviceSecret)
	}
}

func (f *loadFixture) withJSON() requestOption {
	return func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
	}
}

func (f *loadFixture) withForm() requestOption {
	return func(req *http.Request) {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
}

func (f *loadFixture) withCSRF() requestOption {
	return func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "xmdm_csrf", Value: loadCSRFToken})
		req.Header.Set("X-XMDM-CSRF-Token", loadCSRFToken)
	}
}

func (f *loadFixture) newRequest(method, path string, body io.Reader, opts ...requestOption) *http.Request {
	req, err := http.NewRequest(method, f.baseURL+path, body)
	if err != nil {
		panic(err)
	}
	for _, opt := range opts {
		opt(req)
	}
	return req
}

type requestOption func(*http.Request)

func runLoadCase(tb testing.TB, client *http.Client, workers, requests int, build func() *http.Request, wantStatus int) {
	tb.Helper()
	if requests <= 0 {
		tb.Fatalf("invalid request count: %d", requests)
	}
	if workers <= 0 {
		tb.Fatalf("invalid worker count: %d", workers)
	}
	jobs := make(chan struct{}, workers)
	var (
		errMu    sync.Mutex
		firstErr error
	)
	recordErr := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				req := build()
				res, err := client.Do(req)
				if err != nil {
					recordErr(err)
					continue
				}
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
				if res.StatusCode != wantStatus {
					recordErr(fmt.Errorf("unexpected status %d, want %d", res.StatusCode, wantStatus))
				}
			}
		}()
	}
	for i := 0; i < requests; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		tb.Fatalf("load request failed: %v", firstErr)
	}
}

func runMixedLoadCase(tb testing.TB, client *http.Client, workers, requests int, builders []func() *http.Request, wantStatus int) {
	tb.Helper()
	if len(builders) == 0 {
		tb.Fatalf("mixed load requires builders")
	}
	if requests <= 0 {
		tb.Fatalf("invalid request count: %d", requests)
	}
	if workers <= 0 {
		tb.Fatalf("invalid worker count: %d", workers)
	}
	jobs := make(chan int, workers)
	var (
		errMu    sync.Mutex
		firstErr error
	)
	recordErr := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				req := builders[idx%len(builders)]()
				res, err := client.Do(req)
				if err != nil {
					recordErr(err)
					continue
				}
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
				if res.StatusCode != wantStatus {
					recordErr(fmt.Errorf("unexpected status %d, want %d", res.StatusCode, wantStatus))
				}
			}
		}()
	}
	for i := 0; i < requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		tb.Fatalf("load request failed: %v", firstErr)
	}
}

type loadDeviceStore struct {
	mu             sync.Mutex
	expectedSecret string
	device         device.Device
	authCount      int
}

func (s *loadDeviceStore) ListDevices(context.Context, string, pagination.Params) ([]device.Device, error) {
	return nil, nil
}

func (s *loadDeviceStore) ListDevicesByFilter(context.Context, string, pagination.Params, device.DeviceListFilter) ([]device.Device, error) {
	return nil, nil
}

func (s *loadDeviceStore) ListActiveDevices(context.Context, string) ([]device.Device, error) {
	if s.device.Status == device.StatusActive || s.device.Status == device.StatusEnrolled {
		return []device.Device{s.device}, nil
	}
	return nil, nil
}

func (s *loadDeviceStore) GetOverviewStats(context.Context, string) (device.OverviewStats, error) {
	stats := device.OverviewStats{Total: 1}
	switch s.device.Status {
	case device.StatusActive:
		stats.Active = 1
	case device.StatusPending:
		stats.Pending = 1
	}
	if s.device.Status == device.StatusRetired || s.device.Status == device.StatusWiped {
		stats.RetiredOrWiped = 1
	}
	if s.device.PolicyID != nil && *s.device.PolicyID != "" {
		stats.AssignedPolicy = 1
	}
	return stats, nil
}

func (s *loadDeviceStore) GetStatusCounts(context.Context, string) (device.StatusCounts, error) {
	counts := device.StatusCounts{}
	switch s.device.Status {
	case device.StatusPending:
		counts.Pending = 1
	case device.StatusEnrolled:
		counts.Enrolled = 1
	case device.StatusActive:
		counts.Active = 1
	case device.StatusLocked:
		counts.Locked = 1
	case device.StatusSuspended:
		counts.Suspended = 1
	case device.StatusRetired:
		counts.Retired = 1
	case device.StatusWiped:
		counts.Wiped = 1
	}
	return counts, nil
}

func (s *loadDeviceStore) GetDevice(_ context.Context, _ string, id string) (device.Device, error) {
	if s.device.ID == id {
		return s.device, nil
	}
	return device.Device{}, httpx.ErrNotFound
}

func (s *loadDeviceStore) CreateDevice(context.Context, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}

func (s *loadDeviceStore) UpdateDevice(context.Context, string, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}

func (s *loadDeviceStore) RetireDevice(context.Context, string, string) (device.Device, error) {
	return device.Device{}, nil
}

func (s *loadDeviceStore) Authenticate(_ context.Context, _ string, deviceID, secret string) (device.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authCount++
	expectedID := s.device.ID
	if expectedID == "" {
		expectedID = s.device.Name
	}
	if deviceID != expectedID || secret != s.expectedSecret {
		return device.Device{}, httpx.ErrNotFound
	}
	return s.device, nil
}

func (s *loadDeviceStore) authenticated() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authCount
}

type loadAppStore struct {
	mu       sync.Mutex
	apps     []apps.App
	versions map[string][]apps.Version
}

func (s *loadAppStore) ListApps(context.Context, string, pagination.Params) ([]apps.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]apps.App(nil), s.apps...), nil
}

func (s *loadAppStore) GetOverviewStats(context.Context, string) (apps.OverviewStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := apps.OverviewStats{Total: len(s.apps)}
	for _, item := range s.apps {
		if item.Status == apps.StatusActive {
			stats.Active++
		}
	}
	return stats, nil
}

func (s *loadAppStore) GetApp(_ context.Context, _ string, id string) (apps.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.apps {
		if item.ID == id {
			return item, nil
		}
	}
	return apps.App{}, httpx.ErrNotFound
}

func (s *loadAppStore) GetAppByPackageName(_ context.Context, _ string, packageName string) (apps.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.apps {
		if item.PackageName == packageName {
			return item, nil
		}
	}
	return apps.App{}, httpx.ErrNotFound
}

func (s *loadAppStore) CreateApp(context.Context, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}

func (s *loadAppStore) UpdateApp(context.Context, string, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}

func (s *loadAppStore) RetireApp(context.Context, string, string) (apps.App, error) {
	return apps.App{}, nil
}

func (s *loadAppStore) ListVersions(_ context.Context, _ string, appID string, _ pagination.Params) ([]apps.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]apps.Version(nil), s.versions[appID]...), nil
}

func (s *loadAppStore) GetVersionByCode(_ context.Context, _ string, appID string, versionCode int64) (apps.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, version := range s.versions[appID] {
		if version.VersionCode == versionCode {
			return version, nil
		}
	}
	return apps.Version{}, httpx.ErrNotFound
}

func (s *loadAppStore) GetLatestPublishedVersion(_ context.Context, _ string, appID string) (apps.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest apps.Version
	found := false
	for _, version := range s.versions[appID] {
		if version.Status != apps.VersionStatusPublished {
			continue
		}
		if !found || (version.PublishedAt != nil && latest.PublishedAt != nil && version.PublishedAt.After(*latest.PublishedAt)) || (version.PublishedAt != nil && latest.PublishedAt == nil) || (version.PublishedAt == nil && latest.PublishedAt == nil && version.CreatedAt.After(latest.CreatedAt)) {
			latest = version
			found = true
		}
	}
	if !found {
		return apps.Version{}, httpx.ErrNotFound
	}
	return latest, nil
}

func (s *loadAppStore) GetVersion(_ context.Context, _ string, appID, versionID string) (apps.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, version := range s.versions[appID] {
		if version.ID == versionID {
			return version, nil
		}
	}
	return apps.Version{}, httpx.ErrNotFound
}

func (s *loadAppStore) CreateVersion(context.Context, string, string, apps.VersionUpsert) (apps.Version, error) {
	return apps.Version{}, nil
}

type loadCertificateStore struct {
	mu           sync.Mutex
	certificates []certificates.Certificate
}

func (s *loadCertificateStore) ListCertificates(context.Context, string, pagination.Params) ([]certificates.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]certificates.Certificate(nil), s.certificates...), nil
}

func (s *loadCertificateStore) ListActiveCertificates(context.Context, string, pagination.Params) ([]certificates.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]certificates.Certificate(nil), s.certificates...), nil
}

func (s *loadCertificateStore) GetOverviewStats(context.Context, string) (certificates.OverviewStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := certificates.OverviewStats{Total: len(s.certificates), Active: len(s.certificates)}
	return stats, nil
}

func (s *loadCertificateStore) GetCertificate(_ context.Context, _ string, certificateID string) (certificates.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cert := range s.certificates {
		if cert.ID == certificateID {
			return cert, nil
		}
	}
	return certificates.Certificate{}, httpx.ErrNotFound
}

func (s *loadCertificateStore) CreateCertificate(context.Context, string, certificates.CertificateUpsert) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

func (s *loadCertificateStore) RetireCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

type loadArtifactStore struct {
	mu        sync.Mutex
	artifacts map[string][]byte
	getCount  int
}

func (s *loadArtifactStore) Put(context.Context, string, io.Reader, string, int64) error {
	return nil
}

func (s *loadArtifactStore) Get(_ context.Context, storageKey string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCount++
	content, ok := s.artifacts[storageKey]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (s *loadArtifactStore) Delete(context.Context, string) error {
	return nil
}

func (s *loadArtifactStore) HealthCheck(context.Context) error {
	return nil
}

func (s *loadArtifactStore) gets() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getCount
}

type loadCommandStore struct {
	mu            sync.Mutex
	nextID        int
	enqueuedCount int
	ackedCount    int
}

func (s *loadCommandStore) Enqueue(_ context.Context, tenantID string, req commands.Upsert) ([]commands.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	s.enqueuedCount++
	cmd := commands.Command{
		ID:        fmt.Sprintf("cmd-%d", s.nextID),
		TenantID:  tenantID,
		Type:      req.Type,
		Payload:   req.Payload,
		Status:    commands.StatusQueued,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return []commands.Command{cmd}, nil
}

func (s *loadCommandStore) ListRecent(context.Context, string, pagination.Params) ([]commands.Command, error) {
	return nil, nil
}

func (s *loadCommandStore) ListRecentAll(context.Context, string) ([]commands.Command, error) {
	return nil, nil
}

func (s *loadCommandStore) GetOverviewStats(context.Context, string) (commands.OverviewStats, error) {
	return commands.OverviewStats{}, nil
}

func (s *loadCommandStore) Get(context.Context, string, string) (commands.Command, error) {
	return commands.Command{}, httpx.ErrNotFound
}

func (s *loadCommandStore) ListPending(context.Context, string, string, pagination.Params) ([]commands.Command, error) {
	return nil, nil
}

func (s *loadCommandStore) ListPendingForDevice(context.Context, string, string) ([]commands.Command, error) {
	return nil, nil
}

func (s *loadCommandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ackedCount++
	return commands.Command{
		ID:       "cmd-load",
		Status:   commands.StatusAcked,
		DeviceID: loadDeviceID,
	}, nil
}

func (s *loadCommandStore) enqueued() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enqueuedCount
}

func (s *loadCommandStore) acked() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ackedCount
}

type loadManagedFileStore struct {
	mu    sync.Mutex
	items []managedfiles.ManagedFile
}

func (s *loadManagedFileStore) ListManagedFiles(context.Context, string, pagination.Params) ([]managedfiles.ManagedFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]managedfiles.ManagedFile(nil), s.items...), nil
}

func (s *loadManagedFileStore) GetOverviewStats(context.Context, string) (managedfiles.OverviewStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return managedfiles.OverviewStats{Total: len(s.items), Active: len(s.items)}, nil
}

func (s *loadManagedFileStore) GetManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) == 0 {
		return managedfiles.ManagedFile{}, httpx.ErrNotFound
	}
	return s.items[0], nil
}

func (s *loadManagedFileStore) CreateManagedFile(context.Context, string, managedfiles.ManagedFileUpsert) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}

func (s *loadManagedFileStore) RetireManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}

type loadTelemetryStore struct {
	mu           sync.Mutex
	uploadsCount int
}

func (s *loadTelemetryStore) Upload(context.Context, string, string, string, telemetry.UploadRequest) (telemetry.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploadsCount++
	return telemetry.Record{
		ID:         fmt.Sprintf("telemetry-%d", s.uploadsCount),
		TenantID:   loadTenantID,
		DeviceID:   loadDeviceID,
		ObservedAt: time.Now().UTC(),
		Payload:    map[string]any{"heartbeat": true},
	}, nil
}

func (s *loadTelemetryStore) uploads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.uploadsCount
}

type loadPolicyStore struct {
	mu                 sync.Mutex
	policies           []policy.Policy
	policyApps         []policy.PolicyApp
	policyCertificates []policy.PolicyCertificate
	policyManagedFiles []policy.PolicyManagedFile
}

func (s *loadPolicyStore) ListPolicies(context.Context, string, pagination.Params) ([]policy.Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]policy.Policy(nil), s.policies...), nil
}

func (s *loadPolicyStore) ListActivePolicies(context.Context, string) ([]policy.Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]policy.Policy, 0, len(s.policies))
	for _, item := range s.policies {
		if item.Status == policy.StatusActive {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *loadPolicyStore) GetOverviewStats(context.Context, string) (policy.OverviewStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := policy.OverviewStats{Total: len(s.policies)}
	for _, item := range s.policies {
		switch item.Status {
		case policy.StatusActive:
			stats.Active++
		case policy.StatusRetired:
			stats.Retired++
		}
	}
	return stats, nil
}

func (s *loadPolicyStore) GetPolicy(_ context.Context, _ string, id string) (policy.Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.policies {
		if item.ID == id {
			return item, nil
		}
	}
	return policy.Policy{}, httpx.ErrNotFound
}

func (s *loadPolicyStore) CreatePolicy(context.Context, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *loadPolicyStore) UpdatePolicy(context.Context, string, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *loadPolicyStore) RetirePolicy(context.Context, string, string) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *loadPolicyStore) ListPolicyApps(context.Context, string, string, pagination.Params) ([]policy.PolicyApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]policy.PolicyApp(nil), s.policyApps...), nil
}

func (s *loadPolicyStore) ListActivePolicyApps(context.Context, string, string) ([]policy.PolicyApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]policy.PolicyApp, 0, len(s.policyApps))
	for _, item := range s.policyApps {
		if item.Status == policy.StatusActive {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *loadPolicyStore) GetPolicyApp(_ context.Context, tenantID, policyID, appID string) (policy.PolicyApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.policyApps {
		if item.TenantID == tenantID && item.PolicyID == policyID && item.AppID == appID {
			return item, nil
		}
	}
	return policy.PolicyApp{}, httpx.ErrNotFound
}

func (s *loadPolicyStore) AddPolicyApp(context.Context, string, string, string) (policy.PolicyApp, error) {
	return policy.PolicyApp{}, nil
}

func (s *loadPolicyStore) RemovePolicyApp(context.Context, string, string, string) error {
	return nil
}

func (s *loadPolicyStore) ListPolicyCertificates(context.Context, string, string, pagination.Params) ([]policy.PolicyCertificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]policy.PolicyCertificate(nil), s.policyCertificates...), nil
}

func (s *loadPolicyStore) ListActivePolicyCertificates(context.Context, string, string) ([]policy.PolicyCertificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]policy.PolicyCertificate, 0, len(s.policyCertificates))
	for _, item := range s.policyCertificates {
		if item.Status == policy.StatusActive {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *loadPolicyStore) GetPolicyCertificate(_ context.Context, tenantID, policyID, certificateID string) (policy.PolicyCertificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.policyCertificates {
		if item.TenantID == tenantID && item.PolicyID == policyID && item.CertificateID == certificateID {
			return item, nil
		}
	}
	return policy.PolicyCertificate{}, httpx.ErrNotFound
}

func (s *loadPolicyStore) AddPolicyCertificate(context.Context, string, string, string) (policy.PolicyCertificate, error) {
	return policy.PolicyCertificate{}, nil
}

func (s *loadPolicyStore) RemovePolicyCertificate(context.Context, string, string, string) error {
	return nil
}

func (s *loadPolicyStore) ListPolicyManagedFiles(context.Context, string, string, pagination.Params) ([]policy.PolicyManagedFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]policy.PolicyManagedFile(nil), s.policyManagedFiles...), nil
}

func (s *loadPolicyStore) ListActivePolicyManagedFiles(context.Context, string, string) ([]policy.PolicyManagedFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]policy.PolicyManagedFile, 0, len(s.policyManagedFiles))
	for _, item := range s.policyManagedFiles {
		if item.Status == policy.StatusActive {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *loadPolicyStore) GetPolicyManagedFile(_ context.Context, tenantID, policyID, managedFileID string) (policy.PolicyManagedFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.policyManagedFiles {
		if item.TenantID == tenantID && item.PolicyID == policyID && item.ManagedFileID == managedFileID {
			return item, nil
		}
	}
	return policy.PolicyManagedFile{}, httpx.ErrNotFound
}

func (s *loadPolicyStore) AddPolicyManagedFile(context.Context, string, string, string) (policy.PolicyManagedFile, error) {
	return policy.PolicyManagedFile{}, nil
}

func (s *loadPolicyStore) RemovePolicyManagedFile(context.Context, string, string, string) error {
	return nil
}

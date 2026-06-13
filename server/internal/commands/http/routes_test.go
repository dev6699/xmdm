package commandhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
)

func TestRegisterPollsPendingCommands(t *testing.T) {
	store := &fakeCommandStore{
		items: []commands.Command{
			{
				ID:      "cmd-1",
				Type:    "reboot",
				Status:  commands.StatusQueued,
				Payload: map[string]any{"force": true},
			},
		},
	}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/commands", nil)
	req.Header.Set(deviceSecretHeader, "secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	var payload PollResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Commands) != 1 || payload.Commands[0].ID != "cmd-1" || payload.Commands[0].Type != "reboot" {
		t.Fatalf("unexpected commands: %#v", payload.Commands)
	}
}

func TestRegisterPollsPendingCommandsAcrossPages(t *testing.T) {
	var items []commands.Command
	for i := 0; i < 40; i++ {
		items = append(items, commands.Command{ID: "cmd-" + strconv.Itoa(i+1), Type: "sync", Status: commands.StatusQueued})
	}
	store := &fakeCommandStore{items: items}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/commands", nil)
	req.Header.Set(deviceSecretHeader, "secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	var payload PollResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Commands) != 40 {
		t.Fatalf("expected all pending commands, got %d", len(payload.Commands))
	}
}

func TestRegisterRejectsBadDeviceSecret(t *testing.T) {
	store := &fakeCommandStore{}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/commands", nil)
	req.Header.Set(deviceSecretHeader, "wrong")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rr.Code)
	}
}

func TestRegisterRejectsWrongDeviceID(t *testing.T) {
	store := &fakeCommandStore{}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-999/commands", nil)
	req.Header.Set(deviceSecretHeader, "secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rr.Code)
	}
}

func TestRegisterAcksCommand(t *testing.T) {
	store := &fakeCommandStore{
		acked: commands.Command{
			ID:       "cmd-1",
			Type:     "reboot",
			Status:   commands.StatusAcked,
			DeviceID: "device-123",
		},
	}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/commands/cmd-1/ack", strings.NewReader(`{"status":"acked","message":"done"}`))
	req.Header.Set(deviceSecretHeader, "secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	var payload commands.Command
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != commands.StatusAcked || payload.ID != "cmd-1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRegisterRejectsAckForWrongCommandID(t *testing.T) {
	store := &fakeCommandStore{
		ackErr: httpx.ErrNotFound,
	}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/commands/cmd-404/ack", strings.NewReader(`{"status":"acked"}`))
	req.Header.Set(deviceSecretHeader, "secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected not found, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterRejectsAckWithBadSecret(t *testing.T) {
	store := &fakeCommandStore{}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/commands/cmd-1/ack", strings.NewReader(`{"status":"acked"}`))
	req.Header.Set(deviceSecretHeader, "wrong")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rr.Code)
	}
}

func TestRegisterRejectsAckForWrongDeviceID(t *testing.T) {
	store := &fakeCommandStore{}
	devices := &fakeDeviceStore{deviceID: "device-123", secret: "secret"}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), devices, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-999/commands/cmd-1/ack", strings.NewReader(`{"status":"acked"}`))
	req.Header.Set(deviceSecretHeader, "secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d body=%s", rr.Code, rr.Body.String())
	}
}

type fakeCommandStore struct {
	items  []commands.Command
	acked  commands.Command
	ackErr error
}

func (s *fakeCommandStore) Enqueue(context.Context, string, commands.Upsert) ([]commands.Command, error) {
	return nil, nil
}

func (s *fakeCommandStore) ListRecent(context.Context, string, pagination.Params) ([]commands.Command, error) {
	return append([]commands.Command(nil), s.items...), nil
}

func (s *fakeCommandStore) ListRecentAll(context.Context, string) ([]commands.Command, error) {
	return append([]commands.Command(nil), s.items...), nil
}

func (s *fakeCommandStore) GetOverviewStats(context.Context, string) (commands.OverviewStats, error) {
	return commands.OverviewStats{Total: len(s.items)}, nil
}

func (s *fakeCommandStore) Get(context.Context, string, string) (commands.Command, error) {
	return commands.Command{}, httpx.ErrNotFound
}

func (s *fakeCommandStore) ListPending(_ context.Context, _ string, _ string, params pagination.Params) ([]commands.Command, error) {
	if len(s.items) == 0 {
		return nil, nil
	}
	params = pagination.Normalize(params, pagination.DefaultLimit, 100)
	if params.Offset >= len(s.items) {
		return []commands.Command{}, nil
	}
	end := params.Offset + params.Limit
	if end > len(s.items) {
		end = len(s.items)
	}
	return append([]commands.Command(nil), s.items[params.Offset:end]...), nil
}

func (s *fakeCommandStore) ListPendingForDevice(context.Context, string, string) ([]commands.Command, error) {
	return append([]commands.Command(nil), s.items...), nil
}

func (s *fakeCommandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	if s.ackErr != nil {
		return commands.Command{}, s.ackErr
	}
	return s.acked, nil
}

type fakeDeviceStore struct {
	deviceID string
	secret   string
}

func (s *fakeDeviceStore) ListDevices(context.Context, string, pagination.Params) ([]device.Device, error) {
	return nil, nil
}

func (s *fakeDeviceStore) ListActiveDevices(context.Context, string) ([]device.Device, error) {
	return nil, nil
}

func (s *fakeDeviceStore) GetOverviewStats(context.Context, string) (device.OverviewStats, error) {
	return device.OverviewStats{}, nil
}

func (s *fakeDeviceStore) GetStatusCounts(context.Context, string) (device.StatusCounts, error) {
	return device.StatusCounts{}, nil
}

func (s *fakeDeviceStore) GetDevice(context.Context, string, string) (device.Device, error) {
	return device.Device{}, httpx.ErrNotFound
}

func (s *fakeDeviceStore) CreateDevice(context.Context, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}

func (s *fakeDeviceStore) UpdateDevice(context.Context, string, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}

func (s *fakeDeviceStore) RetireDevice(context.Context, string, string) (device.Device, error) {
	return device.Device{}, nil
}

func (s *fakeDeviceStore) Authenticate(_ context.Context, _ string, deviceID, secret string) (device.Device, error) {
	if deviceID == s.deviceID && secret == s.secret {
		return device.Device{RecordBase: device.RecordBase{ID: s.deviceID}, Name: s.deviceID}, nil
	}
	return device.Device{}, httpx.ErrNotFound
}

var _ commands.Repository = (*fakeCommandStore)(nil)
var _ device.Repository = (*fakeDeviceStore)(nil)

package commandhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
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

type fakeCommandStore struct {
	items []commands.Command
}

func (s *fakeCommandStore) Enqueue(context.Context, string, string, commands.Upsert) (commands.Command, error) {
	return commands.Command{}, nil
}

func (s *fakeCommandStore) ListPending(context.Context, string, string) ([]commands.Command, error) {
	return append([]commands.Command(nil), s.items...), nil
}

type fakeDeviceStore struct {
	deviceID string
	secret   string
}

func (s *fakeDeviceStore) ListDevices(context.Context, string) ([]device.Device, error) {
	return nil, nil
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
		return device.Device{}, nil
	}
	return device.Device{}, httpx.ErrNotFound
}

var _ commands.Repository = (*fakeCommandStore)(nil)
var _ device.Repository = (*fakeDeviceStore)(nil)

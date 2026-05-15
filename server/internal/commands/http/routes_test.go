package commandhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

type fakeCommandStore struct {
	items  []commands.Command
	acked  commands.Command
	ackErr error
}

func (s *fakeCommandStore) Enqueue(context.Context, string, commands.Upsert) ([]commands.Command, error) {
	return nil, nil
}

func (s *fakeCommandStore) ListRecent(context.Context, string, int) ([]commands.Command, error) {
	return append([]commands.Command(nil), s.items...), nil
}

func (s *fakeCommandStore) Get(context.Context, string, string) (commands.Command, error) {
	return commands.Command{}, httpx.ErrNotFound
}

func (s *fakeCommandStore) ListPending(context.Context, string, string) ([]commands.Command, error) {
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
		return device.Device{DeviceID: s.deviceID, Name: s.deviceID}, nil
	}
	return device.Device{}, httpx.ErrNotFound
}

var _ commands.Repository = (*fakeCommandStore)(nil)
var _ device.Repository = (*fakeDeviceStore)(nil)

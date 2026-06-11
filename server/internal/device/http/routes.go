package devicehttp

import (
	"context"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	device "xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
)

func Register(mux httpx.Router, svc *auth.Service, store device.Repository, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[device.DeviceUpsert, device.Device]{
		Kind:      "devices",
		ReadPerm:  auth.PermissionDevicesRead,
		WritePerm: auth.PermissionDevicesWrite,
		Decode:    decodeDeviceRequest,
		List: func(ctx context.Context, params pagination.Params) ([]device.Device, error) {
			return store.ListDevices(ctx, tenantID, params)
		},
		Create: func(ctx context.Context, req device.DeviceUpsert) (device.Device, error) {
			return store.CreateDevice(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req device.DeviceUpsert) (device.Device, error) {
			return store.UpdateDevice(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (device.Device, error) {
			return store.RetireDevice(ctx, tenantID, id)
		},
		Audit: func(rec device.Device) map[string]any {
			return map[string]any{"name": rec.Name, "deviceId": rec.ID}
		},
	})
}

func decodeDeviceRequest(r *http.Request) (device.DeviceUpsert, error) {
	var payload device.DeviceUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return device.DeviceUpsert{}, err
	}
	if payload.Name == "" || payload.SecretHash == "" {
		return device.DeviceUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}

package telemetryhttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"xmdm/server/internal/httpx"
	"xmdm/server/internal/telemetry"
)

const deviceSecretHeader = "X-XMDM-Device-Secret"

func Register(mux httpx.Router, store telemetry.Repository, tenantID string) {
	mux.HandleFunc("/devices/{deviceId}/telemetry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		deviceID := strings.TrimSpace(r.PathValue("deviceId"))
		if deviceID == "" {
			writeTelemetryError(w, httpx.ErrInvalidInput)
			return
		}
		secret := strings.TrimSpace(r.Header.Get(deviceSecretHeader))
		if secret == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		req, err := decodeTelemetryRequest(r)
		if err != nil {
			writeTelemetryError(w, err)
			return
		}
		rec, err := store.Upload(r.Context(), tenantID, deviceID, secret, req)
		if err != nil {
			writeTelemetryError(w, err)
			return
		}
		writeJSON(w, rec)
	})
}

func decodeTelemetryRequest(r *http.Request) (telemetry.UploadRequest, error) {
	var payload telemetry.UploadRequest
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return telemetry.UploadRequest{}, err
	}
	if payload.ObservedAt.IsZero() {
		payload.ObservedAt = time.Now().UTC()
	}
	if payload.Heartbeat == nil && payload.Battery == nil && payload.Network == nil && payload.Location == nil && payload.AppState == nil {
		return telemetry.UploadRequest{}, httpx.ErrInvalidInput
	}
	return payload, nil
}

func writeTelemetryError(w http.ResponseWriter, err error) {
	switch err {
	case httpx.ErrInvalidInput, telemetry.ErrTelemetryInvalid:
		http.Error(w, "invalid input", http.StatusBadRequest)
	case telemetry.ErrDeviceNotFound:
		http.Error(w, "not found", http.StatusNotFound)
	case telemetry.ErrDeviceUnauthorized:
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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

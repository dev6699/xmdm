package telemetry

import "errors"

var (
	ErrDeviceNotFound     = errors.New("device not found")
	ErrDeviceUnauthorized = errors.New("device unauthorized")
	ErrTelemetryInvalid   = errors.New("invalid telemetry payload")
	ErrTelemetryNotFound  = errors.New("telemetry not found")
	ErrTelemetryConflict  = errors.New("telemetry conflict")
)

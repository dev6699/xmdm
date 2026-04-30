package logs

import "errors"

var (
	ErrLogsInvalid        = errors.New("invalid logs payload")
	ErrDeviceNotFound     = errors.New("device not found")
	ErrDeviceUnauthorized = errors.New("device unauthorized")
)

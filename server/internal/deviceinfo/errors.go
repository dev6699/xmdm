package deviceinfo

import "errors"

var (
	ErrDeviceInfoInvalid   = errors.New("device info invalid")
	ErrDeviceNotFound      = errors.New("device not found")
	ErrDeviceUnauthorized  = errors.New("device unauthorized")
	ErrDeviceInfoMalformed = errors.New("device info malformed")
)

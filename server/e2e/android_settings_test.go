package e2e_test

import "fmt"

const (
	// Android Settings.Global.STAY_ON_WHILE_PLUGGED_IN bitmask values.
	stayOnWhilePluggedInAC       = 1
	stayOnWhilePluggedInUSB      = 2
	stayOnWhilePluggedInWireless = 4
	stayOnWhilePluggedInDock     = 8
)

// stayOnWhilePluggedInMask returns the Android stay-on bitmask for kiosk
// charging behavior.
//
// Android stores the configured power sources as a bitmask:
// 1 = AC, 2 = USB, 4 = wireless, 8 = dock.
// The common kiosk values are therefore:
// 7  = AC + USB + wireless
// 15 = AC + USB + wireless + dock
func stayOnWhilePluggedInMask(includeDock bool) string {
	mask := stayOnWhilePluggedInAC | stayOnWhilePluggedInUSB | stayOnWhilePluggedInWireless
	if includeDock {
		mask |= stayOnWhilePluggedInDock
	}
	return fmt.Sprint(mask)
}

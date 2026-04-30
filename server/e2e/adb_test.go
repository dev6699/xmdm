package e2e_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// startLauncher issues an ADB intent to (re)start the launcher with the given bootstrap URI.
func startLauncher(t *testing.T, serial, bootstrapURI string) {
	t.Helper()
	if _, err := adb(serial, "shell", "am", "start", "-S", "-n",
		"com.xmdm.launcher/.MainActivity",
		"-d", bootstrapURI,
	); err != nil {
		t.Fatalf("start launcher on device: %v", err)
	}
}

// waitForManagedFileOnDevice polls until the managed file appears on the device with the expected content.
func waitForManagedFileOnDevice(t *testing.T, serial, deviceID string) {
	t.Helper()
	want := "managed-file-on-device " + deviceID + " Acme"
	waitForCondition(t, time.Minute, "managed file to render on device",
		func() string { return adbContentSnapshot(t, serial) },
		func() (bool, error) {
			out, err := adbShellOutput(serial, "run-as", "com.xmdm.launcher", "cat", "files/managed-files/adb-managed-file.txt")
			if err != nil {
				return false, nil
			}
			return strings.TrimSpace(out) == want, nil
		},
	)
}

// waitForChromeInstalled polls until Chrome is listed by the package manager.
func waitForChromeInstalled(t *testing.T, serial string) {
	t.Helper()
	waitForCondition(t, time.Minute, "Chrome to be installed on device",
		func() string { return adbChromeSnapshot(t, serial) },
		func() (bool, error) {
			out, err := adbShellOutput(serial, "pm", "list", "packages", "com.android.chrome")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "com.android.chrome"), nil
		},
	)
}

// waitForChromeUninstalled polls until Chrome is no longer listed by the package manager.
func waitForChromeUninstalled(t *testing.T, serial string) {
	t.Helper()
	waitForCondition(t, time.Minute, "Chrome to be uninstalled from device",
		func() string { return adbChromeSnapshot(t, serial) },
		func() (bool, error) {
			out, err := adbShellOutput(serial, "pm", "list", "packages", "com.android.chrome")
			if err != nil {
				return false, nil
			}
			return !strings.Contains(out, "com.android.chrome"), nil
		},
	)
}

// waitForManagedFileRemovedFromDevice polls until the managed file disappears from the device.
func waitForManagedFileRemovedFromDevice(t *testing.T, serial string) {
	t.Helper()
	waitForCondition(t, time.Minute, "managed file to be removed from device",
		func() string { return adbContentSnapshot(t, serial) },
		func() (bool, error) {
			_, err := adbShellOutput(serial, "run-as", "com.xmdm.launcher", "ls", "files/managed-files/adb-managed-file.txt")
			return err != nil, nil
		},
	)
}

// waitForPackageSuspendedOnDevice polls until the package manager reports the package as suspended.
func waitForPackageSuspendedOnDevice(t *testing.T, serial, packageName string) {
	t.Helper()
	waitForCondition(t, time.Minute, packageName+" to be suspended on device",
		func() string { return adbPackageSnapshot(t, serial, packageName) },
		func() (bool, error) {
			out, err := adbShellOutput(serial, "dumpsys", "package", packageName)
			if err != nil {
				return false, nil
			}
			return isPackageSuspendedDump(out), nil
		},
	)
}

// waitForKioskModeOnDevice polls until the launcher enters lock-task mode.
func waitForKioskModeOnDevice(t *testing.T, serial string) {
	t.Helper()
	waitForCondition(t, time.Minute, "launcher to enter kiosk mode",
		func() string { return adbKioskSnapshot(t, serial) },
		func() (bool, error) {
			out, err := adbShellOutput(serial, "dumpsys", "activity", "activities")
			if err != nil {
				return false, nil
			}
			return isKioskModeDump(out), nil
		},
	)
}

// waitForCommandTransportWarmup gives the launcher a short window to finish
// bringing up its MQTT subscription after the config snapshot has landed.
func waitForCommandTransportWarmup(t *testing.T) {
	t.Helper()
	time.Sleep(5 * time.Second)
}

// reverseADBPort sets up port forwarding from the device to the host for the given server URL's port.
func reverseADBPort(t *testing.T, serial, baseURL string) {
	t.Helper()
	port := serverPortFromURL(t, baseURL)
	if _, err := adb(serial, "reverse", "tcp:"+port, "tcp:"+port); err != nil {
		t.Fatalf("adb reverse: %v", err)
	}
}

// removeADBPortReverse tears down port forwarding previously set up by reverseADBPort.
func removeADBPortReverse(serial, baseURL string) error {
	port, err := serverPortFromURLNoTest(baseURL)
	if err != nil {
		return err
	}
	_, err = adb(serial, "reverse", "--remove", "tcp:"+port, "tcp:"+port)
	return err
}

// resetADBLauncherState uninstalls the launcher (and Chrome), wipes app data, then reinstalls the APK.
func resetADBLauncherState(t *testing.T, serial, launcherAPKPath string) {
	t.Helper()

	stopAndUninstallLauncher(t, serial)
	uninstallChrome(serial)

	if strings.TrimSpace(launcherAPKPath) == "" {
		return
	}
	if _, err := adb(serial, "install", "-r", launcherAPKPath); err != nil {
		t.Fatalf("install launcher apk: %v", err)
	}
	if _, err := adb(serial, "shell", "run-as", "com.xmdm.launcher", "rm", "-rf",
		"files", "cache", "shared_prefs", "databases",
	); err != nil {
		t.Fatalf("wipe launcher app data: %v", err)
	}
}

// stopAndUninstallLauncher force-stops and removes the launcher package and its device-admin registration.
func stopAndUninstallLauncher(t *testing.T, serial string) {
	t.Helper()
	if _, err := adb(serial, "shell", "am", "force-stop", "com.xmdm.launcher"); err != nil {
		t.Fatalf("force-stop launcher: %v", err)
	}
	_, _ = adb(serial, "shell", "dpm", "remove-active-admin", "com.xmdm.launcher/.AdminReceiver")
	_, _ = adb(serial, "shell", "dpm", "remove-active-admin", "--user", "0", "com.xmdm.launcher/.AdminReceiver")
	_, _ = adb(serial, "shell", "pm", "uninstall", "--user", "0", "com.xmdm.launcher")
	_, _ = adb(serial, "shell", "pm", "clear", "com.xmdm.launcher")
}

// uninstallChrome removes Chrome from the device (best-effort).
func uninstallChrome(serial string) {
	adb(serial, "shell", "pm", "uninstall", "--user", "0", "com.android.chrome")
}

// waitForCondition retries fn every 5 seconds until it returns true or the timeout elapses.
// snapshot is called periodically to log diagnostic state.
func waitForCondition(t *testing.T, timeout time.Duration, description string, snapshot func() string, fn func() (bool, error)) {
	t.Helper()
	start := time.Now()
	deadline := start.Add(timeout)
	var lastErr error
	var lastLogAt time.Time
	attempts := 0

	for time.Now().Before(deadline) {
		attempts++
		ok, err := fn()
		if err == nil && ok {
			t.Logf("wait succeeded: %s attempts=%d elapsed=%s", description, attempts, time.Since(start).Truncate(time.Second))
			return
		}
		if err != nil {
			lastErr = err
		}
		if lastLogAt.IsZero() || time.Since(lastLogAt) >= 30*time.Second {
			if snapshot != nil {
				t.Logf("waiting for %s attempts=%d elapsed=%s state=%s", description, attempts, time.Since(start).Truncate(time.Second), snapshot())
			} else {
				t.Logf("waiting for %s attempts=%d elapsed=%s", description, attempts, time.Since(start).Truncate(time.Second))
			}
			lastLogAt = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	if snapshot != nil {
		t.Logf("wait timed out for %s state=%s", description, snapshot())
	}
	if lastErr != nil {
		t.Fatalf("timeout waiting for %s: %v", description, lastErr)
	}
	t.Fatalf("timeout waiting for %s", description)
}

// adb runs an adb command against the given serial and returns combined stdout.
func adb(serial string, args ...string) (string, error) {
	cmdArgs := append([]string{"-s", serial}, args...)
	cmd := exec.Command("adb", cmdArgs...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// adbShellOutput is a convenience wrapper that prepends "shell" to the args list.
func adbShellOutput(serial string, args ...string) (string, error) {
	return adb(serial, append([]string{"shell"}, args...)...)
}

func adbKioskSnapshot(t *testing.T, serial string) string {
	t.Helper()
	out, err := adbShellOutput(serial, "dumpsys", "activity", "activities")
	if err != nil {
		return "kiosk_err=" + strings.TrimSpace(err.Error())
	}
	lines := strings.Split(out, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "locktask") || strings.Contains(lower, "lock task") {
			filtered = append(filtered, line)
			continue
		}
		if strings.Contains(lower, "mlocktaskmodestate") {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		return "kiosk=unknown"
	}
	return strings.Join(filtered, " | ")
}

func adbPackageSnapshot(t *testing.T, serial, packageName string) string {
	t.Helper()
	out, err := adbShellOutput(serial, "dumpsys", "package", packageName)
	if err != nil {
		return "package_err=" + strings.TrimSpace(err.Error())
	}
	lines := strings.Split(out, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "suspended") || strings.Contains(lower, "hidden") || strings.Contains(lower, "enabled") {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		return "package=unknown"
	}
	return strings.Join(filtered, " | ")
}

func isKioskModeDump(out string) bool {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "mlocktaskmodestate=locked") || strings.Contains(lower, "mlocktaskmodestate=1") {
		return true
	}
	if strings.Contains(lower, "locktaskmode=locked") || strings.Contains(lower, "lock task mode: locked") {
		return true
	}
	return false
}

func isPackageSuspendedDump(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "suspended=true")
}

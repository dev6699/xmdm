package e2e_test

import "testing"

func TestDeviceInfoReporting(t *testing.T) {
	env := newContentTestEnv(t)

	waitForDeviceInfoUpload(t, env.requests, env.deviceID)
	assertDeviceInfoUploadPayload(t, env.requests, env.deviceID)
	assertDeviceInfoRecordedViaAPI(t, env.client, env.baseURL, env.deviceID)
}

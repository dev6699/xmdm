package enrollment

import "testing"

func TestSnapshotRevisionChangesWithContent(t *testing.T) {
	base := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false},
		nil,
		nil,
		nil,
	)
	if base.Version == "" {
		t.Fatalf("expected non-empty revision")
	}

	same := NewBootstrapConfigSnapshot(
		"device-abc",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false},
		nil,
		nil,
		nil,
	)
	if same.Version != base.Version {
		t.Fatalf("expected device identity not to affect revision: %q != %q", same.Version, base.Version)
	}

	changedPolicy := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{
			Name:            "policy",
			Version:         2,
			KioskMode:       true,
			KioskAppPackage: "com.example.kiosk",
			Restrictions: PolicyRestrictions{
				KioskExitPasscodeHash: "hash-1",
			},
		},
		nil,
		nil,
		nil,
	)
	if changedPolicy.Version == base.Version {
		t.Fatalf("expected policy change to affect revision")
	}

	changedKioskPackage := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false, KioskAppPackage: "com.example.kiosk"},
		nil,
		nil,
		nil,
	)
	if changedKioskPackage.Version == base.Version {
		t.Fatalf("expected kiosk app package change to affect revision")
	}

	changedKioskExitPasscode := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{
			KioskMode: false,
			Restrictions: PolicyRestrictions{
				KioskExitPasscodeHash: "hash-1",
			},
		},
		nil,
		nil,
		nil,
	)
	if changedKioskExitPasscode.Version == base.Version {
		t.Fatalf("expected kiosk exit passcode change to affect revision")
	}

	changedRuntime := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "10.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false},
		nil,
		nil,
		nil,
	)
	if changedRuntime.Version == base.Version {
		t.Fatalf("expected runtime change to affect revision")
	}

	changedApps := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false},
		[]AppSnapshot{{AppID: "app-1", PackageName: "com.example.app", VersionID: "v1", VersionName: "1", VersionCode: 1, ArtifactID: "artifact-1", Checksum: "abc", DownloadPath: "/artifact"}},
		nil,
		nil,
	)
	if changedApps.Version == base.Version {
		t.Fatalf("expected app change to affect revision")
	}

	changedFiles := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false},
		nil,
		[]ManagedFileSnapshot{{FileID: "file-1", Name: "file", Path: "config.txt", DownloadPath: "/artifact", Checksum: "xyz"}},
		nil,
	)
	if changedFiles.Version == base.Version {
		t.Fatalf("expected file change to affect revision")
	}

	changedCertificates := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false},
		nil,
		nil,
		[]CertificateSnapshot{{ID: "cert-1", Name: "cert", ArtifactID: "artifact-1", Checksum: "cert-checksum", DownloadPath: "/artifact"}},
	)
	if changedCertificates.Version == base.Version {
		t.Fatalf("expected certificate change to affect revision")
	}
}

func TestVerifyConfigSnapshotRejectsMissingOrInvalidSignature(t *testing.T) {
	snapshot := NewBootstrapConfigSnapshot(
		"device-123",
		RuntimeSnapshot{MqttAddress: "127.0.0.1:1883", CommandPollIntervalMs: 1000, ConfigSyncIntervalMs: 1000},
		PolicySnapshot{KioskMode: false},
		nil,
		nil,
		nil,
	)

	if err := VerifyConfigSnapshot(snapshot, "device-secret"); err == nil {
		t.Fatalf("expected missing signature to be rejected")
	}

	signed, err := SignConfigSnapshot(snapshot, "device-secret")
	if err != nil {
		t.Fatalf("sign snapshot: %v", err)
	}
	if err := VerifyConfigSnapshot(signed, "device-secret"); err != nil {
		t.Fatalf("verify signed snapshot: %v", err)
	}

	signed.Signature = "tampered"
	if err := VerifyConfigSnapshot(signed, "device-secret"); err == nil {
		t.Fatalf("expected tampered signature to be rejected")
	}
}

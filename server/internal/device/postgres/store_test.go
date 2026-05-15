package devicepg

import (
	"errors"
	"testing"
	"time"

	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCanAuthenticateDeviceStatus(t *testing.T) {
	if !canAuthenticateDeviceStatus(device.StatusEnrolled) {
		t.Fatalf("expected enrolled devices to authenticate")
	}
	if !canAuthenticateDeviceStatus(device.StatusActive) {
		t.Fatalf("expected active devices to authenticate")
	}
	if canAuthenticateDeviceStatus(device.StatusRetired) {
		t.Fatalf("expected retired devices to be rejected")
	}
	if canAuthenticateDeviceStatus(device.StatusWiped) {
		t.Fatalf("expected wiped devices to be rejected")
	}
}

func TestScanAuthenticatedDeviceRejectsRetiredAndWiped(t *testing.T) {
	for _, status := range []string{device.StatusRetired, device.StatusWiped} {
		t.Run(status, func(t *testing.T) {
			rec, err := scanAuthenticatedDevice(fakeRowScanner{
				scan: func(dest ...any) error {
					*(dest[0].(*string)) = "device-row-123"
					*(dest[1].(*string)) = "tenant-1"
					*(dest[2].(*string)) = "device-123"
					*(dest[3].(*string)) = "name"
					*(dest[4].(*string)) = status
					now := time.Now()
					*(dest[5].(*pgtype.Timestamptz)) = pgtype.Timestamptz{Time: now, Valid: true}
					*(dest[6].(*time.Time)) = now
					*(dest[10].(*[]string)) = nil
					return nil
				},
			})
			if !errors.Is(err, httpx.ErrNotFound) {
				t.Fatalf("expected not found, got %v", err)
			}
			if rec.ID != "" {
				t.Fatalf("expected empty record, got %#v", rec)
			}
		})
	}
}

type fakeRowScanner struct {
	scan func(...any) error
}

func (f fakeRowScanner) Scan(dest ...any) error {
	if f.scan == nil {
		return pgx.ErrNoRows
	}
	return f.scan(dest...)
}

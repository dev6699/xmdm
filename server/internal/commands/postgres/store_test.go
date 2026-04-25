package commandspg

import (
	"errors"
	"testing"
	"time"

	"xmdm/server/internal/commands"
	"xmdm/server/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestScanCommandDecodesPayloadAndExpiry(t *testing.T) {
	expiry := time.Date(2026, 4, 25, 15, 0, 0, 0, time.UTC)
	rec, err := scanCommand(fakeRowScanner{
		scan: func(dest ...any) error {
			*(dest[0].(*string)) = "cmd-1"
			*(dest[1].(*string)) = "tenant-1"
			*(dest[2].(*string)) = "device-1"
			*(dest[3].(*string)) = "reboot"
			*(dest[4].(*[]byte)) = []byte(`{"force":true}`)
			*(dest[5].(*string)) = commands.StatusQueued
			*(dest[6].(*pgtype.Timestamptz)) = pgtype.Timestamptz{Time: expiry, Valid: true}
			*(dest[7].(*time.Time)) = time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC)
			*(dest[8].(*time.Time)) = time.Date(2026, 4, 25, 14, 30, 0, 0, time.UTC)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("scan command: %v", err)
	}
	if rec.ID != "cmd-1" || rec.Type != "reboot" || rec.Status != commands.StatusQueued {
		t.Fatalf("unexpected command: %#v", rec)
	}
	if rec.ExpiresAt == nil || !rec.ExpiresAt.Equal(expiry) {
		t.Fatalf("unexpected expiry: %#v", rec.ExpiresAt)
	}
	if got := rec.Payload["force"]; got != true {
		t.Fatalf("unexpected payload: %#v", rec.Payload)
	}
}

func TestScanCommandMapsNoRowsToNotFound(t *testing.T) {
	_, err := scanCommand(fakeRowScanner{})
	if !errors.Is(err, httpx.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
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

package policypg

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestScanPolicyAllowsNullKioskAppPackage(t *testing.T) {
	rec, err := scanPolicy(fakePolicyRow{kioskAppPackage: nil})
	if err != nil {
		t.Fatalf("scan policy: %v", err)
	}
	if rec.KioskAppPackage != "" {
		t.Fatalf("expected empty kiosk app package, got %q", rec.KioskAppPackage)
	}
}

func TestScanPolicyReadsKioskAppPackage(t *testing.T) {
	rec, err := scanPolicy(fakePolicyRow{kioskAppPackage: "com.android.chrome"})
	if err != nil {
		t.Fatalf("scan policy: %v", err)
	}
	if rec.KioskAppPackage != "com.android.chrome" {
		t.Fatalf("expected kiosk app package to round-trip, got %q", rec.KioskAppPackage)
	}
}

type fakePolicyRow struct {
	kioskAppPackage any
}

func (r fakePolicyRow) Scan(dest ...any) error {
	values := []any{
		"id-1",
		"tenant-1",
		"policy-1",
		1,
		true,
		r.kioskAppPackage,
		json.RawMessage(`{"blockPackages":["com.android.chrome"]}`),
		"active",
		time.Unix(123, 0).UTC(),
		pgtype.Timestamptz{},
	}
	for i, d := range dest {
		if err := assignPolicyValue(d, values[i]); err != nil {
			return err
		}
	}
	return nil
}

func assignPolicyValue(dest any, value any) error {
	switch d := dest.(type) {
	case *string:
		if value == nil {
			*d = ""
			return nil
		}
		*d = value.(string)
	case *int:
		*d = value.(int)
	case *bool:
		*d = value.(bool)
	case *pgtype.Text:
		if value == nil {
			*d = pgtype.Text{}
			return nil
		}
		*d = pgtype.Text{String: value.(string), Valid: true}
	case *json.RawMessage:
		if value == nil {
			*d = nil
			return nil
		}
		raw := value.(json.RawMessage)
		*d = append(json.RawMessage(nil), raw...)
	case *time.Time:
		*d = value.(time.Time)
	case *pgtype.Timestamptz:
		*d = value.(pgtype.Timestamptz)
	default:
		return fmt.Errorf("unsupported destination type %T", dest)
	}
	return nil
}

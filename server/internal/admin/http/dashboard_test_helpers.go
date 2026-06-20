package adminhttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/pagination"
)

func assertRedirect(t *testing.T, rr *httptest.ResponseRecorder, wantLocation string) {
	t.Helper()
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != wantLocation {
		t.Fatalf("unexpected redirect location: %q want %q", got, wantLocation)
	}
}

func assertAuditRecord(t *testing.T, store *recordingAuditStore, action, resourceType, resourceID string) {
	t.Helper()
	if len(store.records) != 1 {
		t.Fatalf("expected one audit record, got %#v", store.records)
	}
	record := store.records[0]
	if record.Action != action || record.ResourceType != resourceType || record.ResourceID != resourceID {
		t.Fatalf("unexpected audit record: %#v", record)
	}
}

type recordingAuditStore struct {
	records []audit.Event
}

func (s *recordingAuditStore) Record(_ context.Context, tenantID, actor, action, resourceType, resourceID string, details map[string]any) (audit.Event, error) {
	rec := audit.Event{
		TenantID:     tenantID,
		Actor:        actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
	}
	s.records = append(s.records, rec)
	return rec, nil
}

func (s *recordingAuditStore) List(context.Context, string, pagination.Params) ([]audit.Event, error) {
	return nil, nil
}

func (s *recordingAuditStore) ListNewest(context.Context, string) ([]audit.Event, error) {
	return nil, nil
}

func (s *recordingAuditStore) CountSince(context.Context, string, time.Time) (int, error) {
	return 0, nil
}

type recordingArtifactStore struct{}

func (recordingArtifactStore) Put(context.Context, string, io.Reader, string, int64) error {
	return nil
}

func (recordingArtifactStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, http.ErrBodyNotAllowed
}

func (recordingArtifactStore) Delete(context.Context, string) error { return nil }

func (recordingArtifactStore) HealthCheck(context.Context) error { return nil }

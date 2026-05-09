package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestExitCodeForStatus(t *testing.T) {
	cases := []struct {
		status int
		want   int
	}{
		{http.StatusUnauthorized, 2},
		{http.StatusForbidden, 3},
		{http.StatusNotFound, 4},
		{http.StatusConflict, 5},
		{http.StatusInternalServerError, 6},
		{http.StatusBadGateway, 6},
		{http.StatusBadRequest, 1},
	}
	for _, tc := range cases {
		if got := exitCodeForStatus(tc.status); got != tc.want {
			t.Fatalf("status %d: got %d want %d", tc.status, got, tc.want)
		}
	}
}

func TestHTTPFailureErrorAndTransportFailureErrorMapExitCodes(t *testing.T) {
	httpErr := httpFailureError("request failed", http.StatusNotFound, []byte("missing"))
	var cliErr *cliError
	if !errors.As(httpErr, &cliErr) {
		t.Fatalf("expected cli error, got %T", httpErr)
	}
	if cliErr.code != 4 {
		t.Fatalf("expected 4, got %d", cliErr.code)
	}

	transportErr := transportFailureError("request failed", errors.New("dial tcp"))
	if !errors.As(transportErr, &cliErr) {
		t.Fatalf("expected cli error, got %T", transportErr)
	}
	if cliErr.code != 6 {
		t.Fatalf("expected 6, got %d", cliErr.code)
	}
}

func TestWriteItemsTableAlignsColumns(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"id":"1","name":"short","status":"active","type":"device","deviceId":"abc"}`),
		json.RawMessage(`{"id":"2","name":"a much longer name","status":"retired","type":"device","deviceId":"xyz"}`),
	}
	var buf bytes.Buffer
	if err := writeItemsTable(&buf, items); err != nil {
		t.Fatalf("write items table: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}
	if strings.Contains(buf.String(), "\t") {
		t.Fatalf("expected spaces not tabs:\n%s", buf.String())
	}
	wantLen := len(lines[0])
	for i, line := range lines[1:] {
		if got := len(line); got != wantLen {
			t.Fatalf("line %d length mismatch: got %d want %d\n%s", i+1, got, wantLen, buf.String())
		}
	}
}

package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"xmdm/cli/internal/config"
)

type cliError struct {
	code int
	err  error
}

func (e *cliError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func exitError(code int, err error) error {
	if err == nil {
		return nil
	}
	return &cliError{code: code, err: err}
}

func exitCodeForStatus(status int) int {
	switch status {
	case httpStatusUnauthorized:
		return 2
	case httpStatusForbidden:
		return 3
	case httpStatusNotFound:
		return 4
	case httpStatusConflict:
		return 5
	default:
		if status >= 500 {
			return 6
		}
		return 1
	}
}

func httpFailureError(prefix string, status int, payload []byte) error {
	msg := strings.TrimSpace(http.StatusText(status))
	if len(payload) > 0 {
		msg = strings.TrimSpace(string(payload))
	}
	if msg == "" {
		msg = "request failed"
	}
	return exitError(exitCodeForStatus(status), fmt.Errorf("%s: %s", prefix, msg))
}

func transportFailureError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return exitError(6, fmt.Errorf("%s: %w", prefix, err))
}

const (
	httpStatusUnauthorized = 401
	httpStatusForbidden    = 403
	httpStatusNotFound     = 404
	httpStatusConflict     = 409
)

func (a *app) writeSuccess(w ioWriter, resolved config.Resolved, command string, data any) error {
	if strings.EqualFold(resolved.OutputFormat, "json") {
		return a.writeJSONEnvelope(w, resolved, command, data)
	}
	return a.writeHumanSuccess(w, command, data)
}

func (a *app) writeJSONEnvelope(w ioWriter, resolved config.Resolved, command string, data any) error {
	payload := map[string]any{
		"ok":      true,
		"command": command,
		"data":    data,
		"meta": map[string]any{
			"requestId": requestID(),
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"baseUrl":   resolved.BaseURL,
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func (a *app) writeHumanSuccess(w ioWriter, command string, data any) error {
	switch value := data.(type) {
	case map[string]any:
		return writeHumanMap(w, value, 0)
	case []json.RawMessage:
		return writeItemsTable(w, value)
	case json.RawMessage:
		return writeHumanRaw(w, value, 0)
	default:
		_, err := fmt.Fprintln(w, command)
		return err
	}
}

func writeHumanMap(w ioWriter, payload map[string]any, indent int) error {
	for _, key := range orderedKeys(payload) {
		if err := writeHumanEntry(w, key, payload[key], indent); err != nil {
			return err
		}
	}
	return nil
}

func writeHumanEntry(w ioWriter, key string, value any, indent int) error {
	prefix := strings.Repeat(" ", indent)
	switch typed := value.(type) {
	case nil:
		_, err := fmt.Fprintf(w, "%s%s: null\n", prefix, key)
		return err
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(typed, &decoded); err != nil {
			return err
		}
		return writeHumanEntry(w, key, decoded, indent)
	case []byte:
		return writeHumanEntry(w, key, json.RawMessage(typed), indent)
	case map[string]any:
		if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
			return err
		}
		return writeHumanMap(w, typed, indent+2)
	case []any:
		if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
			return err
		}
		if len(typed) == 0 {
			_, err := fmt.Fprintf(w, "%s  []\n", prefix)
			return err
		}
		return writeHumanSlice(w, typed, indent+2)
	case []json.RawMessage:
		if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
			return err
		}
		if len(typed) == 0 {
			_, err := fmt.Fprintf(w, "%s  []\n", prefix)
			return err
		}
		return writeHumanRawSlice(w, typed, indent+2)
	default:
		_, err := fmt.Fprintf(w, "%s%s: %v\n", prefix, key, typed)
		return err
	}
}

func writeHumanRaw(w ioWriter, raw json.RawMessage, indent int) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return writeHumanValue(w, value, indent)
}

func writeHumanValue(w ioWriter, value any, indent int) error {
	prefix := strings.Repeat(" ", indent)
	switch typed := value.(type) {
	case nil:
		_, err := fmt.Fprintf(w, "%snull\n", prefix)
		return err
	case map[string]any:
		return writeHumanMap(w, typed, indent)
	case []any:
		if len(typed) == 0 {
			_, err := fmt.Fprintf(w, "%s[]\n", prefix)
			return err
		}
		return writeHumanSlice(w, typed, indent)
	case []json.RawMessage:
		if len(typed) == 0 {
			_, err := fmt.Fprintf(w, "%s[]\n", prefix)
			return err
		}
		return writeHumanRawSlice(w, typed, indent)
	default:
		_, err := fmt.Fprintf(w, "%s%v\n", prefix, typed)
		return err
	}
}

func writeHumanSlice(w ioWriter, items []any, indent int) error {
	for _, item := range items {
		if err := writeHumanListItem(w, item, indent); err != nil {
			return err
		}
	}
	return nil
}

func writeHumanRawSlice(w ioWriter, items []json.RawMessage, indent int) error {
	for _, raw := range items {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		if err := writeHumanListItem(w, value, indent); err != nil {
			return err
		}
	}
	return nil
}

func writeHumanListItem(w ioWriter, value any, indent int) error {
	prefix := strings.Repeat(" ", indent)
	switch typed := value.(type) {
	case map[string]any:
		keys := orderedKeys(typed)
		if len(keys) == 0 {
			_, err := fmt.Fprintf(w, "%s- {}\n", prefix)
			return err
		}
		firstKey := keys[0]
		firstValue := typed[firstKey]
		if isHumanScalar(firstValue) {
			if _, err := fmt.Fprintf(w, "%s- %s: %v\n", prefix, firstKey, firstValue); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "%s- %s:\n", prefix, firstKey); err != nil {
				return err
			}
			if err := writeHumanValue(w, firstValue, indent+2); err != nil {
				return err
			}
		}
		for _, key := range keys[1:] {
			if err := writeHumanEntry(w, key, typed[key], indent+2); err != nil {
				return err
			}
		}
		return nil
	case []any:
		if _, err := fmt.Fprintf(w, "%s- []\n", prefix); err != nil {
			return err
		}
		return writeHumanSlice(w, typed, indent+2)
	default:
		_, err := fmt.Fprintf(w, "%s- %v\n", prefix, typed)
		return err
	}
}

func writeItemsTable(w ioWriter, items []json.RawMessage) error {
	columns := []string{"id", "name", "status", "type", "deviceId"}
	rows := make([]map[string]string, 0, len(items))
	widths := map[string]int{}
	for _, col := range columns {
		widths[col] = len(col)
	}
	for _, raw := range items {
		row := summarizeItem(raw)
		rows = append(rows, row)
		for _, col := range columns {
			if n := len(row[col]); n > widths[col] {
				widths[col] = n
			}
		}
	}
	if _, err := fmt.Fprintln(w, formatTableRow(columns, widths)); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(w, formatTableRow(row, widths)); err != nil {
			return err
		}
	}
	return nil
}

func formatTableRow(values any, widths map[string]int) string {
	columns := []string{"id", "name", "status", "type", "deviceId"}
	fields := make([]string, 0, len(columns))
	switch typed := values.(type) {
	case []string:
		for i, col := range columns {
			fields = append(fields, padTableCell(typed[i], widths[col]))
		}
	case map[string]string:
		for _, col := range columns {
			fields = append(fields, padTableCell(typed[col], widths[col]))
		}
	default:
		return ""
	}
	return strings.Join(fields, "  ")
}

func padTableCell(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func summarizeItem(raw json.RawMessage) map[string]string {
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	return map[string]string{
		"id":       stringField(payload, "id"),
		"name":     firstNonEmpty(stringField(payload, "name"), stringField(payload, "email"), stringField(payload, "packageName"), stringField(payload, "path"), stringField(payload, "message"), stringField(payload, "versionName")),
		"status":   stringField(payload, "status"),
		"type":     firstNonEmpty(stringField(payload, "type"), stringField(payload, "resourceType"), stringField(payload, "action")),
		"deviceId": stringField(payload, "deviceId"),
	}
}

func stringField(payload map[string]any, key string) string {
	if value, ok := payload[key]; ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func orderedKeys(payload map[string]any) []string {
	keys := []string{
		"id",
		"name",
		"status",
		"type",
		"email",
		"packageName",
		"path",
		"message",
		"versionName",
		"deviceId",
		"tenantId",
		"createdAt",
		"updatedAt",
		"deletedAt",
		"resourceType",
		"resourceId",
		"action",
		"actor",
		"details",
		"payload",
		"configPath",
		"profile",
		"baseUrl",
		"authMode",
		"outputFormat",
		"timeout",
		"user",
		"device",
		"deviceInfo",
		"commands",
		"logs",
		"audit",
		"item",
		"items",
	}
	out := make([]string, 0, len(payload))
	for _, key := range keys {
		if _, ok := payload[key]; ok {
			out = append(out, key)
		}
	}
	var rest []string
	for key := range payload {
		if !containsString(out, key) {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	out = append(out, rest...)
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func isHumanScalar(value any) bool {
	switch value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		json.Number:
		return true
	default:
		return false
	}
}

func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

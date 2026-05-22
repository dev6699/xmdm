package managedfiles

import (
	"encoding/json"
	"strings"
)

func RenderTemplate(content []byte, deviceID string, bootstrapExtras map[string]any) []byte {
	values := TemplateValues(deviceID, bootstrapExtras)
	pairs := make([]string, 0, len(values)*2)
	for key, value := range values {
		pairs = append(pairs, key, value)
	}
	if len(pairs) == 0 {
		return append([]byte(nil), content...)
	}
	rendered := strings.NewReplacer(pairs...).Replace(string(content))
	return []byte(rendered)
}

func TemplateValues(deviceID string, bootstrapExtras map[string]any) map[string]string {
	values := map[string]string{
		"DEVICE_NUMBER": deviceID,
		"DEVICE_ID":     deviceID,
		"IMEI":          "",
	}
	for key, value := range bootstrapExtras {
		rendered := templateValue(value)
		if rendered == "" {
			continue
		}
		for _, candidate := range normalizedTemplateKeys(key) {
			values[candidate] = rendered
		}
	}
	return values
}

func normalizedTemplateKeys(key string) []string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	keys := make([]string, 0, 4)
	add := func(value string) {
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		keys = append(keys, value)
	}
	add(trimmed)
	add(strings.ToUpper(trimmed))
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "com.xmdm.") {
		stripped := strings.TrimSpace(trimmed[len("com.xmdm."):])
		add(stripped)
		add(strings.ToUpper(stripped))
		lower = strings.ToLower(stripped)
	}
	switch lower {
	case "secondarybaseurl", "secondary_base_url":
		add("SECONDARY_BASE_URL")
	}
	return keys
}

func templateValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []string:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		return strings.Join(parts, ",")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if rendered := templateValue(item); rendered != "" {
				parts = append(parts, rendered)
			}
		}
		return strings.Join(parts, ",")
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
}

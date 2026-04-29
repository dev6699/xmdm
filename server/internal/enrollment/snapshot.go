package enrollment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
)

func NewBootstrapConfigSnapshot(deviceID, deviceIDUse string, policy PolicySnapshot, apps []AppSnapshot, files []ManagedFileSnapshot, certificates []CertificateSnapshot) ConfigSnapshot {
	if apps == nil {
		apps = []AppSnapshot{}
	}
	if files == nil {
		files = []ManagedFileSnapshot{}
	}
	if certificates == nil {
		certificates = []CertificateSnapshot{}
	}
	snapshot := ConfigSnapshot{
		Device: DeviceSnapshot{
			DeviceID:    deviceID,
			DeviceIDUse: deviceIDUse,
		},
		Policy:       policy,
		Apps:         apps,
		Files:        files,
		Certificates: certificates,
	}
	snapshot.Version = snapshotRevision(snapshot)
	return snapshot
}

func SignConfigSnapshot(snapshot ConfigSnapshot, secret string) (ConfigSnapshot, error) {
	if secret == "" {
		return ConfigSnapshot{}, errors.New("invalid config snapshot secret")
	}
	rawSignature, err := expectedConfigSignature(snapshot, secret)
	if err != nil {
		return ConfigSnapshot{}, err
	}
	snapshot.Signature = base64.RawURLEncoding.EncodeToString(rawSignature)
	return snapshot, nil
}

func VerifyConfigSnapshot(snapshot ConfigSnapshot, secret string) error {
	if secret == "" {
		return errors.New("invalid config snapshot secret")
	}
	if snapshot.Signature == "" {
		return errors.New("missing config snapshot signature")
	}
	rawSignature, err := expectedConfigSignature(snapshot, secret)
	if err != nil {
		return err
	}
	actualSignature, err := base64.RawURLEncoding.DecodeString(snapshot.Signature)
	if err != nil {
		return errors.New("invalid config snapshot signature")
	}
	if !hmac.Equal(rawSignature, actualSignature) {
		return errors.New("invalid config snapshot signature")
	}
	return nil
}

func expectedConfigSignature(snapshot ConfigSnapshot, secret string) ([]byte, error) {
	snapshot.Signature = ""
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	canonical, err := canonicalJSON(normalized)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return mac.Sum(nil), nil
}

func snapshotRevision(snapshot ConfigSnapshot) string {
	snapshot.Version = ""
	snapshot.Signature = ""
	snapshot.Device = DeviceSnapshot{}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "0"
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return "0"
	}
	canonical, err := canonicalJSON(normalized)
	if err != nil {
		return "0"
	}
	sum := sha256.Sum256([]byte(canonical))
	revision := int64(binary.BigEndian.Uint64(sum[:8]))
	return strconv.FormatInt(revision, 10)
}

func canonicalJSON(value any) (string, error) {
	var out strings.Builder
	if err := appendCanonicalJSON(&out, value); err != nil {
		return "", err
	}
	return out.String(), nil
}

func appendCanonicalJSON(out *strings.Builder, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		out.Write(data)
	case json.Number:
		out.WriteString(v.String())
	case float64:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		out.Write(data)
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := appendCanonicalJSON(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		out.WriteByte('{')
		keys := canonicalObjectKeys(v)
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			data, err := json.Marshal(key)
			if err != nil {
				return err
			}
			out.Write(data)
			out.WriteByte(':')
			if err := appendCanonicalJSON(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		out.Write(data)
	}
	return nil
}

func canonicalObjectKeys(value map[string]any) []string {
	ordered := make([]string, 0, len(snapshotFieldOrder))
	for _, key := range snapshotFieldOrder {
		if _, ok := value[key]; ok {
			ordered = append(ordered, key)
		}
	}
	if len(ordered) > 0 && len(ordered) == len(value) {
		return ordered
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var snapshotFieldOrder = []string{
	"version",
	"device",
	"policy",
	"apps",
	"files",
	"certificates",
	"signature",
}

package enrollment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
)

func NewBootstrapConfigSnapshot(deviceID, deviceIDUse string, bootstrapExtras map[string]any) ConfigSnapshot {
	policy := map[string]any{}
	if len(bootstrapExtras) > 0 {
		policy["bootstrapExtras"] = bootstrapExtras
	}
	return ConfigSnapshot{
		Version: "1",
		Device: map[string]any{
			"deviceId":    deviceID,
			"deviceIdUse": deviceIDUse,
		},
		Policy:       policy,
		Apps:         []any{},
		Files:        []any{},
		Certificates: []any{},
		Commands:     []any{},
	}
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
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	return mac.Sum(nil), nil
}

package checksum

import (
	"crypto/sha256"
	"encoding/base64"
)

func SHA256Base64URL(content []byte) string {
	sum := sha256.Sum256(content)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

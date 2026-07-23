package security

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func RandomBytes(buf []byte) (int, error) { return rand.Read(buf) }

func RandomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

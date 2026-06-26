package serviceiamsession

import (
	"crypto/rand"
	"encoding/hex"
)

const sessionIDBytes = 32

// NewSessionID returns an opaque random session identifier.
func NewSessionID() (string, error) {
	buf := make([]byte, sessionIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

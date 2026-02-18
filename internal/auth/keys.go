package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func NewAPIKey() (plain string, prefix string, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate key bytes: %w", err)
	}
	token := hex.EncodeToString(buf)
	prefix = token[:8]
	plain = "dooh_" + token
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return plain, prefix, hash, nil
}

func HashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plain)))
	return hex.EncodeToString(sum[:])
}

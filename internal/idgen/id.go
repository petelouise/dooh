package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

func ULIDLike() (string, error) {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	binary.BigEndian.PutUint64(b[:8], ms)
	if _, err := rand.Read(b[8:]); err != nil {
		return "", fmt.Errorf("random bytes: %w", err)
	}
	return strings.ToLower(enc.EncodeToString(b[:])), nil
}

func Short(prefix string) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	v := strings.ToUpper(enc.EncodeToString(b))
	if len(v) > 6 {
		v = v[:6]
	}
	return prefix + "_" + v, nil
}

package core

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

func NewID(prefix string) string {
	token := randomBytes(16)
	return prefix + "_" + hex.EncodeToString(token)
}

func NewToken() string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(32))
}

func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password must contain at least 8 characters")
	}

	salt := randomBytes(16)
	sum := passwordDigest(password, salt)
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(sum[:]), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}

	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}

	expected, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	sum := passwordDigest(password, salt)
	return subtle.ConstantTimeCompare(sum[:], expected) == 1
}

func passwordDigest(password string, salt []byte) [32]byte {
	payload := append([]byte(password), salt...)
	sum := sha256.Sum256(payload)
	for i := 0; i < 120_000; i++ {
		next := append(sum[:], salt...)
		sum = sha256.Sum256(next)
	}
	return sum
}

func randomBytes(size int) []byte {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return buf
}

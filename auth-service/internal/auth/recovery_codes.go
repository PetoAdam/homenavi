package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const recoveryCodeCount = 10

func GenerateRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		codeBytes := make([]byte, 12)
		if _, err := rand.Read(codeBytes); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		code := base64.RawURLEncoding.EncodeToString(codeBytes)
		digest := sha256.Sum256([]byte(code))
		codes = append(codes, code)
		hashes = append(hashes, hex.EncodeToString(digest[:]))
	}
	return codes, hashes, nil
}

func RecoveryCodeHash(code string) string {
	digest := sha256.Sum256([]byte(code))
	return hex.EncodeToString(digest[:])
}

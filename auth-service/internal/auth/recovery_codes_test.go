package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestGenerateRecoveryCodes(t *testing.T) {
	codes, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	if len(codes) != recoveryCodeCount || len(hashes) != recoveryCodeCount {
		t.Fatalf("generated %d codes and %d hashes, want %d", len(codes), len(hashes), recoveryCodeCount)
	}
	seen := make(map[string]struct{}, recoveryCodeCount)
	for index, code := range codes {
		if _, exists := seen[code]; exists {
			t.Fatal("generated duplicate recovery code")
		}
		seen[code] = struct{}{}
		digest := sha256.Sum256([]byte(code))
		if hashes[index] != hex.EncodeToString(digest[:]) {
			t.Fatal("recovery code hash does not match plaintext code")
		}
	}
}

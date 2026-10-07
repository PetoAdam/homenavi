package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestTOTPSecretEncryption(t *testing.T) {
	service := NewService(Config{TOTPEncryptionKey: []byte("01234567890123456789012345678901")}, nil)
	ciphertext, err := service.EncryptTOTPSecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ciphertext == "JBSWY3DPEHPK3PXP" {
		t.Fatal("secret was not encrypted")
	}
	secondCiphertext, err := service.EncryptTOTPSecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encrypt again: %v", err)
	}
	if ciphertext == secondCiphertext {
		t.Fatal("ciphertexts must use distinct nonces")
	}
	plaintext, err := service.DecryptTOTPSecret(ciphertext)
	if err != nil || plaintext != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, encryptedTOTPSecretPrefix))
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	payload[len(payload)-1] ^= 1
	tampered := encryptedTOTPSecretPrefix + base64.RawStdEncoding.EncodeToString(payload)
	if _, err := service.DecryptTOTPSecret(tampered); err == nil {
		t.Fatal("expected tampered ciphertext to fail")
	}
	wrongKeyService := NewService(Config{TOTPEncryptionKey: []byte("abcdefghijklmnopqrstuvwxyz123456")}, nil)
	if _, err := wrongKeyService.DecryptTOTPSecret(ciphertext); err == nil {
		t.Fatal("expected ciphertext encrypted with a different key to fail")
	}
	legacy, err := service.DecryptTOTPSecret("JBSWY3DPEHPK3PXP")
	if err != nil || legacy != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("legacy secret = %q, %v", legacy, err)
	}
}

func TestTOTPSecretEncryptionRejectsInvalidInputs(t *testing.T) {
	service := NewService(Config{TOTPEncryptionKey: []byte("01234567890123456789012345678901")}, nil)
	if _, err := service.EncryptTOTPSecret(""); err == nil {
		t.Fatal("expected empty secret to be rejected")
	}
	for _, value := range []string{
		encryptedTOTPSecretPrefix + "not-base64!",
		encryptedTOTPSecretPrefix + "AA",
	} {
		if _, err := service.DecryptTOTPSecret(value); err == nil {
			t.Fatalf("expected malformed encrypted value %q to be rejected", value)
		}
	}
	invalidKeyService := NewService(Config{TOTPEncryptionKey: []byte("short")}, nil)
	if _, err := invalidKeyService.EncryptTOTPSecret("JBSWY3DPEHPK3PXP"); err == nil {
		t.Fatal("expected invalid AES key size to be rejected")
	}
}

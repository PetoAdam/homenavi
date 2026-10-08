package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const encryptedTOTPSecretPrefix = "v1:"

func (s *Service) EncryptTOTPSecret(secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("TOTP secret is required")
	}
	block, err := aes.NewCipher(s.config.TOTPEncryptionKey)
	if err != nil {
		return "", fmt.Errorf("create TOTP cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create TOTP GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate TOTP nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)
	return encryptedTOTPSecretPrefix + base64.RawStdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (s *Service) DecryptTOTPSecret(value string) (string, error) {
	if !strings.HasPrefix(value, encryptedTOTPSecretPrefix) {
		return value, nil
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encryptedTOTPSecretPrefix))
	if err != nil {
		return "", fmt.Errorf("decode TOTP secret: %w", err)
	}
	block, err := aes.NewCipher(s.config.TOTPEncryptionKey)
	if err != nil {
		return "", fmt.Errorf("create TOTP cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create TOTP GCM: %w", err)
	}
	if len(payload) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted TOTP secret is too short")
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt TOTP secret: %w", err)
	}
	return string(plaintext), nil
}

package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

func TestKeyRingSignsTokensAndPublishesMatchingJWKS(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ring := NewKeyRing("key-2026-01", privateKey)
	expectedKeyID, err := authx.KeyIDForRSAPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("derive key ID: %v", err)
	}

	tokenString, err := ring.Sign(jwt.MapClaims{"sub": "user-1"})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	parsed, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse token header: %v", err)
	}
	if got := parsed.Header["kid"]; got != expectedKeyID {
		t.Fatalf("kid = %v, want %s", got, expectedKeyID)
	}

	keySet, err := ring.JSONWebKeySet()
	if err != nil {
		t.Fatalf("create JWKS: %v", err)
	}
	if len(keySet.Keys) != 1 {
		t.Fatalf("key count = %d, want 1", len(keySet.Keys))
	}
	key := keySet.Keys[0]
	if key.KeyType != "RSA" || key.Use != "sig" || key.Algorithm != jwt.SigningMethodRS256.Alg() || key.KeyID != expectedKeyID {
		t.Fatalf("unexpected JWK metadata: %#v", key)
	}
	modulus, err := base64.RawURLEncoding.DecodeString(key.Modulus)
	if err != nil {
		t.Fatalf("decode modulus: %v", err)
	}
	exponent, err := base64.RawURLEncoding.DecodeString(key.Exponent)
	if err != nil {
		t.Fatalf("decode exponent: %v", err)
	}
	if new(big.Int).SetBytes(modulus).Cmp(privateKey.PublicKey.N) != 0 {
		t.Fatal("JWKS modulus does not match signing key")
	}
	if new(big.Int).SetBytes(exponent).Int64() != int64(privateKey.PublicKey.E) {
		t.Fatal("JWKS exponent does not match signing key")
	}
}

func TestKeyRingDerivesStableKeyID(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ring := NewKeyRing("", privateKey)

	firstKeySet, err := ring.JSONWebKeySet()
	if err != nil {
		t.Fatalf("create first JWKS: %v", err)
	}
	secondKeySet, err := ring.JSONWebKeySet()
	if err != nil {
		t.Fatalf("create second JWKS: %v", err)
	}
	if firstKeySet.Keys[0].KeyID == "" || firstKeySet.Keys[0].KeyID != secondKeySet.Keys[0].KeyID {
		t.Fatalf("expected stable derived key ID, got %q and %q", firstKeySet.Keys[0].KeyID, secondKeySet.Keys[0].KeyID)
	}
}

func TestKeyRingPublishesOverlapVerificationKeys(t *testing.T) {
	activeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate active key: %v", err)
	}
	overlapKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate overlap key: %v", err)
	}
	ring := NewKeyRing("active-key", activeKey, &overlapKey.PublicKey, &activeKey.PublicKey)

	keySet, err := ring.JSONWebKeySet()
	if err != nil {
		t.Fatalf("create JWKS: %v", err)
	}
	if len(keySet.Keys) != 2 {
		t.Fatalf("key count = %d, want 2", len(keySet.Keys))
	}
	activeKeyID, err := authx.KeyIDForRSAPublicKey(&activeKey.PublicKey)
	if err != nil {
		t.Fatalf("derive active key ID: %v", err)
	}
	if keySet.Keys[0].KeyID != activeKeyID {
		t.Fatalf("active key ID = %q, want %q", keySet.Keys[0].KeyID, activeKeyID)
	}
	overlapKeyID, err := authx.KeyIDForRSAPublicKey(&overlapKey.PublicKey)
	if err != nil {
		t.Fatalf("derive overlap key ID: %v", err)
	}
	if keySet.Keys[1].KeyID != overlapKeyID {
		t.Fatalf("overlap key ID = %q, want %q", keySet.Keys[1].KeyID, overlapKeyID)
	}
}

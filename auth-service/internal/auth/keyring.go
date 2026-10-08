package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"sort"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

// JSONWebKeySet is the public key discovery document served to token consumers.
type JSONWebKeySet struct {
	Keys []JSONWebKey `json:"keys"`
}

// JSONWebKey represents the public parameters for an RSA signing key.
type JSONWebKey struct {
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

// KeyRing owns the active signing key and the overlap verification keys.
type KeyRing struct {
	privateKey             *rsa.PrivateKey
	verificationPublicKeys []*rsa.PublicKey
}

func NewKeyRing(keyID string, privateKey *rsa.PrivateKey, verificationPublicKeys ...*rsa.PublicKey) *KeyRing {
	_ = keyID
	return &KeyRing{
		privateKey:             privateKey,
		verificationPublicKeys: verificationPublicKeys,
	}
}

// Sign creates an RS256 token with the active key identifier.
func (r *KeyRing) Sign(claims jwt.Claims) (string, error) {
	if r == nil || r.privateKey == nil {
		return "", fmt.Errorf("active signing key is not configured")
	}
	keyID, err := r.activeKeyID()
	if err != nil {
		return "", err
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	return token.SignedString(r.privateKey)
}

// JSONWebKeySet returns public active and overlap verification keys.
func (r *KeyRing) JSONWebKeySet() (JSONWebKeySet, error) {
	if r == nil || r.privateKey == nil {
		return JSONWebKeySet{}, fmt.Errorf("active signing key is not configured")
	}
	keyID, err := r.activeKeyID()
	if err != nil {
		return JSONWebKeySet{}, err
	}
	keys := []JSONWebKey{newJSONWebKey(keyID, &r.privateKey.PublicKey)}
	activeKeyFingerprint, err := authx.KeyIDForRSAPublicKey(&r.privateKey.PublicKey)
	if err != nil {
		return JSONWebKeySet{}, err
	}
	seenKeyFingerprints := map[string]struct{}{activeKeyFingerprint: {}}
	for _, publicKey := range r.verificationPublicKeys {
		overlapKeyID, err := authx.KeyIDForRSAPublicKey(publicKey)
		if err != nil {
			return JSONWebKeySet{}, err
		}
		if _, seen := seenKeyFingerprints[overlapKeyID]; seen {
			continue
		}
		seenKeyFingerprints[overlapKeyID] = struct{}{}
		keys = append(keys, newJSONWebKey(overlapKeyID, publicKey))
	}
	sort.Slice(keys[1:], func(left, right int) bool {
		return keys[left+1].KeyID < keys[right+1].KeyID
	})
	return JSONWebKeySet{Keys: keys}, nil
}

func (r *KeyRing) activeKeyID() (string, error) {
	if r.privateKey == nil {
		return "", fmt.Errorf("active signing key is not configured")
	}
	return authx.KeyIDForRSAPublicKey(&r.privateKey.PublicKey)
}

func (r *KeyRing) PublicKeyForID(keyID string) (*rsa.PublicKey, error) {
	if r == nil || r.privateKey == nil {
		return nil, fmt.Errorf("active signing key is not configured")
	}
	activeKeyID, err := r.activeKeyID()
	if err != nil {
		return nil, err
	}
	if keyID == activeKeyID {
		return &r.privateKey.PublicKey, nil
	}
	for _, publicKey := range r.verificationPublicKeys {
		overlapKeyID, err := authx.KeyIDForRSAPublicKey(publicKey)
		if err != nil {
			return nil, err
		}
		if keyID == overlapKeyID {
			return publicKey, nil
		}
	}
	return nil, fmt.Errorf("unknown JWT key ID")
}

func newJSONWebKey(keyID string, publicKey *rsa.PublicKey) JSONWebKey {
	return JSONWebKey{
		KeyType:   "RSA",
		Use:       "sig",
		Algorithm: jwt.SigningMethodRS256.Alg(),
		KeyID:     keyID,
		Modulus:   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
	}
}

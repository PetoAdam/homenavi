package oauth

import (
	"encoding/json"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
)

type jwksProvider interface {
	JSONWebKeySet() (authdomain.JSONWebKeySet, error)
}

// JWKSHandler serves public signing keys for JWT verification.
type JWKSHandler struct {
	provider jwksProvider
}

func NewJWKSHandler(provider jwksProvider) *JWKSHandler {
	return &JWKSHandler{provider: provider}
}

func (h *JWKSHandler) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	keySet, err := h.provider.JSONWebKeySet()
	if err != nil {
		http.Error(w, "signing keys unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(keySet); err != nil {
		http.Error(w, "failed to encode signing keys", http.StatusInternalServerError)
	}
}

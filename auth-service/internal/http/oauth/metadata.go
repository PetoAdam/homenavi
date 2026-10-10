package oauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// MetadataHandler publishes the currently supported authorization-server metadata.
type MetadataHandler struct {
	issuer string
}

func NewMetadataHandler(issuer string) *MetadataHandler {
	return &MetadataHandler{issuer: strings.TrimRight(issuer, "/")}
}

func (h *MetadataHandler) HandleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	issuer, err := h.issuerForRequest(r)
	if err != nil {
		http.Error(w, "invalid public authorization server", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"jwks_uri":                              issuer + "/oauth/jwks.json",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "urn:ietf:params:oauth:grant-type:token-exchange"},
		"scopes_supported":                      []string{"home.devices.read", "home.devices.write", "home.inventory.read", "home.inventory.write", "home.history.read", "home.automation.read", "home.automation.execute"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"registration_endpoint":                 issuer + "/oauth/register",
	})
}

func (h *MetadataHandler) issuerForRequest(request *http.Request) (string, error) {
	issuer := h.issuer
	if issuer == "" {
		origin, err := publicOriginForRequest(request)
		if err != nil {
			return "", err
		}
		issuer = origin + "/api/auth"
	}
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "/api/auth" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("authorization-server issuer must be an absolute /api/auth URI")
	}
	return parsed.String(), nil
}

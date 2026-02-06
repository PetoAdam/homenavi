package backend

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SecretSpec struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
}

type SecretsAPI struct {
	Store   *SecretStore
	Specs   []SecretSpec
	Admin   *AdminAuth
	Allowed map[string]SecretSpec
}

func NewSecretsAPI(store *SecretStore, specs []SecretSpec, admin *AdminAuth) *SecretsAPI {
	allowed := map[string]SecretSpec{}
	for _, spec := range specs {
		key := strings.TrimSpace(spec.Key)
		if key == "" {
			continue
		}
		allowed[key] = spec
	}
	return &SecretsAPI{Store: store, Specs: specs, Admin: admin, Allowed: allowed}
}

func (s *SecretsAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/secrets", s.handleSecrets)
}

func (s *SecretsAPI) handleSecrets(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Admin == nil || !s.Admin.RequireAdmin(w, r) {
		return
	}
	if len(s.Allowed) == 0 {
		writeJSONError(w, http.StatusBadRequest, "integration does not declare secrets")
		return
	}
	if r.Method == http.MethodGet {
		status, err := s.Store.Status(s.Allowed)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"secrets": status})
		return
	}
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	filtered := map[string]string{}
	for key, value := range payload.Secrets {
		if _, ok := s.Allowed[key]; ok {
			filtered[key] = value
		}
	}
	if err := s.Store.Set(filtered); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func ParseSecretSpecs(manifestJSON []byte) []SecretSpec {
	var payload struct {
		Secrets []json.RawMessage `json:"secrets"`
	}
	if err := json.Unmarshal(manifestJSON, &payload); err != nil {
		return nil
	}
	out := make([]SecretSpec, 0, len(payload.Secrets))
	seen := map[string]struct{}{}
	for _, raw := range payload.Secrets {
		var keyOnly string
		if err := json.Unmarshal(raw, &keyOnly); err == nil {
			key := strings.TrimSpace(keyOnly)
			if key != "" {
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					out = append(out, SecretSpec{Key: key})
				}
			}
			continue
		}
		var spec SecretSpec
		if err := json.Unmarshal(raw, &spec); err == nil {
			key := strings.TrimSpace(spec.Key)
			if key != "" {
				spec.Key = key
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					out = append(out, spec)
				}
			}
		}
	}
	return out
}

type SecretStore struct {
	path string
	mu   sync.Mutex
}

func NewSecretStore(path string) *SecretStore {
	return &SecretStore{path: path}
}

func DefaultSecretsPath() string {
	if v := strings.TrimSpace(os.Getenv("INTEGRATION_SECRETS_PATH")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("INTEGRATIONS_SECRETS_PATH")); v != "" {
		return v
	}
	return filepath.Join("config", "integration.secrets.json")
}

func (s *SecretStore) Status(allowed map[string]SecretSpec) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for key := range allowed {
		_, has := current[key]
		out[key] = has
	}
	return out, nil
}

func (s *SecretStore) Set(values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	for key, value := range values {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(value)
		if k == "" || v == "" {
			continue
		}
		current[k] = v
	}
	return s.saveUnlocked(current)
}

func (s *SecretStore) loadUnlocked() (map[string]string, error) {
	if strings.TrimSpace(s.path) == "" {
		return map[string]string{}, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var payload map[string]string
	if err := json.Unmarshal(data, &payload); err != nil {
		return map[string]string{}, nil
	}
	if payload == nil {
		payload = map[string]string{}
	}
	return payload, nil
}

func (s *SecretStore) saveUnlocked(values map[string]string) error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

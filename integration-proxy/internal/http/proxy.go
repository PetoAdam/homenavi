package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/integrations/") {
		p = strings.TrimPrefix(p, "/integrations/")
	} else if strings.HasPrefix(p, "/integrations") {
		p = strings.TrimPrefix(p, "/integrations")
		p = strings.TrimPrefix(p, "/")
	}
	if p == "" || p == "/" {
		http.NotFound(w, r)
		return
	}

	parts := strings.SplitN(p, "/", 2)
	id := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}

	s.mu.RLock()
	proxy := s.proxies[id]
	s.mu.RUnlock()
	if proxy == nil {
		http.NotFound(w, r)
		return
	}

	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + rest
	if strings.HasSuffix(r.URL.Path, "/") && !strings.HasSuffix(r2.URL.Path, "/") {
		r2.URL.Path += "/"
	}
	r2.URL.RawPath = ""
	r2.Host = r.Host

	proxy.ServeHTTP(w, r2)
}

func (s *Server) refreshManifestFromUpstream(ctx context.Context, id string) {
	s.mu.RLock()
	up := s.upstreams[id]
	s.mu.RUnlock()
	if up == nil {
		return
	}
	_ = s.refreshManifest(ctx, id, up)
}

func (s *Server) refreshManifest(ctx context.Context, id string, upstream *url.URL) error {
	mURL := *upstream
	mURL.Path = path.Join(mURL.Path, "/.well-known/homenavi-integration.json")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, mURL.String(), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		s.setManifestErr(id, err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		s.setManifestErr(id, fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b))))
		return fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		s.setManifestErr(id, err.Error())
		return err
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		s.setManifestErr(id, "invalid json")
		return err
	}
	if s.validator != nil {
		if err := s.validator.Validate(raw); err != nil {
			s.setManifestErr(id, "schema validation failed")
			return err
		}
	}

	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		s.setManifestErr(id, "invalid manifest")
		return err
	}
	m.ID = id

	s.mu.Lock()
	s.manifests[id] = m
	delete(s.manifestErr, id)
	state := s.updates[id]
	state.ID = id
	if strings.TrimSpace(state.InstalledVersion) == "" {
		state.InstalledVersion = strings.TrimSpace(m.Version)
	}
	state.InProgress = s.updating[id]
	s.updates[id] = state
	s.mu.Unlock()
	return nil
}

func (s *Server) setManifestErr(id, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifestErr[id] = msg
}

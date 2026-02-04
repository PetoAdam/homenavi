package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/homenavi/spotify-integration/internal/ratelimit"
	"github.com/homenavi/spotify-integration/internal/security"
	"github.com/homenavi/spotify-integration/src/backend"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8099"
	}

	manifestPath := os.Getenv("MANIFEST")
	if manifestPath == "" {
		manifestPath = "manifest/homenavi-integration.json"
	}
	manifestJSON, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("read manifest: %v", err)
	}
	secretSpecs := backend.ParseSecretSpecs(manifestJSON)
	secretStore := backend.NewSecretStore(backend.DefaultSecretsPath())
	adminAuth, err := backend.NewAdminAuthFromEnv()
	if err != nil {
		log.Fatalf("load admin auth: %v", err)
	}

	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "web"
	}
	webDir = filepath.Clean(webDir)
	webFS := os.DirFS(webDir)
	if _, err := fs.Stat(webFS, "."); err != nil {
		log.Fatalf("web dir error: %v", err)
	}

	spotifyClient, err := backend.NewSpotifyClientFromEnv()
	if err != nil {
		log.Printf("spotify config missing: %v", err)
		spotifyClient = nil
	}

	s := &backend.Server{
		WebFS:        webFS,
		ManifestJSON: manifestJSON,
		Spotify:      spotifyClient,
		Playback:     backend.NewPlaybackCache(),
		SecretStore:  secretStore,
		SecretSpecs:  secretSpecs,
		AdminAuth:    adminAuth,
	}
	h := s.Routes()

	h = ratelimit.NewIPRateLimiter(10, 20)(h)
	h = security.SecurityHeaders(h)

	addr := ":" + port
	log.Printf("spotify integration listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

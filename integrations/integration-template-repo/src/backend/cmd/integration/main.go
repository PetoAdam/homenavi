package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/homenavi/integration-template/internal/ratelimit"
	"github.com/homenavi/integration-template/internal/security"
	"github.com/homenavi/integration-template/src/backend"
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
	manifestPath = filepath.Clean(manifestPath)
	manifestJSON, err := os.ReadFile(manifestPath) // #nosec G304 -- path comes from env/config
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

	s := &backend.Server{
		WebFS:        webFS,
		ManifestJSON: manifestJSON,
		SecretStore:  secretStore,
		SecretSpecs:  secretSpecs,
		AdminAuth:    adminAuth,
	}
	h := s.Routes()

	h = ratelimit.NewIPRateLimiter(10, 20)(h)
	h = security.SecurityHeaders(h)

	addr := ":" + port
	log.Printf("integration listening on %s", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

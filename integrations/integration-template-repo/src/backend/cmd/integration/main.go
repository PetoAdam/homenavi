package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

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
	manifestJSON, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("read manifest: %v", err)
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

	s := &backend.Server{WebFS: webFS, ManifestJSON: manifestJSON}
	h := s.Routes()

	h = ratelimit.NewIPRateLimiter(10, 20)(h)
	h = security.SecurityHeaders(h)

	addr := ":" + port
	log.Printf("integration listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

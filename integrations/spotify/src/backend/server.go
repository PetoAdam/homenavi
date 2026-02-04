package backend

import (
	"io"
	"io/fs"
	"net/http"
)

type Server struct {
	Mux          *http.ServeMux
	WebFS        fs.FS
	ManifestJSON []byte
	Spotify      *SpotifyClient
	Playback     *PlaybackCache
	SecretStore  *SecretStore
	SecretSpecs  []SecretSpec
	AdminAuth    *AdminAuth
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		return fsys
	}
	return sub
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/.well-known/homenavi-integration.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.ManifestJSON)
	})

	RegisterAPIRoutes(mux, s.Spotify, s.Playback)
	if s.SecretStore != nil {
		NewSecretsAPI(s.SecretStore, s.SecretSpecs, s.AdminAuth).Register(mux)
	}

	assets := http.FileServer(http.FS(mustSub(s.WebFS, "assets")))
	mux.Handle("/assets/", http.StripPrefix("/assets/", assets))

	ui := http.FileServer(http.FS(mustSub(s.WebFS, "ui")))
	mux.Handle("/ui/", http.StripPrefix("/ui/", ui))

	widgets := http.FileServer(http.FS(mustSub(s.WebFS, "widgets")))
	mux.Handle("/widgets/", http.StripPrefix("/widgets/", widgets))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "not found")
	})

	s.Mux = mux
	return mux
}

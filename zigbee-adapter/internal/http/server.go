package http

import "net/http"

type Server struct {
	promHandler http.Handler
}

func NewServer(promHandler http.Handler) *Server {
	return &Server{promHandler: promHandler}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if s.promHandler != nil {
		mux.Handle("/metrics", s.promHandler)
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

package http

import "net/http"

func NewRouter(server *Server) http.Handler {
	return server.Handler()
}

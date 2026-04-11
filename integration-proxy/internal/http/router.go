package http

import (
	"crypto/rsa"
	"net/http"

	proxyauth "github.com/PetoAdam/homenavi/integration-proxy/internal/auth"
)

func NewRouter(server *Server, pubKey *rsa.PublicKey) http.Handler {
	return proxyauth.RequireResident(pubKey)(server.Routes())
}

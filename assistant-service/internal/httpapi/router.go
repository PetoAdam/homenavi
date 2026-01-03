package httpapi

import (
	"crypto/rsa"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/homenavi/assistant-service/internal/config"
)

// NewRouter creates the HTTP router
func NewRouter(h *Handler, cfg *config.Config) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(CORSMiddleware)

	// Health check (public)
	r.Get("/health", h.Health)

	// Load JWT public key
	pubKey, err := loadRSAPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		// Log warning but continue - auth will fail if key missing
		pubKey = nil
	}

	// Protected routes
	r.Group(func(r chi.Router) {
		if pubKey != nil {
			r.Use(JWTAuthMiddleware(pubKey))
		}

		// REST endpoints
		r.Get("/api/assistant/conversations", h.ListConversations)
		r.Post("/api/assistant/conversations", h.CreateConversation)
		r.Get("/api/assistant/devices", h.ListDevicesMerged)
		r.Get("/api/assistant/rooms", h.ListRooms)
		r.Get("/api/assistant/conversations/{id}", h.GetConversation)
		r.Put("/api/assistant/conversations/{id}", h.UpdateConversation)
		r.Delete("/api/assistant/conversations/{id}", h.DeleteConversation)

		// WebSocket endpoint
		r.Get("/ws/assistant", h.HandleWebSocket)
	})

	// Admin routes
	r.Group(func(r chi.Router) {
		if pubKey != nil {
			r.Use(JWTAuthMiddleware(pubKey))
			r.Use(RequireRoleMiddleware("admin"))
		}

		r.Get("/api/assistant/admin/status", h.AdminStatus)
		r.Get("/api/assistant/admin/models", h.AdminListModels)
	})

	return r
}

func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jwt.ParseRSAPublicKeyFromPEM(keyData)
}

// CORSMiddleware handles CORS
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

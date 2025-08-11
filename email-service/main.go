package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"email-service/internal/config"
	"email-service/internal/handlers"
	"email-service/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize email service
	emailService := services.NewEmailService(cfg)

	// Initialize handlers
	emailHandler := handlers.NewEmailHandler(emailService)

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Email endpoints
	r.Post("/send/verification", emailHandler.SendVerificationEmail)
	r.Post("/send/password-reset", emailHandler.SendPasswordResetEmail)
	r.Post("/send/2fa", emailHandler.Send2FAEmail)

	// Start server
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown
	// Initialize structured logging
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if os.Getenv("LOG_FORMAT") == "json" { handler = slog.NewJSONHandler(os.Stdout, nil) }
	slog.SetDefault(slog.New(handler))

	go func() {
		slog.Info("email service starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("email service shutting down")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("email service forced shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("email service stopped")
}

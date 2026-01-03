package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/homenavi/assistant-service/internal/config"
	"github.com/homenavi/assistant-service/internal/httpapi"
	"github.com/homenavi/assistant-service/internal/llm"
	"github.com/homenavi/assistant-service/internal/repository"
	"github.com/homenavi/assistant-service/internal/tools"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := repository.NewPostgresDB(cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize Ollama client
	ollamaClient := llm.NewOllamaClient(cfg.OllamaHost, cfg.OllamaModel, cfg.OllamaContextLength)

	// Initialize tool registry
	toolRegistry := tools.NewRegistry(cfg)

	// Initialize repositories
	conversationRepo := repository.NewConversationRepository(db)
	messageRepo := repository.NewMessageRepository(db)

	// Initialize HTTP API handler
	handler := httpapi.NewHandler(
		cfg,
		ollamaClient,
		toolRegistry,
		conversationRepo,
		messageRepo,
	)

	// Create router
	router := httpapi.NewRouter(handler, cfg)

	// Create server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second, // Longer for streaming responses
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Assistant service starting on port %s", cfg.Port)
		log.Printf("Using Ollama at %s with model %s", cfg.OllamaHost, cfg.OllamaModel)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

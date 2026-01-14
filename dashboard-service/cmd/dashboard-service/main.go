package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dashboard-service/internal/config"
	"dashboard-service/internal/httpapi"
	"dashboard-service/internal/middleware"
	"dashboard-service/internal/store"

	"github.com/go-chi/chi/v5"
)

func main() {
	cfg := config.Load()
	if cfg.JWTPublicKeyPath == "" {
		slog.Error("JWT_PUBLIC_KEY_PATH not set for dashboard-service")
		os.Exit(1)
	}

	pub, err := middleware.LoadRSAPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		slog.Error("failed to load jwt public key", "error", err)
		os.Exit(1)
	}

	db, err := store.OpenPostgres(
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.DBName,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.SSLMode,
	)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}

	repo, err := store.New(db)
	if err != nil {
		slog.Error("db init failed", "error", err)
		os.Exit(1)
	}

	srv := httpapi.NewServer(repo)

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.JWTAuthMiddlewareRS256(pub))
		// NOTE: gateway should enforce resident/admin route access; this is extra safety.
		r.Use(middleware.RoleAtLeastMiddleware("resident"))
		srv.RegisterRoutes(r)
	})

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	go func() {
		slog.Info("dashboard-service started", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)

	slog.Info("dashboard-service stopped")
}

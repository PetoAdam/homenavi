package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"auth-service/internal/config"
	"auth-service/internal/handlers/auth"
	"auth-service/internal/handlers/email"
	"auth-service/internal/handlers/oauth"
	"auth-service/internal/handlers/password"
	"auth-service/internal/handlers/profile"
	"auth-service/internal/handlers/twofactor"
	"auth-service/internal/handlers/user"
	"auth-service/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil { slog.Error("failed to load configuration", "error", err); os.Exit(1) }

	// Initialize structured logger
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if os.Getenv("LOG_FORMAT") == "json" { handler = slog.NewJSONHandler(os.Stdout, nil) }
	slog.SetDefault(slog.New(handler))

	slog.Info("auth service init", "port", cfg.Port)

	// Initialize services
	authService := services.NewAuthService(cfg)
	userService := services.NewUserService(cfg)
	emailService := services.NewEmailService(cfg)
	profilePictureService := services.NewProfilePictureService(cfg)

	// Initialize handlers
	signupHandler := auth.NewSignupHandler(userService)
	loginHandler := auth.NewLoginHandler(authService, userService, emailService)
	refreshHandler := auth.NewRefreshHandler(authService, userService)
	logoutHandler := auth.NewLogoutHandler(authService)

	passwordResetHandler := password.NewResetHandler(authService, userService, emailService)
	passwordChangeHandler := password.NewChangeHandler(authService, userService)
	emailVerifyHandler := email.NewVerificationHandler(authService, userService, emailService)

	twoFactorSetupHandler := twofactor.NewSetupHandler(authService, userService)
	twoFactorVerifyHandler := twofactor.NewVerifyHandler(authService, userService)
	twoFactorEmailHandler := twofactor.NewEmailHandler(authService, userService, emailService)

	profileHandler := profile.NewProfileHandler(authService, userService)
	avatarHandler := profile.NewAvatarHandler(authService, userService, profilePictureService)

	userDeleteHandler := user.NewDeleteHandler(authService, userService)
	userManageHandler := user.NewManageHandler(authService, userService)

	googleOAuthHandler := oauth.NewGoogleHandler(authService, userService)

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Authentication routes
	r.Route("/api/auth", func(r chi.Router) {
		// Auth endpoints
		r.Post("/signup", signupHandler.HandleSignup)
		r.Post("/login/start", loginHandler.HandleLoginStart)
		r.Post("/login/finish", loginHandler.HandleLoginFinish)
		r.Post("/refresh", refreshHandler.HandleRefresh)
		r.Post("/logout", logoutHandler.HandleLogout)

		// Password management
		r.Post("/password/reset/request", passwordResetHandler.HandlePasswordResetRequest)
		r.Post("/password/reset/confirm", passwordResetHandler.HandlePasswordResetConfirm)
		r.Post("/password/change", passwordChangeHandler.HandleChangePassword)

		// Email verification
		r.Post("/email/verify/request", emailVerifyHandler.HandleEmailVerifyRequest)
		r.Post("/email/verify/confirm", emailVerifyHandler.HandleEmailVerifyConfirm)

		// 2FA
		r.Post("/2fa/setup", twoFactorSetupHandler.Handle2FASetup)
		r.Post("/2fa/verify", twoFactorVerifyHandler.Handle2FAVerify)
		r.Post("/2fa/email/request", twoFactorEmailHandler.Handle2FAEmailRequest)
		r.Post("/2fa/email/verify", twoFactorEmailHandler.Handle2FAEmailVerify)

		// Profile
		r.Get("/me", profileHandler.HandleMe)
		r.Delete("/delete", userDeleteHandler.HandleDeleteUser)

		// User management
		r.Get("/users", userManageHandler.HandleList)
		r.Get("/users/{id}", userManageHandler.HandleGet)
		r.Patch("/users/{id}", userManageHandler.HandlePatch)
		r.Post("/users/{id}/lockout", userManageHandler.HandleLockout)

		// Profile pictures
		r.Post("/profile/generate-avatar", avatarHandler.HandleGenerateAvatar)
		r.Post("/profile/upload", avatarHandler.HandleUploadProfilePicture)

		// OAuth
		r.Get("/oauth/google/login", func(w http.ResponseWriter, r *http.Request) {
			state, err := authService.GenerateOAuthState()
			if err != nil {
				http.Error(w, "Failed to generate OAuth state", http.StatusInternalServerError)
				return
			}
			url := authService.GetGoogleAuthURL(state)
			http.Redirect(w, r, url, http.StatusTemporaryRedirect)
		})
		r.Get("/oauth/google/callback", googleOAuthHandler.HandleOAuthGoogleCallback)
	})

	// Start server
	server := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	go func() {
		slog.Info("auth service starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server listen failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("auth service shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("auth service forced shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("auth service stopped")
}

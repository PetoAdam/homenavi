package main

import (
	"context"
	"log"
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
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

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
	
	googleOAuthHandler := oauth.NewGoogleHandler(authService, userService)

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
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
		
		// Profile pictures
		r.Post("/profile/generate-avatar", avatarHandler.HandleGenerateAvatar)
		r.Post("/profile/upload", avatarHandler.HandleUploadProfilePicture)
		
		// OAuth
		r.Post("/oauth/google", googleOAuthHandler.HandleOAuthGoogle)
	})

	// Start server
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Auth service starting on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
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

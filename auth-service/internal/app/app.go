package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	httptransport "github.com/PetoAdam/homenavi/auth-service/internal/http"
	authhandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/auth"
	emailhandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/email"
	oauthhandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/oauth"
	passwordhandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/password"
	profilehandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/profile"
	twofactorhandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/twofactor"
	userhandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/user"
	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
)

// App is the composed auth-service application.
type App struct {
	server      *http.Server
	shutdownObs func()
	authService *authdomain.Service
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	cacheStore, err := cacheinfra.NewRedisStore(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("create redis store: %w", err)
	}
	authService := authdomain.NewService(authdomain.Config{
		JWTPrivateKey:           cfg.JWTPrivateKey,
		AccessTokenTTL:          cfg.AccessTokenTTL,
		RefreshTokenTTL:         cfg.RefreshTokenTTL,
		EmailVerificationTTL:    cfg.EmailVerificationTTL,
		PasswordResetTTL:        cfg.PasswordResetTTL,
		TwoFactorTTL:            cfg.TwoFactorTTL,
		GoogleOAuthClientID:     cfg.GoogleOAuthClientID,
		GoogleOAuthClientSecret: cfg.GoogleOAuthClientSecret,
		GoogleOAuthRedirectURL:  cfg.GoogleOAuthRedirectURL,
		LoginMaxFailures:        cfg.LoginMaxFailures,
		LoginLockoutSeconds:     cfg.LoginLockoutSeconds,
		CodeMaxFailures:         cfg.CodeMaxFailures,
		CodeLockoutSeconds:      cfg.CodeLockoutSeconds,
	}, cacheStore)
	userClient := clientsinfra.NewUserClient(clientsinfra.UserConfig{BaseURL: cfg.UserServiceURL, JWTPrivateKey: cfg.JWTPrivateKey})
	emailClient := clientsinfra.NewEmailClient(cfg.EmailServiceURL)
	profilePictureClient := clientsinfra.NewProfilePictureClient(cfg.ProfilePictureServiceURL)

	signupHandler := authhandlers.NewSignupHandler(userClient)
	loginHandler := authhandlers.NewLoginHandler(authService, userClient, emailClient)
	refreshHandler := authhandlers.NewRefreshHandler(authService, userClient)
	logoutHandler := authhandlers.NewLogoutHandler(authService)
	passwordResetHandler := passwordhandlers.NewResetHandler(authService, userClient, emailClient)
	passwordChangeHandler := passwordhandlers.NewChangeHandler(authService, userClient)
	emailVerifyHandler := emailhandlers.NewVerificationHandler(authService, userClient, emailClient)
	twoFactorSetupHandler := twofactorhandlers.NewSetupHandler(authService, userClient)
	twoFactorVerifyHandler := twofactorhandlers.NewVerifyHandler(authService, userClient)
	twoFactorEmailHandler := twofactorhandlers.NewEmailHandler(authService, userClient, emailClient)
	profileHandler := profilehandlers.NewProfileHandler(authService, userClient)
	avatarHandler := profilehandlers.NewAvatarHandler(authService, userClient, profilePictureClient)
	userDeleteHandler := userhandlers.NewDeleteHandler(authService, userClient)
	userManageHandler := userhandlers.NewManageHandler(authService, userClient)
	googleOAuthHandler := oauthhandlers.NewGoogleHandler(authService, userClient)

	shutdownObs, promHandler, tracer, err := sharedobs.SetupObservability("auth-service")
	if err != nil {
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	router := httptransport.NewRouter(httptransport.Routes{
		HandleSignup:               signupHandler.HandleSignup,
		HandleLoginStart:           loginHandler.HandleLoginStart,
		HandleLoginFinish:          loginHandler.HandleLoginFinish,
		HandleRefresh:              refreshHandler.HandleRefresh,
		HandleLogout:               logoutHandler.HandleLogout,
		HandlePasswordResetRequest: passwordResetHandler.HandlePasswordResetRequest,
		HandlePasswordResetConfirm: passwordResetHandler.HandlePasswordResetConfirm,
		HandlePasswordChange:       passwordChangeHandler.HandleChangePassword,
		HandleEmailVerifyRequest:   emailVerifyHandler.HandleEmailVerifyRequest,
		HandleEmailVerifyConfirm:   emailVerifyHandler.HandleEmailVerifyConfirm,
		HandleTwoFactorSetup:       twoFactorSetupHandler.Handle2FASetup,
		HandleTwoFactorVerify:      twoFactorVerifyHandler.Handle2FAVerify,
		HandleTwoFactorEmailReq:    twoFactorEmailHandler.Handle2FAEmailRequest,
		HandleTwoFactorEmailVerify: twoFactorEmailHandler.Handle2FAEmailVerify,
		HandleMe:                   profileHandler.HandleMe,
		HandleDeleteUser:           userDeleteHandler.HandleDeleteUser,
		HandleListUsers:            userManageHandler.HandleList,
		HandleGetUser:              userManageHandler.HandleGet,
		HandlePatchUser:            userManageHandler.HandlePatch,
		HandleLockoutUser:          userManageHandler.HandleLockout,
		HandleGenerateAvatar:       avatarHandler.HandleGenerateAvatar,
		HandleCreateUploadURL:      avatarHandler.HandleCreateUploadURL,
		HandleCompleteUpload:       avatarHandler.HandleCompleteProfilePictureUpload,
		HandleUploadProfilePicture: avatarHandler.HandleUploadProfilePicture,
		HandleGoogleOAuthLogin: func(w http.ResponseWriter, r *http.Request) {
			state, err := authService.GenerateOAuthState()
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to generate oauth state", "code": http.StatusInternalServerError})
				return
			}
			http.Redirect(w, r, authService.GetGoogleAuthURL(state), http.StatusTemporaryRedirect)
		},
		HandleGoogleOAuthCallback: googleOAuthHandler.HandleOAuthGoogleCallback,
	}, promHandler, tracer)

	return &App{
		server:      &http.Server{Addr: ":" + cfg.Port, Handler: router},
		shutdownObs: shutdownObs,
		authService: authService,
		logger:      logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.shutdownObs()
	defer a.authService.Close()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("auth service starting", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("auth service shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

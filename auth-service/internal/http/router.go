package http

import (
	"net/http"
	"strings"
	"time"

	authhandlers "github.com/PetoAdam/homenavi/auth-service/internal/http/auth"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Routes groups auth-service HTTP handlers.
type Routes struct {
	HandleSignup                http.HandlerFunc
	HandleLoginStart            http.HandlerFunc
	HandleLoginFinish           http.HandlerFunc
	HandleRefresh               http.HandlerFunc
	HandleLogout                http.HandlerFunc
	HandlePasswordResetRequest  http.HandlerFunc
	HandlePasswordResetConfirm  http.HandlerFunc
	HandlePasswordChange        http.HandlerFunc
	HandleEmailVerifyRequest    http.HandlerFunc
	HandleEmailVerifyConfirm    http.HandlerFunc
	HandleTwoFactorSetup        http.HandlerFunc
	HandleTwoFactorVerify       http.HandlerFunc
	HandleTwoFactorEmailReq     http.HandlerFunc
	HandleTwoFactorEmailVerify  http.HandlerFunc
	HandleMe                    http.HandlerFunc
	HandleDeleteUser            http.HandlerFunc
	HandleListUsers             http.HandlerFunc
	HandleGetUser               http.HandlerFunc
	HandlePatchUser             http.HandlerFunc
	HandleLockoutUser           http.HandlerFunc
	HandleGenerateAvatar        http.HandlerFunc
	HandleCreateUploadURL       http.HandlerFunc
	HandleCompleteUpload        http.HandlerFunc
	HandleUploadProfilePicture  http.HandlerFunc
	HandleGoogleOAuthLogin      http.HandlerFunc
	HandleGoogleOAuthCallback   http.HandlerFunc
	HandleJWKS                  http.HandlerFunc
	HandleAuthorizationMetadata http.HandlerFunc
	HandleOAuthAuthorize        http.HandlerFunc
	HandleOAuthToken            http.HandlerFunc
	HandleOAuthRegister         http.HandlerFunc
	HandleOAuthTransaction      http.HandlerFunc
	HandleOAuthLogin            http.HandlerFunc
	HandleOAuthVerification     http.HandlerFunc
	HandleOAuthApproval         http.HandlerFunc
	HandleOAuthDenial           http.HandlerFunc
	HandleOAuthConsentGrant     http.HandlerFunc
	HandleOAuthConsentRevoke    http.HandlerFunc
}

func NewRouter(routes Routes, promHandler http.Handler, tracer oteltrace.Tracer) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(sharedobs.MetricsAndTracingMiddleware(tracer, "auth-service"))

	r.Handle("/metrics", promHandler)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	r.Get("/.well-known/oauth-authorization-server", orNotImplemented(routes.HandleAuthorizationMetadata))
	r.Get("/.well-known/oauth-authorization-server/api/auth", orNotImplemented(routes.HandleAuthorizationMetadata))

	r.Route("/api/auth", func(r chi.Router) {
		r.Use(browserSessionAuthorization)
		r.Post("/signup", orNotImplemented(routes.HandleSignup))
		r.Post("/login/start", orNotImplemented(routes.HandleLoginStart))
		r.Post("/login/finish", orNotImplemented(routes.HandleLoginFinish))
		r.Post("/refresh", orNotImplemented(routes.HandleRefresh))
		r.Post("/logout", orNotImplemented(routes.HandleLogout))
		r.Post("/password/reset/request", orNotImplemented(routes.HandlePasswordResetRequest))
		r.Post("/password/reset/confirm", orNotImplemented(routes.HandlePasswordResetConfirm))
		r.Post("/password/change", orNotImplemented(routes.HandlePasswordChange))
		r.Post("/email/verify/request", orNotImplemented(routes.HandleEmailVerifyRequest))
		r.Post("/email/verify/confirm", orNotImplemented(routes.HandleEmailVerifyConfirm))
		r.Post("/2fa/setup", orNotImplemented(routes.HandleTwoFactorSetup))
		r.Post("/2fa/verify", orNotImplemented(routes.HandleTwoFactorVerify))
		r.Post("/2fa/email/request", orNotImplemented(routes.HandleTwoFactorEmailReq))
		r.Post("/2fa/email/verify", orNotImplemented(routes.HandleTwoFactorEmailVerify))
		r.Get("/me", orNotImplemented(routes.HandleMe))
		r.Delete("/delete", orNotImplemented(routes.HandleDeleteUser))
		r.Get("/users", orNotImplemented(routes.HandleListUsers))
		r.Get("/users/{id}", orNotImplemented(routes.HandleGetUser))
		r.Patch("/users/{id}", orNotImplemented(routes.HandlePatchUser))
		r.Post("/users/{id}/lockout", orNotImplemented(routes.HandleLockoutUser))
		r.Post("/profile/generate-avatar", orNotImplemented(routes.HandleGenerateAvatar))
		r.Post("/profile/upload-url", orNotImplemented(routes.HandleCreateUploadURL))
		r.Post("/profile/upload-complete", orNotImplemented(routes.HandleCompleteUpload))
		r.Post("/profile/upload", orNotImplemented(routes.HandleUploadProfilePicture))
		r.Get("/oauth/google/login", orNotImplemented(routes.HandleGoogleOAuthLogin))
		r.Get("/oauth/google/callback", orNotImplemented(routes.HandleGoogleOAuthCallback))
		r.Get("/oauth/jwks.json", orNotImplemented(routes.HandleJWKS))
		r.Get("/oauth/authorize", orNotImplemented(routes.HandleOAuthAuthorize))
		r.Post("/oauth/authorize", orNotImplemented(routes.HandleOAuthAuthorize))
		r.Post("/oauth/token", orNotImplemented(routes.HandleOAuthToken))
		r.Post("/oauth/register", orNotImplemented(routes.HandleOAuthRegister))
		r.Get("/oauth/transaction", orNotImplemented(routes.HandleOAuthTransaction))
		r.Post("/oauth/transaction/login", orNotImplemented(routes.HandleOAuthLogin))
		r.Post("/oauth/transaction/verify", orNotImplemented(routes.HandleOAuthVerification))
		r.Post("/oauth/transaction/approve", orNotImplemented(routes.HandleOAuthApproval))
		r.Post("/oauth/transaction/deny", orNotImplemented(routes.HandleOAuthDenial))
		r.Post("/oauth/consents", orNotImplemented(routes.HandleOAuthConsentGrant))
		r.Delete("/oauth/consents/{clientID}", orNotImplemented(routes.HandleOAuthConsentRevoke))
	})

	return r
}

func browserSessionAuthorization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if cookie, err := r.Cookie(authhandlers.AccessTokenCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
				r.Header.Set("Authorization", "Bearer "+cookie.Value)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func orNotImplemented(fn http.HandlerFunc) http.HandlerFunc {
	if fn != nil {
		return fn
	}
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	}
}

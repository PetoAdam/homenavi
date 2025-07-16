package main

import (
	"log"
	"net/http"

	"auth-service/handlers"
)

func main() {
	// Load env/config as needed
	log.Println("Starting auth-service on :8000")

	http.HandleFunc("/api/auth/signup", handlers.HandleSignup)
	http.HandleFunc("/api/auth/login/start", handlers.HandleLoginStart)
	http.HandleFunc("/api/auth/login/finish", handlers.HandleLoginFinish)
	http.HandleFunc("/api/auth/2fa/setup", handlers.Handle2FASetup)
	http.HandleFunc("/api/auth/2fa/verify", handlers.Handle2FAVerify)
	http.HandleFunc("/api/auth/password/reset/request", handlers.HandlePasswordResetRequest)
	http.HandleFunc("/api/auth/password/reset/confirm", handlers.HandlePasswordResetConfirm)
	http.HandleFunc("/api/auth/email/verify/request", handlers.HandleEmailVerifyRequest)
	http.HandleFunc("/api/auth/email/verify/confirm", handlers.HandleEmailVerifyConfirm)
	http.HandleFunc("/api/auth/2fa/email/request", handlers.Handle2FAEmailRequest)
	http.HandleFunc("/api/auth/2fa/email/verify", handlers.Handle2FAEmailVerify)
	http.HandleFunc("/api/auth/delete", handlers.HandleDeleteUser)
	http.HandleFunc("/api/auth/refresh", handlers.HandleRefresh)
	http.HandleFunc("/api/auth/logout", handlers.HandleLogout)
	http.HandleFunc("/api/auth/oauth/google", handlers.HandleOAuthGoogle)
	http.HandleFunc("/api/auth/me", handlers.HandleMe)
	http.HandleFunc("/api/auth/password/change", handlers.HandleChangePassword)

	http.ListenAndServe(":8000", nil)
}

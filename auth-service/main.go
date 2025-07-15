package main

import (
	"log"
	"net/http"

	"auth-service/handlers"
)

func main() {
	// Load env/config as needed
	log.Println("Starting auth-service on :8000")

	http.HandleFunc("/signup", handlers.HandleSignup)
	http.HandleFunc("/login", handlers.HandleLogin)
	http.HandleFunc("/2fa/setup", handlers.Handle2FASetup)
	http.HandleFunc("/2fa/verify", handlers.Handle2FAVerify)
	http.HandleFunc("/password/reset/request", handlers.HandlePasswordResetRequest)
	http.HandleFunc("/password/reset/confirm", handlers.HandlePasswordResetConfirm)
	http.HandleFunc("/email/verify/request", handlers.HandleEmailVerifyRequest)
	http.HandleFunc("/email/verify/confirm", handlers.HandleEmailVerifyConfirm)
	http.HandleFunc("/2fa/email/request", handlers.Handle2FAEmailRequest)
	http.HandleFunc("/2fa/email/verify", handlers.Handle2FAEmailVerify)
	http.HandleFunc("/delete", handlers.HandleDeleteUser)

	http.ListenAndServe(":8000", nil)
}

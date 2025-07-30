package oauth

import (
	"net/http"

	"auth-service/internal/services"
)

type GoogleHandler struct {
	authService *services.AuthService
	userService *services.UserService
}

func NewGoogleHandler(authService *services.AuthService, userService *services.UserService) *GoogleHandler {
	return &GoogleHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *GoogleHandler) HandleOAuthGoogleCallback(w http.ResponseWriter, r *http.Request) {
	// Get code from query parameters (this is how Google sends it)
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		// Check for error from Google
		if errorParam := r.URL.Query().Get("error"); errorParam != "" {
			// Redirect back to frontend with error
			http.Redirect(w, r, "/?error=oauth_cancelled", http.StatusTemporaryRedirect)
			return
		}
		http.Redirect(w, r, "/?error=oauth_failed", http.StatusTemporaryRedirect)
		return
	}

	// Validate OAuth state parameter for security (CSRF protection)
	if err := h.authService.ValidateOAuthState(state); err != nil {
		http.Redirect(w, r, "/?error=invalid_state", http.StatusTemporaryRedirect)
		return
	}

	// Exchange Google OAuth code for user information
	userInfo, err := h.authService.ExchangeGoogleOAuthCode(code, "/api/auth/oauth/google/callback")
	if err != nil {
		http.Redirect(w, r, "/?error=oauth_exchange_failed", http.StatusTemporaryRedirect)
		return
	}

	// Try to find user by Google ID first
	user, err := h.userService.GetUserByGoogleID(userInfo.ID)
	if err == nil && user != nil {
		// User exists with this Google ID, log them in
		accessToken, err := h.authService.IssueAccessToken(user)
		if err != nil {
			http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect)
			return
		}

		refreshToken, err := h.authService.IssueRefreshToken(user.ID)
		if err != nil {
			http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect)
			return
		}

		// Redirect back to frontend with tokens
		redirectURL := "/?access_token=" + accessToken + "&refresh_token=" + refreshToken
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	// Try to find user by email
	user, err = h.userService.GetUserByEmail(userInfo.Email)
	if err == nil && user != nil {
		// User exists with this email
		if user.GoogleID == "" {
			// Link Google ID to existing user
			if err := h.userService.LinkGoogleID(user.ID, userInfo.ID); err != nil {
				http.Redirect(w, r, "/?error=link_failed", http.StatusTemporaryRedirect)
				return
			}
			user.GoogleID = userInfo.ID // Update local copy
		} else if user.GoogleID != userInfo.ID {
			// Conflict: email used by different Google account
			http.Redirect(w, r, "/?error=email_conflict", http.StatusTemporaryRedirect)
			return
		}

		// Issue tokens
		accessToken, err := h.authService.IssueAccessToken(user)
		if err != nil {
			http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect)
			return
		}

		refreshToken, err := h.authService.IssueRefreshToken(user.ID)
		if err != nil {
			http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect)
			return
		}

		// Redirect back to frontend with tokens
		redirectURL := "/?access_token=" + accessToken + "&refresh_token=" + refreshToken
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	// User doesn't exist, create new user
	user, err = h.userService.CreateGoogleUser(userInfo)
	if err != nil {
		http.Redirect(w, r, "/?error=user_creation_failed", http.StatusTemporaryRedirect)
		return
	}

	// Issue tokens for new user
	accessToken, err := h.authService.IssueAccessToken(user)
	if err != nil {
		http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect)
		return
	}

	refreshToken, err := h.authService.IssueRefreshToken(user.ID)
	if err != nil {
		http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect)
		return
	}

	// Redirect back to frontend with tokens
	redirectURL := "/?access_token=" + accessToken + "&refresh_token=" + refreshToken
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

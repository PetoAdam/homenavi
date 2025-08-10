package oauth

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

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
	started := time.Now()
	slog.Info("oauth google callback start", "query", r.URL.RawQuery)
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
		slog.Error("oauth google exchange failed", "error", err)
		http.Redirect(w, r, "/?error=oauth_exchange_failed", http.StatusTemporaryRedirect)
		return
	}

	// Prefer email match first (link scenario where user signed up via password before OAuth)
	user, err := h.userService.GetUserByEmail(userInfo.Email)
	if err == nil && user != nil {
		slog.Info("oauth google existing user by email", "email", userInfo.Email, "user_id", user.ID, "google_id", user.GoogleID)
		if user.GoogleID == "" {
			if err := h.userService.LinkGoogleID(user.ID, userInfo.ID); err != nil {
				slog.Error("oauth google link failure", "user_id", user.ID, "error", err)
				http.Redirect(w, r, "/?error=link_failed", http.StatusTemporaryRedirect)
				return
			}
			user.GoogleID = userInfo.ID
			slog.Info("oauth google linked google id", "google_id", userInfo.ID, "user_id", user.ID)
		} else if user.GoogleID != userInfo.ID {
			slog.Warn("oauth google email conflict", "existing_google_id", user.GoogleID, "incoming_google_id", userInfo.ID)
			http.Redirect(w, r, "/?error=email_conflict", http.StatusTemporaryRedirect)
			return
		}
		// Proceed to token issuance
		accessToken, err := h.authService.IssueAccessToken(user)
		if err != nil { http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect); return }
		refreshToken, err := h.authService.IssueRefreshToken(user.ID)
		if err != nil { http.Redirect(w, r, "/?error=token_failed", http.StatusTemporaryRedirect); return }
		redirectURL := fmt.Sprintf("/?access_token=%s&refresh_token=%s", accessToken, refreshToken)
		slog.Info("oauth google success email path", "user_id", user.ID, "ms", time.Since(started).Milliseconds())
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	// Fallback: lookup by Google ID (scenario: previously linked only via OAuth route)
	user, err = h.userService.GetUserByGoogleID(userInfo.ID)
	if err == nil && user != nil {
		slog.Info("oauth google existing user by google id", "google_id", userInfo.ID, "user_id", user.ID)
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
		slog.Info("oauth google found existing user by email", "email", userInfo.Email, "user_id", user.ID, "google_id", user.GoogleID)
		if user.GoogleID == "" {
			if err := h.userService.LinkGoogleID(user.ID, userInfo.ID); err != nil {
				slog.Error("oauth google link failure", "user_id", user.ID, "error", err)
				http.Redirect(w, r, "/?error=link_failed", http.StatusTemporaryRedirect)
				return
			}
			user.GoogleID = userInfo.ID
			slog.Info("oauth google linked google id", "google_id", userInfo.ID, "user_id", user.ID)
		} else if user.GoogleID != userInfo.ID {
			slog.Warn("oauth google email conflict", "existing_google_id", user.GoogleID, "incoming_google_id", userInfo.ID)
			http.Redirect(w, r, "/?error=email_conflict", http.StatusTemporaryRedirect)
			return
		}
		// DO NOT override local profile attributes with Google defaults; keep existing first/last/user_name

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
	slog.Info("oauth google creating new user", "email", userInfo.Email)
	user, err = h.userService.CreateGoogleUser(userInfo)
	if err != nil {
		slog.Error("oauth google create user failed", "error", err)
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
	redirectURL := fmt.Sprintf("/?access_token=%s&refresh_token=%s", accessToken, refreshToken)
	slog.Info("oauth google success", "user_id", user.ID, "ms", time.Since(started).Milliseconds())
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

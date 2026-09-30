package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	authtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/auth/transport"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

type demoBootstrapTokenIssuer interface {
	IssueTokenPair(user *clientsinfra.User) (*authdomain.TokenPair, error)
}

type demoBootstrapUserCreator interface {
	CreateResidentDemoUser(userName, email, password, firstName, lastName string) (*clientsinfra.User, error)
}

type DemoBootstrapHandler struct {
	authService demoBootstrapTokenIssuer
	userService demoBootstrapUserCreator
}

func NewDemoBootstrapHandler(authService demoBootstrapTokenIssuer, userService demoBootstrapUserCreator) *DemoBootstrapHandler {
	return &DemoBootstrapHandler{authService: authService, userService: userService}
}

func (h *DemoBootstrapHandler) HandleDemoBootstrap(w http.ResponseWriter, r *http.Request) {
	user, err := h.createDemoUser()
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.InternalServerError("failed to create demo user", err))
		return
	}

	tokens, err := h.authService.IssueTokenPair(user)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.InternalServerError("failed to issue demo tokens", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(authtransport.LoginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	})
}

func (h *DemoBootstrapHandler) createDemoUser() (*clientsinfra.User, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		userName, email, password, err := generateDemoCredentials()
		if err != nil {
			return nil, errors.InternalServerError("failed to generate demo credentials", err)
		}
		user, err := h.userService.CreateResidentDemoUser(userName, email, password, "Demo", "Visitor")
		if err == nil {
			return user, nil
		}
		lastErr = err
		appErr, ok := err.(*errors.AppError)
		if !ok || appErr.Code != http.StatusBadRequest || !strings.Contains(strings.ToLower(appErr.Message), "already exists") {
			return nil, err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.InternalServerError("failed to create demo user", nil)
}

func generateDemoCredentials() (string, string, string, error) {
	suffixBytes := make([]byte, 6)
	if _, err := rand.Read(suffixBytes); err != nil {
		return "", "", "", err
	}
	passwordBytes := make([]byte, 18)
	if _, err := rand.Read(passwordBytes); err != nil {
		return "", "", "", err
	}
	suffix := hex.EncodeToString(suffixBytes)
	passwordTail := hex.EncodeToString(passwordBytes)
	return "demo_" + suffix, "demo+" + suffix + "@example.com", "Demo!" + passwordTail + "9a", nil
}
package clients

import (
	"crypto/rsa"
	"fmt"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	"github.com/golang-jwt/jwt/v5"
)

type DashboardConfig struct {
	BaseURL       string
	JWTPrivateKey *rsa.PrivateKey
}

type DashboardClient struct {
	baseURL       string
	jwtPrivateKey *rsa.PrivateKey
	httpClient    *http.Client
}

func NewDashboardClient(cfg DashboardConfig) *DashboardClient {
	return &DashboardClient{baseURL: cfg.BaseURL, jwtPrivateKey: cfg.JWTPrivateKey, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *DashboardClient) DeleteUserDashboard(userID string) error {
	token, err := c.issueInternalToken("")
	if err != nil {
		return errors.InternalServerError("failed to issue internal token", err)
	}
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/dashboard/users/"+userID, nil)
	if err != nil {
		return errors.InternalServerError("failed to create dashboard cleanup request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.InternalServerError("failed to delete dashboard", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.InternalServerError("dashboard service returned unexpected status", nil)
	}
	return nil
}

func (c *DashboardClient) issueInternalToken(userID string) (string, error) {
	claims := jwt.MapClaims{"sub": userID, "exp": time.Now().Add(2 * time.Minute).Unix(), "iat": time.Now().Unix(), "role": "service"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if c.jwtPrivateKey == nil {
		return "", fmt.Errorf("jwt private key not configured")
	}
	return token.SignedString(c.jwtPrivateKey)
}
